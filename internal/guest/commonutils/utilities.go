package commonutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UnmarshalJSONWithHresult unmarshals the given data into the given interface, and
// wraps any error returned in an HRESULT error.
func UnmarshalJSONWithHresult(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}

// DecodeJSONWithHresult decodes the JSON from the given reader into the given
// interface, and wraps any error returned in an HRESULT error.
func DecodeJSONWithHresult(r io.Reader, v interface{}) error {
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}

// StartTimeSyncService starts the `chronyd` deamon to keep the UVM time synchronized.  We
// use a PTP device provided by the hypervisor as a source of correct time (instead of
// using a network server). We need to create a configuration file that configures chronyd
// to use the PTP device.  The system can have multiple PTP devices so we identify the
// correct PTP device by verifying that the `clock_name` of that device is `hyperv`.
func StartTimeSyncService() error {
	ptpClassDir, err := os.Open("/sys/class/ptp")
	if err != nil {
		return errors.Wrap(err, "failed to open PTP class directory")
	}

	ptpDirList, err := ptpClassDir.Readdirnames(-1)
	if err != nil {
		return errors.Wrap(err, "failed to list PTP class directory")
	}

	var ptpDirPath string
	found := false
	for _, ptpDirPath = range ptpDirList {
		clockNameFilePath := filepath.Join(ptpClassDir.Name(), ptpDirPath, "clock_name")
		clockNameFile, err := os.Open(clockNameFilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to open clock name file at %s", clockNameFilePath)
		}
		// Expected clock name is `hyperv` so read first 6 chars and verify the
		// name
		expectedReadLen := len("hyperv")
		buf := make([]byte, expectedReadLen)
		fileReader := bufio.NewReader(clockNameFile)
		nread, err := fileReader.Read(buf)
		if err != nil {
			return errors.Wrapf(err, "read file %s failed", clockNameFilePath)
		} else if nread != expectedReadLen {
			return errors.Wrapf(err, "read file %s returned %d bytes, expected %d", clockNameFilePath, nread, expectedReadLen)
		}
		clockName := string(buf)
		if strings.EqualFold(clockName, "hyperv") {
			found = true
			break
		}
	}

	if !found {
		return errors.Errorf("no PTP device found with name \"hyperv\"")
	}

	// create chronyd config file
	ptpDevPath := filepath.Join("/dev", filepath.Base(ptpDirPath))
	chronydConfigString := fmt.Sprintf("refclock PHC %s poll 3 dpoll -2 offset 0\n", ptpDevPath)

	chronydConfPath := "/tmp/chronyd.conf"
	err = ioutil.WriteFile(chronydConfPath, []byte(chronydConfigString), 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to create chronyd conf file %s", chronydConfPath)
	}

	// start chronyd
	chronydCmd := exec.Command("chronyd", "-f", chronydConfPath)
	err = chronydCmd.Start()
	if err != nil {
		return errors.Wrap(err, "start chronyd command failed")
	}
	go func() {
		waitErr := chronydCmd.Wait()
		if waitErr != nil {
			logrus.WithError(waitErr).Warn("chronyd command exited with error")
		}
	}()
	return nil
}
