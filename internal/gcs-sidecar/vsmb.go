//go:build windows

package bridge

import (
	"time"
	"unsafe"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	GlobalRdrDeviceName                   = `\\?\GLOBALROOT\Device\LanmanRedirector`
	GlobalVsmbDeviceName                  = `\\?\GLOBALROOT\Device\vmsmb`
	GlobalVsmbInstanceName                = `\Device\vmsmb`
	GlobalVsmbTransportName               = `\Device\VMBus\{4d12e519-17a0-4ae4-8eaa-5270fc6abdb7}-{dcc079ae-60ba-4d07-847c-3493609c0870}-0000`
	SeLoadDriverName                      = "SeLoadDriverPrivilege"
	FsctlLmrStartInstance                 = 0x001403A0
	FsctlLmrBindToTransport               = 0x001401B0
	LmrInstanceFlagRegisterFilesystem     = 0x2
	LmrInstanceFlagUseCustomTransports    = 0x4
	LmrInstanceFlagAllowGuestAuth         = 0x8
	LmrInstanceFlagSupportsDirectmappedIo = 0x10
	SmbCeTransportTypeVmbus               = 3
)

type IOStatusBlock struct {
	Status      uintptr
	Information uintptr
}

func configureAndStartLanmanWorkstation() error {
	m, err := mgr.Connect()
	if err != nil {
		logrus.Errorf("Failed to connect to Service Manager: %v", err)
		return err
	}
	defer func() {
		if m != nil {
			if derr := m.Disconnect(); derr != nil {
				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
			}
		}
	}()

	s, err := m.OpenService("LanmanWorkstation")
	if err != nil {
		logrus.Errorf("Failed to open LanmanWorkstation service: %v", err)
		return err
	}
	defer func() {
		if s != nil {
			if derr := s.Close(); derr != nil {
				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
			}
		}
	}()

	cfg, err := s.Config()
	if err != nil {
		logrus.Errorf("retrieve LanmanWorkstation service config: %v", err)
		return err
	}
	cfg.StartType = mgr.StartAutomatic
	if err = s.UpdateConfig(cfg); err != nil {
		logrus.Errorf("update LanmanWorkstation service confg: %v", err)
		return err
	}
	return s.Start()
}

// Structs
type SMB2InstanceConfiguration struct {
	DormantDirectoryTimeout             uint32
	DormantFileTimeout                  uint32
	DormantFileLimit                    uint32
	FileInfoCacheLifetime               uint32
	FileNotFoundCacheLifetime           uint32
	DirectoryCacheLifetime              uint32
	FileInfoCacheEntriesMax             uint32
	FileNotFoundCacheEntriesMax         uint32
	DirectoryCacheEntriesMax            uint32
	DirectoryCacheSizeMax               uint32
	ReadAheadGranularity                uint32
	VolumeFeatureSupportCacheLifetime   uint32
	VolumeFeatureSupportCacheEntriesMax uint32
	FileAbeStatusCacheLifetime          uint32
	RequireSecuritySignature            byte
	RequireEncryption                   byte
	Padding                             [2]byte
}

type LMRConnectionProperties struct {
	Flags1                          byte
	Flags2                          byte
	Padding                         [2]byte
	SessionTimeoutInterval          uint32
	CAHandleKeepaliveInterval       uint32
	NonCAHandleKeepaliveInterval    uint32
	ActiveIOKeepaliveInterval       uint32
	DisableRdma                     uint32
	ConnectionCountPerRdmaInterface uint32
	AlternateTCPPort                uint16
	AlternateQuicPort               uint16
	AlternateRdmaPort               uint16
	Padding2                        [2]byte
}

type LMRStartInstanceRequest struct {
	StructureSize               uint32
	IoTimeout                   uint32
	IoRetryCount                uint32
	Flags                       uint16
	Padding1                    uint16
	Reserved1                   uint32
	InstanceConfig              SMB2InstanceConfiguration
	DefaultConnectionProperties LMRConnectionProperties
	InstanceID                  byte
	Reserved2                   byte
	DeviceNameLength            uint16
}

type LMRBindUnbindTransportRequest struct {
	StructureSize     uint16
	Flags             uint16
	Type              uint32
	TransportIDLength uint32
}

func isLanmanWorkstationRunning() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer func() {
		if m != nil {
			if derr := m.Disconnect(); derr != nil {
				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
			}
		}
	}()

	s, err := m.OpenService("LanmanWorkstation")
	if err != nil {
		return false, err
	}
	defer func() {
		if s != nil {
			if derr := s.Close(); derr != nil {
				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
			}
		}
	}()

	status, err := s.Query()
	if err != nil {
		return false, err
	}

	// Check if the service state is running
	return status.State == svc.Running, nil
}

