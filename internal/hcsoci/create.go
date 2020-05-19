// +build windows

package hcsoci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var (
	lcowRootInUVM = "/run/gcs/c/%s"
	wcowRootInUVM = `C:\c\%s`
)

// CreateOptions are the set of fields used to call CreateContainer().
// Note: In the spec, the LayerFolders must be arranged in the same way in which
// moby configures them: layern, layern-1,...,layer2,layer1,scratch
// where layer1 is the base read-only layer, layern is the top-most read-only
// layer, and scratch is the RW layer. This is for historical reasons only.
type CreateOptions struct {

	// Common parameters
	ID               string             // Identifier for the container
	Owner            string             // Specifies the owner. Defaults to executable name.
	Spec             *specs.Spec        // Definition of the container or utility VM being created
	SchemaVersion    *hcsschema.Version // Requested Schema Version. Defaults to v2 for RS5, v1 for RS1..RS4
	HostingSystem    *uvm.UtilityVM     // Utility or service VM in which the container is to be created.
	NetworkNamespace string             // Host network namespace to use (overrides anything in the spec)

	// This is an advanced debugging parameter. It allows for diagnosibility by leaving a containers
	// resources allocated in case of a failure. Thus you would be able to use tools such as hcsdiag
	// to look at the state of a utility VM to see what resources were allocated. Obviously the caller
	// must a) not tear down the utility VM on failure (or pause in some way) and b) is responsible for
	// performing the ReleaseResources() call themselves.
	DoNotReleaseResourcesOnFailure bool
}

// createOptionsInternal is the set of user-supplied create options, but includes internal
// fields for processing the request once user-supplied stuff has been validated.
type createOptionsInternal struct {
	*CreateOptions

	actualSchemaVersion    *hcsschema.Version // Calculated based on Windows build and optional caller-supplied override
	actualID               string             // Identifier for the container
	actualOwner            string             // Owner for the container
	actualNetworkNamespace string
	saveAsTemplate         bool   // Are we going to save this container as a template
	templateID             string // is this container being created by cloning a container with id template ID
}

func initializeCreateOptions(ctx context.Context, createOptions *CreateOptions) (*createOptionsInternal, error) {
	coi := &createOptionsInternal{
		CreateOptions: createOptions,
		actualID:      createOptions.ID,
		actualOwner:   createOptions.Owner,
	}

	// Defaults if omitted by caller.
	if coi.actualID == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		coi.actualID = g.String()
	}
	if coi.actualOwner == "" {
		coi.actualOwner = filepath.Base(os.Args[0])
	}

	if coi.Spec == nil {
		return nil, fmt.Errorf("Spec must be supplied")
	}

	if coi.HostingSystem != nil {
		// By definition, a hosting system can only be supplied for a v2 Xenon.
		coi.actualSchemaVersion = schemaversion.SchemaV21()
	} else {
		coi.actualSchemaVersion = schemaversion.DetermineSchemaVersion(coi.SchemaVersion)
	}

	coi.saveAsTemplate = oci.ParseAnnotationsSaveAsTemplate(ctx, createOptions.Spec)
	coi.templateID = oci.ParseAnnotationsTemplateID(ctx, createOptions.Spec)

	log.G(ctx).WithFields(logrus.Fields{
		"options": fmt.Sprintf("%+v", createOptions),
		"schema":  coi.actualSchemaVersion,
	}).Debug("hcsshim::initializeCreateOptions")

	return coi, nil
}

// configureSandboxNetwork creates a new network namespace for the pod (sandbox)
// if required and then adds that namespace to the pod.
func configureSandboxNetwork(ctx context.Context, coi *createOptionsInternal, resources *Resources) error {
	if coi.NetworkNamespace != "" {
		resources.netNS = coi.NetworkNamespace
	} else {
		err := createNetworkNamespace(ctx, coi, resources)
		if err != nil {
			return err
		}
	}
	coi.actualNetworkNamespace = resources.netNS

	if coi.HostingSystem != nil {
		ct, _, err := oci.GetSandboxTypeAndID(coi.Spec.Annotations)
		if err != nil {
			return err
		}
		// Only add the network namespace to a standalone or sandbox
		// container but not a workload container in a sandbox that inherits
		// the namespace.
		if ct == oci.KubernetesContainerTypeNone || ct == oci.KubernetesContainerTypeSandbox {
			if err = SetupNetworkNamespace(ctx, coi.HostingSystem, coi.actualNetworkNamespace); err != nil {
				return err
			}
			resources.addedNetNSToVM = true
		}
	}

	return nil
}

