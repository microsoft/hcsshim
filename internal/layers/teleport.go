package layers

import (
	"context"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/teleportd"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// This file holds the logic for handling "teleported" layers.
// More information about teleport:
// https://azure.microsoft.com/en-us/resources/videos/azure-friday-how-to-expedite-container-startup-with-project-teleport-and-azure-container-registry/
// https://stevelasker.blog/2019/10/29/azure-container-registry-teleportation/
//
// This essentially boils down to making different GCS calls. Instead of mapping in every
// layer.vhd as a vpmem or scsi device, we make a call to mount the smb share and then calls to
// loopback mount the vhds. The way to detect if the image we're getting asked
// to run is a teleportable image is there won't be a layer.vhd in the layer folder, but rather
// a layer file with the layer digest written in it. The files in the share will be named after
// their layer digest (excluding the sha:256 prefix), so it's a straightfoward conversion from
// layer digest to name of the virtual disk.

// Checks if the image/layer folders we've received are teleportable.
func isTeleportable(layers []string) bool {
	// There should never be a case where if one of the layer folders has a layer file
	// in it the others don't. Either the image is deemed to be teleportable and all of the
	// folders will get a layer file placed in them or none of them will and they'll get a layer.vhd
	// unpacked in the directory as normal.
	_, err := os.Stat(filepath.Join(layers[0], "layer"))
	return err == nil
}

// teleportLayers carries out the logic for mounting the layers inside of the guest.
func teleportLayers(ctx context.Context, layers []string, vm *uvm.UtilityVM) ([]string, error) {
	conn, err := grpc.Dial(
		"/run/teleportd/manifest.sock",
		grpc.WithInsecure(),
		grpc.WithContextDialer(unixConnect),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// To grab the smb share info, we need to build up a key to send to the teleport service.
	// Each layer folder should contain a layer file with the layer digest of that layer written to it,
	// and the key will be a comma seperated list of the layer digests. We loop backwards as the key is in
	// the order of the first layer unpacked, but the layerfolders are in the opposite order.
	// We can skip the last layer as this will just contain the scratch vhd.
	//
	// e.g. sha:123,sha:456,sha:789
	var layerDigests []string
	for i := len(layers) - 2; i >= 0; i-- {
		data, err := ioutil.ReadFile(filepath.Join(layers[i], "layer"))
		if err != nil {
			return nil, errors.Wrap(err, "failed to read layer file")
		}
		layerDigests = append(layerDigests, string(data))
	}

	cl := teleportd.NewManifestStoreClient(conn)
	resp, err := cl.Source(ctx, &teleportd.ManifestRequest{Key: strings.Join(layerDigests, ",")})
	if err != nil {
		return nil, err
	}

	if len(resp.Layers) == 0 || resp.Layers[0] == nil {
		return nil, errors.New("received empty response for layers")
	}

	// For now at least, all of the layers contain a copy of the same source, username and password for the
	// share. The reason each layer has the information instead of them being independent fields is to accomodate for
	// the possibility of the vhds being located in seperate shares in the future.
	var (
		source   = resp.Layers[0].Source
		username = resp.Layers[0].Username
		password = resp.Layers[0].Password
	)
	cifs, err := vm.AddCIFSMount(ctx, source, username, password)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to cifs mount %s", source)
	}

	// Loop over the layers in the share and mount each one loopback. Once again in reverse order.
	var uvmPaths []string
	for i := len(resp.Layers) - 1; i >= 0; i-- {
		// Grab the path to the file in the UVM
		backingFile := ospath.Join("linux", cifs.MountPath(), resp.Layers[i].File)
		uvmPath, err := vm.AddLoopback(ctx, backingFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to add loopback device with backing file %s", backingFile)
		}
		uvmPaths = append(uvmPaths, uvmPath)
	}

	return uvmPaths, nil
}

// unixConnect to unix socket
func unixConnect(ctx context.Context, addr string) (net.Conn, error) {
	unixAddr, err := net.ResolveUnixAddr("unix", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUnix("unix", nil, unixAddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
