package gcs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// configureAdapterInNamespace moves a given adapter into a network
// namespace and configures it there.
func (c *gcsCore) configureAdapterInNamespace(container runtime.Container, adapter prot.NetworkAdapter) error {
	id := adapter.AdapterInstanceID
	interfaceName, err := c.instanceIDToName(id)
	if err != nil {
		return err
	}
	nspid := container.Pid()
	cfg, err := json.Marshal(adapter)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal adapter struct to JSON for adapter %s", id)
	}

	out, err := c.OS.Command("netnscfg",
		"-if", interfaceName,
		"-nspid", fmt.Sprintf("%d", nspid),
		"-cfg", string(cfg)).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to configure network adapter %s: %s", adapter.AdapterInstanceID, out)
	}
	logrus.Debugf("netnscfg output:\n%s", out)

	// Handle resolve.conf
	// There is no need to create <baseFilesPath>/etc here as it
	// is created in CreateContainer().
	resolvPath := filepath.Join(baseFilesPath, "etc/resolv.conf")

	if adapter.NatEnabled {
		// Set the DNS configuration.
		if err := c.generateResolvConfFile(resolvPath, adapter); err != nil {
			return errors.Wrapf(err, "failed to generate resolv.conf file for adapter %s", adapter.AdapterInstanceID)
		}
	} else {
		exists, err := c.OS.PathExists(resolvPath)
		if err != nil {
			return errors.Wrapf(err, "failed to check if resolv.conf path already exists for adapter %s", adapter.AdapterInstanceID)
		}
		if !exists {
			if err := c.OS.Link("/etc/resolv.conf", resolvPath); err != nil {
				return errors.Wrapf(err, "failed to link resolv.conf file for adapter %s", adapter.AdapterInstanceID)
			}
		}

	}
	return nil
}

// generateResolvConfFile generate a resolve.conf file in $baseFilesPath/etc
// for the given adapter.
// TODO: This method of managing DNS will potentially be replaced with another
// method in the future.
func (c *gcsCore) generateResolvConfFile(resolvPath string, adapter prot.NetworkAdapter) error {
	fileContents := ""

	split := func(r rune) bool {
		return r == ',' || r == ' '
	}

	nameservers := strings.FieldsFunc(adapter.HostDNSServerList, split)
	for i, server := range nameservers {
		// Limit number of nameservers to 3.
		if i >= 3 {
			break
		}

		fileContents += fmt.Sprintf("nameserver %s\n", server)
	}

	if adapter.HostDNSSuffix != "" {
		fileContents += fmt.Sprintf("search %s\n", adapter.HostDNSSuffix)
	}

	file, err := c.OS.OpenFile(resolvPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to create resolv.conf file for adapter %s", adapter.AdapterInstanceID)
	}
	defer file.Close()
	if _, err := io.WriteString(file, fileContents); err != nil {
		return errors.Wrapf(err, "failed to write to resolv.conf file for adapter %s", adapter.AdapterInstanceID)
	}
	logrus.Debugf("wrote %s:\n%s", resolvPath, fileContents)
	return nil
}

// instanceIDToName converts from the given instance ID (a GUID generated on
// the Windows host) to its corresponding interface name (e.g. "eth0").
func (c *gcsCore) instanceIDToName(id string) (string, error) {
	deviceDirs, err := c.OS.ReadDir(filepath.Join("/sys", "bus", "vmbus", "devices", id, "net"))
	if err != nil {
		return "", errors.Wrapf(err, "failed to read vmbus network device from /sys filesystem for adapter %s", id)
	}
	if len(deviceDirs) == 0 {
		return "", errors.Errorf("no interface name found for adapter %s", id)
	}
	if len(deviceDirs) > 1 {
		return "", errors.Errorf("multiple interface names found for adapter %s", id)
	}
	return deviceDirs[0].Name(), nil
}
