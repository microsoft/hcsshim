//go:build linux
// +build linux

// Package cgroup provides a unified interface for cgroup v1 and v2 operations.
package cgroup

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	cgroups "github.com/containerd/cgroups/v3"
	cgroups1 "github.com/containerd/cgroups/v3/cgroup1"
	cgroups1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroups2 "github.com/containerd/cgroups/v3/cgroup2"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// defaultMemoryThreshold is used when no valid threshold is provided to RegisterMemoryEvent.
const defaultMemoryThreshold = 50 * 1024 * 1024 // 50 MB

// IsCgroupV2 checks if cgroup v2 (unified or hybrid) is available on the system.
func IsCgroupV2() bool {
	mode := cgroups.Mode()
	return mode == cgroups.Unified || mode == cgroups.Hybrid
}

// Manager provides a unified interface for cgroup v1 and v2 operations.
type Manager interface {
	Create(pid int) error
	Delete() error
	// Stats returns cgroup metrics in v1 format for wire protocol compatibility.
	// On cgroup v2 systems, native v2 stats are converted via ConvertV2StatsToV1.
	// TODO: Add StatsV2() method returning native *cgroups2stats.Metrics when
	// host-side consumers (HCS schema, GCS protocol, containerd shim) are updated
	// to handle the v2 wire format.
	Stats() (*cgroups1stats.Metrics, error)
	Update(resources *oci.LinuxResources) error
	AddTask(pid int) error
	Add(process cgroups1.Process, names ...cgroups1.Name) error
	RegisterMemoryEvent(event cgroups1.MemoryEvent) (uintptr, error)
	OOMEventFD() (uintptr, error)
	// GetV1Cgroup returns the underlying v1 cgroup if available, nil for v2.
	GetV1Cgroup() cgroups1.Cgroup
	// GetV2Manager returns the underlying v2 manager if available, nil for v1.
	GetV2Manager() *cgroups2.Manager
}

// NewManager creates the appropriate cgroup manager based on the detected system version.
func NewManager(path string, resources *oci.LinuxResources) (Manager, error) {
	if IsCgroupV2() {
		logrus.Info("Creating cgroup v2 manager for path: " + path)
		v2Resources := ConvertToV2Resources(resources)
		mgr, err := cgroups2.NewManager("/sys/fs/cgroup", path, v2Resources)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create cgroup v2 manager")
		}
		return &V2Mgr{mgr: mgr, path: path, done: make(chan struct{})}, nil
	}

	logrus.Info("Creating cgroup v1 manager for path: " + path)
	cg, err := cgroups1.New(cgroups1.StaticPath(path), resources)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cgroup v1 manager")
	}
	return &V1Mgr{cg: cg}, nil
}

// V1Mgr wraps cgroup v1 operations.
type V1Mgr struct {
	cg cgroups1.Cgroup
}

func (v *V1Mgr) Create(pid int) error {
	return v.cg.Add(cgroups1.Process{Pid: pid})
}

func (v *V1Mgr) Stats() (*cgroups1stats.Metrics, error) {
	return v.cg.Stat(cgroups1.IgnoreNotExist)
}

func (v *V1Mgr) Update(resources *oci.LinuxResources) error {
	return v.cg.Update(resources)
}

func (v *V1Mgr) AddTask(pid int) error {
	return v.cg.AddTask(cgroups1.Process{Pid: pid})
}

func (v *V1Mgr) Add(process cgroups1.Process, names ...cgroups1.Name) error {
	return v.cg.Add(process, names...)
}

func (v *V1Mgr) Delete() error {
	return v.cg.Delete()
}

func (v *V1Mgr) RegisterMemoryEvent(event cgroups1.MemoryEvent) (uintptr, error) {
	return v.cg.RegisterMemoryEvent(event)
}

func (v *V1Mgr) OOMEventFD() (uintptr, error) {
	return v.cg.OOMEventFD()
}

func (v *V1Mgr) GetV1Cgroup() cgroups1.Cgroup {
	return v.cg
}

func (v *V1Mgr) GetV2Manager() *cgroups2.Manager {
	return nil
}

// V2Mgr wraps cgroup v2 operations.
type V2Mgr struct {
	mgr       *cgroups2.Manager
	path      string
	done      chan struct{} // closed by Delete() to stop polling goroutines
	closeOnce sync.Once
}

func (v *V2Mgr) Create(pid int) error {
	if v.mgr == nil {
		return errors.Errorf("cgroup v2 manager not initialized for path %s", v.path)
	}
	return v.mgr.AddProc(uint64(pid))
}

func (v *V2Mgr) Stats() (*cgroups1stats.Metrics, error) {
	v2Stats, err := v.mgr.Stat()
	if err != nil {
		return nil, err
	}
	return ConvertV2StatsToV1(v2Stats), nil
}

func (v *V2Mgr) Update(resources *oci.LinuxResources) error {
	v2Resources := ConvertToV2Resources(resources)
	return v.mgr.Update(v2Resources)
}

func (v *V2Mgr) AddTask(pid int) error {
	return v.mgr.AddProc(uint64(pid))
}

func (v *V2Mgr) Add(process cgroups1.Process, _ ...cgroups1.Name) error {
	return v.mgr.AddProc(uint64(process.Pid))
}

func (v *V2Mgr) Delete() error {
	v.closeOnce.Do(func() {
		close(v.done)
	})
	return v.mgr.Delete()
}