// CreateContainer creates a container. It can cope with a  wide variety of
// scenarios, including v1 HCS schema calls, as well as more complex v2 HCS schema
// calls. Note we always return the resources that have been allocated, even in the
// case of an error. This provides support for the debugging option not to
// release the resources on failure, so that the client can make the necessary
// call to release resources that have been allocated as part of calling this function.
func CreateContainer(ctx context.Context, createOptions *CreateOptions) (_ cow.Container, _ *Resources, err error) {

	coi, err := initializeCreateOptions(ctx, createOptions)
	if err != nil {
		return nil, nil, err
	}

	resources := &Resources{
		id: createOptions.ID,
	}
	defer func() {
		if err != nil {
			if !coi.DoNotReleaseResourcesOnFailure {
				ReleaseResources(ctx, resources, coi.HostingSystem, true)
			}
		}
	}()

	if coi.HostingSystem != nil {
		n := coi.HostingSystem.ContainerCounter()
		if coi.Spec.Linux != nil {
			resources.containerRootInUVM = fmt.Sprintf(lcowRootInUVM, createOptions.ID)
		} else {
			resources.containerRootInUVM = fmt.Sprintf(wcowRootInUVM, strconv.FormatUint(n, 16))
		}
	}

	// Create a network namespace if necessary.
	if coi.Spec.Windows != nil &&
		coi.Spec.Windows.Network != nil &&
		schemaversion.IsV21(coi.actualSchemaVersion) {
		err = configureSandboxNetwork(ctx, coi, resources)
		if err != nil {
			return nil, resources, fmt.Errorf("failure while creating namespace for container: %s", err)
		}
	}

	var hcsDocument, gcsDocument interface{}
	log.G(ctx).Debug("hcsshim::CreateContainer allocating resources")
	if coi.Spec.Linux != nil {
		if schemaversion.IsV10(coi.actualSchemaVersion) {
			return nil, resources, errors.New("LCOW v1 not supported")
		}
		log.G(ctx).Debug("hcsshim::CreateContainer allocateLinuxResources")
		err = allocateLinuxResources(ctx, coi, resources)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
			return nil, resources, err
		}
		gcsDocument, err = createLinuxContainerDocument(ctx, coi, resources.containerRootInUVM)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
			return nil, resources, err
		}
	} else {
		err = allocateWindowsResources(ctx, coi, resources)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
			return nil, resources, err
		}
		log.G(ctx).Debug("hcsshim::CreateContainer creating container document")
		v1, v2, err := createWindowsContainerDocument(ctx, coi)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
			return nil, resources, err
		}

		if schemaversion.IsV10(coi.actualSchemaVersion) {
			// v1 Argon or Xenon. Pass the document directly to HCS.
			hcsDocument = v1
		} else if coi.HostingSystem != nil {
			// v2 Xenon. Pass the container object to the UVM.
			gcsDocument = &hcsschema.HostedSystem{
				SchemaVersion: schemaversion.SchemaV21(),
				Container:     v2,
			}
		} else {
			// v2 Argon. Pass the container object to the HCS.
			hcsDocument = &hcsschema.ComputeSystem{
				Owner:                             coi.actualOwner,
				SchemaVersion:                     schemaversion.SchemaV21(),
				ShouldTerminateOnLastHandleClosed: true,
				Container:                         v2,
			}
		}
	}

	log.G(ctx).Debug("hcsshim::CreateContainer creating compute system")
	if gcsDocument != nil {
		c, err := coi.HostingSystem.CreateContainer(ctx, coi.actualID, gcsDocument)
		if err != nil {
			return nil, resources, err
		}
		return c, resources, nil
	}

	system, err := hcs.CreateComputeSystem(ctx, coi.actualID, hcsDocument)
	if err != nil {
		return nil, resources, err
	}
	return system, resources, nil
}

// CloneContainer is similar to CreateContainer but it does not add layers or namespace like
// CreateContainer does. Also, instead of sending create container request it sends a modify
// request to an existing container. CloneContainer only works for WCOW.
func CloneContainer(ctx context.Context, createOptions *CreateOptions) (_ cow.Container, _ *Resources, err error) {
	coi, err := initializeCreateOptions(ctx, createOptions)
	if err != nil {
		return nil, nil, err
	}

	if coi.Spec.Windows == nil || coi.HostingSystem == nil {
		return nil, nil, fmt.Errorf("CloneContainer is only supported for Hyper-v isolated WCOW ")
	}

	resources := &Resources{
		id: createOptions.ID,
	}
	defer func() {
		if err != nil {
			if !coi.DoNotReleaseResourcesOnFailure {
				ReleaseResources(ctx, resources, coi.HostingSystem, true)
			}
		}
	}()

	if coi.HostingSystem != nil {
		n := coi.HostingSystem.ContainerCounter()
		if coi.Spec.Linux != nil {
			resources.containerRootInUVM = fmt.Sprintf(lcowRootInUVM, createOptions.ID)
		} else {
			resources.containerRootInUVM = fmt.Sprintf(wcowRootInUVM, strconv.FormatUint(n, 16))
		}
	}

	if err = setupMounts(ctx, coi, resources); err != nil {
		return nil, resources, err
	}

	// everything that is added to the container during the createContainer request
	// (via the gcsDocument) must be hot added here.  Add the mounts as mapped
	// directories or mapped pipes. In case of cloned container we must add them as a
	// modify request.
	_, _, mdsv2, mpsv2, err := createMountsConfig(ctx, coi)
	if err != nil {
		return nil, resources, err
	}

	c, err := coi.HostingSystem.CloneContainer(ctx, coi.actualID, nil)
	if err != nil {
		return nil, resources, err
	}

	// send modify requests for mounts one by one.
	// TODO(ambarve) : Find out if there is a way to send request for all the mounts
	// at the same time to save time
	for _, md := range mdsv2 {
		requestDocument := &hcsschema.ModifySettingRequest{
			RequestType:  requesttype.Add,
			ResourcePath: "Container/MappedDirectories",
			Settings:     md,
		}
		err := c.Modify(ctx, requestDocument)
		if err != nil {
			return c, resources, fmt.Errorf("Error while adding mapped directory (%s) to the container: %s", md.HostPath, err)
		}
	}

	for _, mp := range mpsv2 {
		requestDocument := &hcsschema.ModifySettingRequest{
			RequestType:  requesttype.Add,
			ResourcePath: "Container/MappedPipes",
			Settings:     mp,
		}
		err := c.Modify(ctx, requestDocument)
		if err != nil {
			return c, resources, fmt.Errorf("Error while adding mapped pipe (%s) to the container: %s", mp.HostPath, err)
		}
	}

	return c, resources, nil
}
