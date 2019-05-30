// +build linux

package network

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// maxDNSSearches is limited to 6 in `man 5 resolv.conf`
const maxDNSSearches = 6

// GenerateResolvConfContent generates the resolv.conf file content based on
// `searches`, `servers`, and `options`.
func GenerateResolvConfContent(ctx context.Context, searches, servers, options []string) (string, error) {
	if len(searches) > maxDNSSearches {
		return "", errors.Errorf("searches has more than %d domains", maxDNSSearches)
	}

	content := ""
	if len(searches) > 0 {
		content += fmt.Sprintf("search %s\n", strings.Join(searches, " "))
	}
	if len(servers) > 0 {
		content += fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
	}
	if len(options) > 0 {
		content += fmt.Sprintf("options %s\n", strings.Join(options, " "))
	}
	return content, nil
}

// MergeValues merges `first` and `second` maintaining order `first, second`.
func MergeValues(first, second []string) []string {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}
	values := make([]string, len(first), len(first)+len(second))
	copy(values, first)
	for _, v := range second {
		found := false
		for i := 0; i < len(values); i++ {
			if v == values[i] {
				found = true
				break
			}
		}
		if !found {
			values = append(values, v)
		}
	}
	return values
}

// GenerateResolvConfFile parses `dnsServerList` and `dnsSuffix` and writes the
// `nameserver` and `search` entries to `resolvPath`.
func GenerateResolvConfFile(resolvPath, dnsServerList, dnsSuffix string) (err error) {
	activity := "network::GenerateResolvConfFile"
	log := logrus.WithFields(logrus.Fields{
		"resolvPath":    resolvPath,
		"dnsServerList": dnsServerList,
		"dnsSuffix":     dnsSuffix,
	})
	log.Debug(activity + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(activity + " - End Operation")
		} else {
			log.Debug(activity + " - End Operation")
		}
	}()

	fileContents := ""

	split := func(r rune) bool {
		return r == ',' || r == ' '
	}

	nameservers := strings.FieldsFunc(dnsServerList, split)
	for i, server := range nameservers {
		// Limit number of nameservers to 3.
		if i >= 3 {
			break
		}

		fileContents += fmt.Sprintf("nameserver %s\n", server)
	}

	if dnsSuffix != "" {
		fileContents += fmt.Sprintf("search %s\n", dnsSuffix)
	}

	file, err := os.OpenFile(resolvPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create resolv.conf")
	}
	defer file.Close()
	if _, err := io.WriteString(file, fileContents); err != nil {
		return errors.Wrapf(err, "failed to write to resolv.conf")
	}
	return nil
}

// InstanceIDToName converts from the given instance ID (a GUID generated on the
// Windows host) to its corresponding interface name (e.g. "eth0").
func InstanceIDToName(id string, wait bool) (ifname string, err error) {
	id = strings.ToLower(id)

	activity := "network::InstanceIDToName"
	log := logrus.WithFields(logrus.Fields{
		"adapterInstanceID": id,
		"wait":              wait,
	})
	log.Debug(activity + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(activity + " - End Operation")
		} else {
			log.Data["ifname"] = ifname
			log.Debug(activity + " - End Operation")
		}
	}()

	const timeout = 2 * time.Second
	var deviceDirs []os.FileInfo
	start := time.Now()
	for {
		deviceDirs, err = ioutil.ReadDir(filepath.Join("/sys", "bus", "vmbus", "devices", id, "net"))
		if err != nil {
			if wait {
				if os.IsNotExist(errors.Cause(err)) {
					time.Sleep(10 * time.Millisecond)
					if time.Since(start) > timeout {
						return "", errors.Wrapf(err, "timed out waiting for net adapter after %d seconds", timeout)
					}
					continue
				}
			}
			return "", errors.Wrapf(err, "failed to read vmbus network device from /sys filesystem for adapter %s", id)
		}
		break
	}
	if len(deviceDirs) == 0 {
		return "", errors.Errorf("no interface name found for adapter %s", id)
	}
	if len(deviceDirs) > 1 {
		return "", errors.Errorf("multiple interface names found for adapter %s", id)
	}
	ifname = deviceDirs[0].Name()
	return ifname, nil
}