func (v *V2Mgr) RegisterMemoryEvent(event cgroups1.MemoryEvent) (uintptr, error) {
	fullPath := filepath.Join("/sys/fs/cgroup", v.path)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return 0, errors.Wrapf(err, "cgroup path does not exist: %s", v.path)
	}

	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		return 0, errors.Wrap(err, "failed to create eventfd for v2 memory event")
	}

	var threshold uint64
	if event != nil {
		thresholdStr := event.Arg()
		var err error
		threshold, err = strconv.ParseUint(thresholdStr, 10, 64)
		if err != nil {
			threshold = defaultMemoryThreshold
			logrus.WithError(err).WithField("arg", thresholdStr).Warn("Failed to parse threshold from event, using default")
		}
	} else {
		threshold = defaultMemoryThreshold
	}

	go func() {
		// Note: the caller owns the fd lifetime (via os.NewFile + Close).
		// Do not close fd here — it would close the raw fd under the caller's os.File.
		var lastMemoryUsage uint64
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		logrus.WithFields(logrus.Fields{
			"cgroup_version": "v2",
			"path":           v.path,
			"threshold":      threshold,
		}).Info("Started cgroup v2 memory threshold monitoring")

		for {
			select {
			case <-v.done:
				logrus.WithField("path", v.path).Debug("stopping v2 memory threshold monitor")
				return
			case <-ticker.C:
			}

			currentUsage, err := readMemoryCurrentV2(v.path)
			if err != nil {
				logrus.WithError(err).WithField("path", v.path).Debug("failed to read memory.current")
				continue
			}

			if currentUsage > threshold && currentUsage > lastMemoryUsage {
				if _, err := unix.Write(fd, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
					logrus.WithError(err).Debug("failed to write to eventfd")
				} else {
					logrus.WithFields(logrus.Fields{
						"current_usage": currentUsage,
						"threshold":     threshold,
						"path":          v.path,
					}).Info("cgroup v2 memory threshold crossed, eventfd notified")
				}
			}
			lastMemoryUsage = currentUsage
		}
	}()

	return uintptr(fd), nil
}

func (v *V2Mgr) OOMEventFD() (uintptr, error) {
	eventsPath := filepath.Join("/sys/fs/cgroup", v.path, "memory.events")
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		return 0, errors.Wrapf(err, "cgroup memory.events does not exist: %s", v.path)
	}

	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		return 0, errors.Wrap(err, "failed to create eventfd for v2 OOM event")
	}

	go func() {
		// Note: the caller owns the fd lifetime (via os.NewFile + Close).
		// Do not close fd here — it would close the raw fd under the caller's os.File.
		var lastOOMKillCount uint64
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		logrus.WithFields(logrus.Fields{
			"cgroup_version": "v2",
			"path":           v.path,
		}).Info("Started cgroup v2 OOM monitoring")

		for {
			select {
			case <-v.done:
				logrus.WithField("path", v.path).Debug("stopping v2 OOM monitor")
				return
			case <-ticker.C:
			}

			data, err := os.ReadFile(eventsPath)
			if err != nil {
				logrus.WithError(err).WithField("path", v.path).Debug("failed to read memory.events")
				continue
			}

			events := ParseMemoryEvents(string(data))
			oomKillCount := events["oom_kill"]

			if oomKillCount > lastOOMKillCount {
				if _, err := unix.Write(fd, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
					logrus.WithError(err).Debug("failed to write to OOM eventfd")
				} else {
					logrus.WithFields(logrus.Fields{
						"oom_kill_count": oomKillCount,
						"path":           v.path,
					}).Warn("cgroup v2 OOM kill detected, eventfd notified")
				}
				lastOOMKillCount = oomKillCount
			}
		}
	}()

	return uintptr(fd), nil
}

func (v *V2Mgr) GetV1Cgroup() cgroups1.Cgroup {
	return nil
}

func (v *V2Mgr) GetV2Manager() *cgroups2.Manager {
	return v.mgr
}

// ParseMemoryEvents parses cgroup v2 memory.events file.
// Format: "low 0\nhigh 5\nmax 0\noom 0\noom_kill 0\noom_group_kill 0\n"
func ParseMemoryEvents(content string) map[string]uint64 {
	events := make(map[string]uint64)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			if val, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
				events[parts[0]] = val
			}
		}
	}
	return events
}

func readMemoryCurrentV2(cgroupPath string) (uint64, error) {
	filePath := filepath.Join("/sys/fs/cgroup", cgroupPath, "memory.current")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// LoadAndStat loads an existing cgroup by path and returns its stats.
// This is useful for containers whose cgroups are managed by the runtime (e.g., runc)
// rather than created by us directly.
func LoadAndStat(cgroupPath string) (*cgroups1stats.Metrics, error) {
	if IsCgroupV2() {
		mgr, err := cgroups2.Load(cgroupPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load v2 cgroup %s", cgroupPath)
		}
		v2Stats, err := mgr.Stat()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to stat v2 cgroup %s", cgroupPath)
		}
		return ConvertV2StatsToV1(v2Stats), nil
	}

	cg, err := cgroups1.Load(cgroups1.StaticPath(cgroupPath))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load v1 cgroup %s", cgroupPath)
	}
	return cg.Stat(cgroups1.IgnoreNotExist)
}

// Compile-time interface satisfaction checks.
var (
	_ Manager = &V1Mgr{}
	_ Manager = &V2Mgr{}
)
