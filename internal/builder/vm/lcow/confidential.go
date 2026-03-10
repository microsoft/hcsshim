//go:build windows

package lcow

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// parseConfidentialOptions parses LCOW confidential options from annotations.
// This should only be called for confidential scenarios.
func parseConfidentialOptions(
	ctx context.Context,
	bundlePath string,
	opts *runhcsoptions.Options,
	annotations map[string]string,
	scsiCtrl map[string]hcsschema.Scsi,
	hvSocketConfig *hcsschema.HvSocketSystemConfig,
	securityPolicy string,
) (*hcsschema.Chipset, *hcsschema.SecuritySettings, *hcsschema.GuestState, []string, error) {

	log.G(ctx).Debug("parseConfidentialOptions: starting confidential options parsing")

	var filesToCleanOnError []string

	// Resolve boot files path for confidential mode
	bootFilesPath, err := resolveBootFilesPath(ctx, opts, annotations)
	if err != nil {
		return nil, nil, nil, filesToCleanOnError, fmt.Errorf("failed to resolve boot files path for confidential VM: %w", err)
	}

	// Set the default GuestState filename.
	// The kernel and minimal initrd are combined into a single vmgs file.
	guestStateFile := vmutils.DefaultGuestStateFile

	// Allow override from annotation
	if annotationGuestStateFile := oci.ParseAnnotationsString(annotations, shimannotations.LCOWGuestStateFile, ""); annotationGuestStateFile != "" {
		guestStateFile = annotationGuestStateFile
	}

	// Validate the VMGS template file exists and save the full path
	vmgsTemplatePath := filepath.Join(bootFilesPath, guestStateFile)
	if _, err := os.Stat(vmgsTemplatePath); os.IsNotExist(err) {
		return nil, nil, nil, filesToCleanOnError, fmt.Errorf("the GuestState vmgs file '%s' was not found", vmgsTemplatePath)
	}
	log.G(ctx).WithField("vmgsTemplatePath", vmgsTemplatePath).Debug("VMGS template path configured")

	// Set default DmVerity rootfs VHD.
	// The root file system comes from the dmverity vhd file which is mounted by the initrd in the vmgs file.
	dmVerityRootfsFile := vmutils.DefaultDmVerityRootfsVhd

	// Allow override from annotation
	if annotationDmVerityRootFsVhd := oci.ParseAnnotationsString(annotations, shimannotations.DmVerityRootFsVhd, ""); annotationDmVerityRootFsVhd != "" {
		dmVerityRootfsFile = annotationDmVerityRootFsVhd
	}

	// Validate the DmVerity rootfs VHD file exists and save the full path
	dmVerityRootfsTemplatePath := filepath.Join(bootFilesPath, dmVerityRootfsFile)
	if _, err := os.Stat(dmVerityRootfsTemplatePath); os.IsNotExist(err) {
		return nil, nil, nil, filesToCleanOnError, fmt.Errorf("the DM Verity VHD file '%s' was not found", dmVerityRootfsTemplatePath)
	}
	log.G(ctx).WithField("dmVerityRootfsPath", dmVerityRootfsTemplatePath).Debug("DM Verity rootfs path configured")

	// Note: VPMem and vPCI assigned devices are already disabled in parseDeviceOptions
	// when isConfidential is true.

	chipset := &hcsschema.Chipset{}
	// Required by HCS for the isolated boot scheme, see also https://docs.microsoft.com/en-us/windows-server/virtualization/hyper-v/learn-more/generation-2-virtual-machine-security-settings-for-hyper-v
	// A complete explanation of the why's and wherefores of starting an encrypted, isolated VM are beond the scope of these comments.
	log.G(ctx).Debug("configuring UEFI secure boot for confidential VM")
	chipset.Uefi = &hcsschema.Uefi{
		ApplySecureBootTemplate: "Apply",
		// aka MicrosoftWindowsSecureBootTemplateGUID equivalent to "Microsoft Windows" template from Get-VMHost | select SecureBootTemplates
		SecureBootTemplateId: "1734c6e8-3154-4dda-ba5f-a874cc483422",
	}

	// Part of the protocol to ensure that the rules in the user's Security Policy are
	// respected is to provide a hash of the policy to the hardware. This is immutable
	// and can be used to check that the policy used by opengcs is the required one as
	// a condition of releasing secrets to the container.
	log.G(ctx).Debug("creating security policy digest")
	policyDigest, err := securitypolicy.NewSecurityPolicyDigest(securityPolicy)
	if err != nil {
		return nil, nil, nil, filesToCleanOnError, fmt.Errorf("failed to create security policy digest: %w", err)
	}

	// HCS API expects a base64 encoded string as LaunchData. Internally it
	// decodes it to bytes. SEV later returns the decoded byte blob as HostData
	// field of the report.
	hostData := base64.StdEncoding.EncodeToString(policyDigest)

	// Put the measurement into the LaunchData field of the HCS creation command.
	// This will end-up in HOST_DATA of SNP_LAUNCH_FINISH command and the ATTESTATION_REPORT
	// retrieved by the guest later.
	securitySettings := &hcsschema.SecuritySettings{
		EnableTpm: false,
		Isolation: &hcsschema.IsolationSettings{
			IsolationType: "SecureNestedPaging",
			LaunchData:    hostData,
			// HclEnabled:    true, /* Not available in schema 2.5 - REQUIRED when using BlockStorage in 2.6 */
			HclEnabled: oci.ParseAnnotationsNullableBool(ctx, annotations, shimannotations.LCOWHclEnabled),
		},
	}

	// For confidential VMs, configure HvSocket service table with required VSock ports.
	log.G(ctx).Debug("configuring HvSocket service table for confidential VM")
	// Modifying BindSecurityDescriptor and ConnectSecurityDescriptor for confidential cases.
	hvSocketConfig.DefaultBindSecurityDescriptor = "D:P(A;;FA;;;WD)" // Differs for SNP
	hvSocketConfig.DefaultConnectSecurityDescriptor = "D:P(A;;FA;;;SY)(A;;FA;;;BA)"

	// Set permissions for the VSock ports:
	//	entropyVsockPort - 1 is the entropy port
	//	linuxLogVsockPort - 109 used by vsockexec to log stdout/stderr logging
	//	LinuxGcsVsockPort (0x40000000) is the GCS port
	//	LinuxGcsVsockPort + 1 is the bridge (see guestconnection.go)
	hvSockets := []uint32{vmutils.LinuxEntropyVsockPort, vmutils.LinuxLogVsockPort, prot.LinuxGcsVsockPort, prot.LinuxGcsVsockPort + 1}

	// Parse and append extra VSock ports from annotations
	extraVsockPorts := oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, iannotations.ExtraVSockPorts, []uint32{})
	hvSockets = append(hvSockets, extraVsockPorts...)

	log.G(ctx).WithFields(logrus.Fields{
		"vsockPorts": hvSockets,
		"extraPort":  extraVsockPorts,
	}).Debug("configured VSock ports for confidential VM")

	for _, whichSocket := range hvSockets {
		key := winio.VsockServiceID(whichSocket).String()
		hvSocketConfig.ServiceTable[key] = hcsschema.HvSocketServiceConfig{
			AllowWildcardBinds:        true,
			BindSecurityDescriptor:    "D:P(A;;FA;;;WD)",
			ConnectSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
		}
	}

	guestState, filesToCleanOnError, err := setGuestState(ctx, bundlePath, vmgsTemplatePath, guestStateFile, dmVerityRootfsTemplatePath, scsiCtrl)
	if err != nil {
		return nil, nil, nil, filesToCleanOnError, fmt.Errorf("failed to set guest state configuration: %w", err)
	}

	log.G(ctx).Debug("parseConfidentialOptions completed successfully")
	return chipset, securitySettings, guestState, filesToCleanOnError, nil
}

