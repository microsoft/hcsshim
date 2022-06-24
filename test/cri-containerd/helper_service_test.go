//go:build windows && functional

package cri_containerd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	ctrdconfig "github.com/containerd/containerd/services/server/config"
	toml "github.com/pelletier/go-toml"
)

// Implements functionality so tests can start/stop the containerd service.
// Tests assume containerd will be running when they start, since this
// matches with the state a dev box will usually be in. A test that stops containerd should
// therefore ensure it starts it again.

var (
	svcMgr            *mgr.Mgr
	svcMgrConnectOnce sync.Once
	svcMgrConnectErr  error
)

const tomlPath = `C:\containerplat\containerd.toml`

func getSvcMgr() (*mgr.Mgr, error) {
	svcMgrConnectOnce.Do(func() {
		s, err := mgr.Connect()
		if err != nil {
			err = fmt.Errorf("failed to connect to service manager: %w", err)
		}
		svcMgr, svcMgrConnectErr = s, err
	})
	return svcMgr, svcMgrConnectErr
}

func startService(serviceName string) error {
	m, err := getSvcMgr()
	if err != nil {
		return err
	}
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service %s: %w", serviceName, err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service %s: %w", serviceName, err)
	}
	return nil
}

func stopService(serviceName string) error {
	m, err := getSvcMgr()
	if err != nil {
		return err
	}
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service %s: %w", serviceName, err)
	}
	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to send stop control to service %s: %w", serviceName, err)
	}
	tc := time.NewTimer(10 * time.Second)
	defer tc.Stop()
	for status.State != svc.Stopped {
		time.Sleep(1 * time.Second)
		select {
		case <-tc.C:
			return fmt.Errorf("service %s did not stop in time", serviceName)
		default:
			status, err = s.Query()
			if err != nil {
				return fmt.Errorf("failed to query service %s status: %w", serviceName, err)
			}
		}
	}
	return nil
}

func startContainerd(t *testing.T) {
	if err := startService(*flagContainerdServiceName); err != nil {
		t.Fatal(err)
	}
}

func stopContainerd(t *testing.T) {
	if err := stopService(*flagContainerdServiceName); err != nil {
		t.Fatal(err)
	}
}

func loadContainerdConfigFile(path string) (*ctrdconfig.Config, error) {
	config := &ctrdconfig.Config{}

	file, err := toml.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load TOML: %s: %w", path, err)
	}

	if err := file.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TOML: %w", err)
	}

	return config, nil
}

func writeContainerdConfigToFile(path string, config *ctrdconfig.Config) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %s already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to state file %s: %w", path, err)
	}

	bcfg, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err = ioutil.WriteFile(path, bcfg, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// containerdManager manages the lifecycle of containerd during test execution
type containerdManager struct {
	t                *testing.T
	containerdConfig *ctrdconfig.Config
	backupTomlPath   string
}

func NewContainerdManager(t *testing.T, cfg *ctrdconfig.Config) *containerdManager {
	return &containerdManager{
		t:                t,
		containerdConfig: cfg,
	}
}

// init stops any existing containerd service, saves the existing containerd config, replaces the existing containerd config with m.containerdConfig and starts containerd service with that config.
func (m *containerdManager) init() {
	// First stop containerd, update content & snapshot sharing policy and then restart containerd.
	stopContainerd(m.t)

	tempDir := m.t.TempDir()
	m.backupTomlPath = filepath.Join(tempDir, "containerd.toml")
	if err := copyfile.CopyFile(context.Background(), tomlPath, m.backupTomlPath, true); err != nil {
		m.t.Fatalf("failed to create backup copy of containerd")
	}

	os.Remove(tomlPath)
	if err := writeContainerdConfigToFile(tomlPath, m.containerdConfig); err != nil {
		m.t.Fatalf("failed to write config: %s", err)
	}

	startContainerd(m.t)
}

func (m *containerdManager) cleanup() {
	stopContainerd(m.t)
	if err := copyfile.CopyFile(context.Background(), m.backupTomlPath, tomlPath, true); err != nil {
		m.t.Fatalf("failed to restore saved containerd config: %s", err)
	}
	startContainerd(m.t)
}
