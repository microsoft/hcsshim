package cri_containerd

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
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