func setGuestState(
	ctx context.Context,
	bundlePath string,
	vmgsTemplatePath string,
	guestStateFile string,
	dmVerityRootfsTemplatePath string,
	scsiCtrl map[string]hcsschema.Scsi,
) (*hcsschema.GuestState, []string, error) {
	log.G(ctx).Debug("setGuestState: starting guest state configuration")
	var files []string

	// The kernel and minimal initrd are combined into a single vmgs file.
	vmgsFileFullPath := filepath.Join(bundlePath, guestStateFile)
	log.G(ctx).WithFields(logrus.Fields{
		"source": vmgsTemplatePath,
		"dest":   vmgsFileFullPath,
	}).Debug("copying VMGS template file")

	// Copy the VMGS template to the bundle directory.
	if err := copyfile.CopyFile(ctx, vmgsTemplatePath, vmgsFileFullPath, true); err != nil {
		return nil, nil, fmt.Errorf("failed to copy VMGS template file: %w", err)
	}
	files = append(files, vmgsFileFullPath)

	dmVerityRootFsFullPath := filepath.Join(bundlePath, vmutils.DefaultDmVerityRootfsVhd)
	log.G(ctx).WithFields(logrus.Fields{
		"source": dmVerityRootfsTemplatePath,
		"dest":   dmVerityRootFsFullPath,
	}).Debug("copying DM-Verity rootfs template file")

	// Copy the DM-Verity rootfs template to the bundle directory.
	if err := copyfile.CopyFile(ctx, dmVerityRootfsTemplatePath, dmVerityRootFsFullPath, true); err != nil {
		return nil, files, fmt.Errorf("failed to copy DM Verity rootfs template file: %w", err)
	}

	files = append(files, dmVerityRootFsFullPath)

	// Grant VM group access to the copied files.
	// Both files need to be accessible by the VM group for the confidential VM to use them.
	log.G(ctx).Debug("granting VM group access to confidential files")
	for _, filename := range []string{
		vmgsFileFullPath,
		dmVerityRootFsFullPath,
	} {
		if err := security.GrantVmGroupAccessWithMask(filename, security.AccessMaskAll); err != nil {
			return nil, files, fmt.Errorf("failed to grant VM group access ALL: %w", err)
		}
	}

	scsiController0 := guestrequest.ScsiControllerGuids[0]
	if _, ok := scsiCtrl[scsiController0]; !ok {
		scsiCtrl[scsiController0] = hcsschema.Scsi{
			Attachments: make(map[string]hcsschema.Attachment),
		}
	}

	// Attach the dm-verity rootfs VHD to SCSI controller 0, LUN 0.
	// This makes the verified rootfs available to the guest VM as a read-only disk.
	scsiCtrl[scsiController0].Attachments["0"] = hcsschema.Attachment{
		Type_:    "VirtualDisk",
		Path:     dmVerityRootFsFullPath,
		ReadOnly: true, // Read-only to ensure integrity of the verified rootfs
	}

	log.G(ctx).WithFields(logrus.Fields{
		"controller": scsiController0,
		"lun":        "0",
		"path":       dmVerityRootFsFullPath,
	}).Debug("configured SCSI attachment for dm-verity rootfs in confidential mode")

	// Configure guest state for the confidential VM.
	// Set up the VMGS file as the source for guest state.
	guestState := &hcsschema.GuestState{
		GuestStateFilePath:  vmgsFileFullPath,
		GuestStateFileType:  "FileMode",
		ForceTransientState: true, // tell HCS that this is just the source of the images, not ongoing state
	}

	log.G(ctx).Debug("setGuestState completed successfully")
	return guestState, files, nil
}