func VsmbMain() {
	logrus.Info("Starting VSMB initialization...")

	logrus.Debug("Configuring LanmanWorkstation service...")
	if err := configureAndStartLanmanWorkstation(); err != nil {
		logrus.Errorf("LanmanWorkstation setup failed: %v", err)
		return
	}

	time.Sleep(3 * time.Second) // TODO: This needs to be better logic.
	running, err := isLanmanWorkstationRunning()
	if err != nil {
		logrus.Errorf("Failed to query LanmanWorkstation status: %v", err)
	} else if running {
		logrus.Info("LanmanWorkstation service is running.")
	} else {
		logrus.Warn("LanmanWorkstation service is NOT running.")
	}

	if err := winio.EnableProcessPrivileges([]string{SeLoadDriverName}); err != nil {
		logrus.Errorf("Failed to enable privilege: %v", err)
		return
	}
	// Open LanmanRedirector
	namePtr, nerr := windows.UTF16PtrFromString(GlobalRdrDeviceName)
	if nerr != nil {
		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalRdrDeviceName)
		return
	}

	lmrHandle, err := windows.CreateFile(
		namePtr,
		windows.SYNCHRONIZE|windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to open redirector")
		return
	}
	defer func() {
		if derr := windows.CloseHandle(lmrHandle); derr != nil {
			logrus.WithError(derr).Warn("Failed to close LanmanRedirector handle")
		}
	}()

	logrus.Info("Successfully opened LanmanRedirector device.")

	// Build StartInstance buffer
	instanceNameUTF16, nerr := windows.UTF16FromString(GlobalVsmbInstanceName)
	if nerr != nil {
		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbInstanceName)
		return
	}
	structSize := int(unsafe.Sizeof(LMRStartInstanceRequest{}))
	bufferSize := structSize + (len(instanceNameUTF16)-1)*2
	buffer := make([]byte, bufferSize)

	startReq := LMRStartInstanceRequest{
		StructureSize: uint32(structSize),
		IoTimeout:     30,
		IoRetryCount:  3,
		Flags: LmrInstanceFlagRegisterFilesystem |
			LmrInstanceFlagUseCustomTransports |
			LmrInstanceFlagAllowGuestAuth |
			LmrInstanceFlagSupportsDirectmappedIo,
		InstanceID:       1,
		DeviceNameLength: uint16((len(instanceNameUTF16) - 1) * 2),
	}

	startReq.Reserved1 = 0
	startReq.InstanceConfig = SMB2InstanceConfiguration{}
	startReq.DefaultConnectionProperties = LMRConnectionProperties{}
	startReq.DefaultConnectionProperties.Flags1 = 0x1F
	startReq.DefaultConnectionProperties.SessionTimeoutInterval = 55
	startReq.DefaultConnectionProperties.CAHandleKeepaliveInterval = 10
	startReq.DefaultConnectionProperties.NonCAHandleKeepaliveInterval = 30
	startReq.DefaultConnectionProperties.ActiveIOKeepaliveInterval = 30

	copy(buffer[:structSize], (*[1 << 20]byte)(unsafe.Pointer(&startReq))[:structSize])
	copy(buffer[structSize:], (*[1 << 20]byte)(unsafe.Pointer(&instanceNameUTF16[0]))[:(len(instanceNameUTF16)-1)*2])

	// lmrHandle is a windows.Handle from windows.CreateFile(...)
	var iosb winapi.IOStatusBlock
	status := winapi.NtFsControlFile(
		lmrHandle,             // file
		0,                     // event (none → synchronous)
		0,                     // apcRoutine (none)
		0,                     // apcCtx
		&iosb,                 // IO_STATUS_BLOCK
		FsctlLmrStartInstance, // FSCTL
		buffer,                // input buffer
		nil,                   // output buffer
	)
	switch status {
	case 0:
		logrus.Info("VMSMB RDR instance started.")
	case 0xC0000035:
		logrus.Warn("VMSMB RDR instance already started.")
	default:
		logrus.Errorf("NtFsControlFile failed: 0x%08X", status)
	}

	// BindTransport
	namePtr, nerr = windows.UTF16PtrFromString(GlobalVsmbDeviceName)
	if nerr != nil {
		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalVsmbDeviceName)
		return
	}
	vmsmbHandle, err := windows.CreateFile(
		namePtr,
		windows.SYNCHRONIZE|windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0,
	)
	if err != nil {
		logrus.Errorf("Failed to open VMSMB device: %v", err)
		return
	}
	defer func() {
		if derr := windows.CloseHandle(vmsmbHandle); derr != nil {
			logrus.WithError(derr).Warn("Failed to close VSMB device handle")
		}
	}()

	transportNameUTF16, nerr := windows.UTF16FromString(GlobalVsmbTransportName)
	if nerr != nil {
		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbTransportName)
		return
	}

	bindStructSize := int(unsafe.Sizeof(LMRBindUnbindTransportRequest{}))
	bindBufferSize := bindStructSize + (len(transportNameUTF16)-1)*2
	bindBuffer := make([]byte, bindBufferSize)

	bindReq := LMRBindUnbindTransportRequest{
		StructureSize:     uint16(bindStructSize) + 4,
		Flags:             0,
		Type:              2,
		TransportIDLength: uint32((len(transportNameUTF16) - 1) * 2),
	}

	copy(bindBuffer[:bindStructSize], (*[1 << 20]byte)(unsafe.Pointer(&bindReq))[:bindStructSize])
	copy(bindBuffer[bindStructSize:], (*[1 << 20]byte)(unsafe.Pointer(&transportNameUTF16[0]))[:(len(transportNameUTF16)-1)*2])

	status = winapi.NtFsControlFile(
		vmsmbHandle,             // windows.Handle from windows.CreateFile
		0,                       // event (0 → synchronous)
		0,                       // apcRoutine
		0,                       // apcCtx
		&iosb,                   // IO_STATUS_BLOCK
		FsctlLmrBindToTransport, // FSCTL
		bindBuffer,              // in
		nil,                     // out
	)
	if status == 0 {
		logrus.Info("VMBUS transport bound to VMSMB RDR instance.")
	} else {
		logrus.Errorf("NtFsControlFile failed: 0x%08X", status)
	}
}
