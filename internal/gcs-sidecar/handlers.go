//go:build windows
// +build windows

package bridge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/fsformatter"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils/etw"
	"github.com/Microsoft/hcsshim/internal/windevice"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

const (
	sandboxStateDirName = "WcSandboxState"
	hivesDirName        = "Hives"
	devPathFormat       = "\\\\.\\PHYSICALDRIVE%d"
	UVMContainerID      = "00000000-0000-0000-0000-000000000000"
	// amdSnpPspDLLName is the AMD SNP PSP API DLL used to fetch SNP attestation
	// reports. It is staged from the UVM's System32 into each confidential
	// container's security-context directory so workloads can load it.
	amdSnpPspDLLName = "amdsnppspapi.dll"
)

// - Handler functions handle the incoming message requests. It
// also enforces security policy for confidential cwcow containers.
// - These handler functions may do some additional processing before
// forwarding requests to inbox GCS or send responses back to hcsshim.
// - In case of any error encountered during processing, appropriate error
// messages are returned and responses are sent back to hcsshim from ListenAndServer().
// TODO (kiashok): Verbose logging is for WIP and will be removed eventually.
func (b *Bridge) createContainer(req *request) (err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::createContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// Refuse to create containers once the UVM has been marked inconsistent by a
	// failed forwarded mount/unmount (cf. LCOW Host.checkState).
	if err := b.hostState.checkState(); err != nil {
		return fmt.Errorf("CreateContainer denied: %w", err)
	}

	var createContainerRequest prot.ContainerCreate
	var containerConfig json.RawMessage
	createContainerRequest.ContainerConfig.Value = &containerConfig
	if err = commonutils.UnmarshalJSONWithHresult(req.message, &createContainerRequest); err != nil {
		return errors.Wrap(err, "failed to unmarshal createContainer")
	}

	// containerConfig can be of type uvnConfig or hcsschema.HostedSystem or guestresource.CWCOWHostedSystem
	var (
		uvmConfig               prot.UvmConfig
		cwcowHostedSystemConfig guestresource.CWCOWHostedSystem
	)
	if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &uvmConfig); err == nil &&
		uvmConfig.SystemType != "" {
		systemType := uvmConfig.SystemType
		timeZoneInformation := uvmConfig.TimeZoneInformation
		log.G(ctx).Tracef("createContainer: uvmConfig: {systemType: %v, timeZoneInformation: %v}}", systemType, timeZoneInformation)
	} else if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &cwcowHostedSystemConfig); err == nil &&
		cwcowHostedSystemConfig.Spec.Version != "" && cwcowHostedSystemConfig.CWCOWHostedSystem.Container != nil {
		cwcowHostedSystem := cwcowHostedSystemConfig.CWCOWHostedSystem
		schemaVersion := cwcowHostedSystem.SchemaVersion
		container := cwcowHostedSystem.Container
		spec := cwcowHostedSystemConfig.Spec
		containerID := createContainerRequest.ContainerID
		if err := validateContainerID(containerID); err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}
		containerJSON, _ := json.Marshal(container)
		log.G(ctx).Tracef("rpcCreate: CWCOWHostedSystemConfig {spec: %v, schemaVersion: %v, container: %s}}", string(req.message), schemaVersion, containerJSON)

		// The block below is a reference example (not executed): a sample CRI
		// container.json and the HostedSystem.Container the host derives from it.
		// It documents the shapes this handler enforces and forwards.
		/*
			Test container.json:

			{
				"metadata": {
					"name": "wcow-test"
				},
				"image": {
					"image": "takurosatodevacr.azurecr.io/payload-demo:250929"
				},
				"command": [
					"python",
					"hello.py"
				],
				"envs": [
					{
					"key": "APP_FOO",
					"value": "BAR"
					}
				],
				"mounts": [
					{
					"host_path": "C:\\share-ro",
					"container_path": "C:\\mnt\\ro",
					"readonly": true
					},
					{
					"host_path": "\\\\.\\pipe\\hostedsystem-demo",
					"container_path": "\\\\.\\pipe\\hostedsystem-demo"
					}
				],
				"windows": {
					"security_context": {
						"credential_spec": "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-1111111111-2222222222-3333333333\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"244818ae-87ac-4fcd-92ec-e79e5252348a\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"CONTOSO\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"CONTOSO\"}]}}"
					},
					"resources": {
						"rootfs_size_in_bytes": 42949672960
					}
				}
			}

			HostedSystem.Container:
			{
				"Storage": {
				"Layers": [
					{
					"Id": "6e2349b7-8215-4325-a88a-38a8e1f67e18",
					"Path": "\\\\?\\Volume{6e2349b7-8215-4325-a88a-38a8e1f67e18}\\"
					}
				],
				"Path": "c:\\mounts\\scsi\\m0"
				},
				"MappedDirectories": [
				{
					"HostPath": "\\\\?\\VMSMB\\VSMB-{dcc079ae-60ba-4d07-847c-3493609c0870}\\s1",
					"ContainerPath": "C:\\mnt\\ro",
					"ReadOnly": true
				}
				],
				"MappedPipes": [
				{
					"ContainerPipeName": "hostedsystem-demo",
					"HostPath": "\\\\?\\VMSMB\\VSMB-{dcc079ae-60ba-4d07-847c-3493609c0870}\\IPC$\\hostedsystem-demo"
				}
				],
				"Processor": {},
				"Networking": {
				"Namespace": "644da769-7f9a-41c7-820b-8ef9e66d747b"
				},
				"ContainerCredentialGuard": {
				"Cookie": "01000000740069000CEBF50D32C0CF80BE559BE206B4EAF9",
				"RpcEndpoint": "91571621-3782-9EC0-3C5C-C0EC10E6E763",
				"Transport": "HvSocket",
				"CredentialSpec": "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-1111111111-2222222222-3333333333\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"244818ae-87ac-4fcd-92ec-e79e5252348a\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"CONTOSO\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"CONTOSO\"}]}}"
				},
				"RegistryChanges": {
				"AddValues": [
					{
					"Key": {
						"Hive": "System",
						"Name": "ControlSet001\\Control"
					},
					"Name": "WaitToKillServiceTimeout",
					"Type": "String",
					"StringValue": "2147483647"
					}
				]
				}
			}
		*/

		// Reject HostedSystem Container fields we don't yet support.
		if err := denyUnsupportedContainerFields(container); err != nil {
			return fmt.Errorf("CreateContainer operation rejected: %w", err)
		}

		// Enforce registry changes policy. This may drop unauthorized
		// non-default registry values from the container before forwarding.
		if container != nil && container.RegistryChanges != nil {
			log.G(ctx).Trace("Container has registry changes, validating against policy")

			// Separate the pre-approved defaults from the changes that must be
			// validated against policy (non-default add values plus all delete
			// keys).
			defaultValues, nonDefaultChanges := splitRegistryChanges(container.RegistryChanges)

			// If there are non-default values or any delete keys, validate them
			// against policy.
			if len(nonDefaultChanges.AddValues) > 0 || len(nonDefaultChanges.DeleteKeys) > 0 {
				log.G(ctx).Tracef("Validating %d registry values and %d delete keys against policy", len(nonDefaultChanges.AddValues), len(nonDefaultChanges.DeleteKeys))

				keptRaw, err := b.hostState.securityOptions.PolicyEnforcer.EnforceRegistryChangesPolicy(ctx, containerID, nonDefaultChanges)
				if err != nil {
					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
					return fmt.Errorf("registry entry operation is denied by policy: %w", err)
				}

				// The policy uses dropping semantics: it may authorize only a
				// subset of the requested non-default values and delete keys.
				// Rebuild the container's registry changes as the pre-approved
				// defaults plus the policy-kept non-default values, and the
				// policy-kept delete keys, so the guest only applies what policy
				// sanctioned.
				container.RegistryChanges.AddValues, container.RegistryChanges.DeleteKeys = mergeKeptRegistryChanges(defaultValues, keptRaw)
			}

			log.G(ctx).Infof("Registry validation complete: %d total values now applied (%d defaults), %d delete keys",
				len(container.RegistryChanges.AddValues), len(defaultValues), len(container.RegistryChanges.DeleteKeys))
		}

		// We enforce `spec`, which is not passed to inbox gcs within this createContainer.
		// The result of enforcement is stored in memory and used for executeProcess.
		user := securitypolicy.IDName{
			Name: spec.Process.User.Username,
		}
		envToKeep, _, allowStdio, err := b.hostState.securityOptions.PolicyEnforcer.EnforceCreateContainerPolicyV2(req.ctx, containerID, spec.Process.Args, spec.Process.Env, spec.Process.Cwd, spec.Mounts, user, nil)

		if err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}

		if envToKeep != nil {
			spec.Process.Env = []string(envToKeep)
		}

		commandLine := len(spec.Process.Args) > 0
		c := &Container{
			id:              containerID,
			spec:            spec,
			processes:       make(map[uint32]*containerProcess),
			commandLine:     commandLine,
			commandLineExec: false,
			allowStdio:      allowStdio,
		}

		log.G(ctx).Tracef("Adding ContainerID: %v", containerID)
		if err := b.hostState.AddContainer(req.ctx, containerID, c); err != nil {
			log.G(ctx).Tracef("Container exists in the map. containerID: %v", containerID)
			return err
		}
		defer func() {
			if err != nil {
				if removeErr := b.hostState.RemoveContainer(ctx, containerID); removeErr != nil {
					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
				}
			}
		}()

		// The security-context dir must always be written; it must not be gated
		// by a host-controlled annotation.
		securityContextDir, err := b.hostState.securityOptions.WriteSecurityContextDir(&spec)
		if err != nil {
			return fmt.Errorf("failed to write security context dir: %w", err)
		}

		// Stage the AMD SNP PSP API DLL into the container's security-context
		// directory so the workload can fetch SNP attestation reports. This
		// happens after security policy enforcement, consistent with the
		// UVM_SECURITY_CONTEXT_DIR env injection done by WriteSecurityContextDir.
		if securityContextDir != "" {
			if err := stageSnpPspDLL(ctx, securityContextDir); err != nil {
				return fmt.Errorf("failed to stage %s: %w", amdSnpPspDLLName, err)
			}
		}
		cwcowHostedSystemConfig.Spec = spec

		// Reconcile the host-provided HostedSystem mounts against the enforced
		// spec. spec.Mounts has already been validated against policy by
		// EnforceCreateContainerPolicyV2 above. Here we make sure the host is
		// not forwarding any MappedDirectories or MappedPipes that don't map to
		// an enforced spec mount, so the host can't smuggle in a mount the
		// policy never saw.
		if err := reconcileHostedSystemMounts(spec.Mounts, container); err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}

		// Cross-check the forwarded Container.Storage against the root path and
		// block-CIM volume the sidecar recorded for this container during layer setup.
		if err := reconcileHostedSystemStorage(b.hostState, containerID, container); err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}

		// Marshal the original cwcowHostedSystem from the request. That's safe
		// because we've enforced `spec` above and reconciled the forwarded
		// MappedDirectories/MappedPipes against it.
		hostedSystemBytes, err := json.Marshal(cwcowHostedSystem)

		if err != nil {
			return fmt.Errorf("failed to marshal hostedSystem: %w", err)
		}

		// marshal it again into a JSON-escaped string which inbox GCS expects
		hostedSystemEscapedBytes, err := json.Marshal(string(hostedSystemBytes))
		if err != nil {
			return fmt.Errorf("failed to marshal hostedSystem JSON: %w", err)
		}

		// Prepare a fixed struct that takes in raw message
		type containerCreateModified struct {
			prot.RequestBase
			ContainerConfig json.RawMessage
		}
		createContainerRequestModified := containerCreateModified{
			RequestBase:     createContainerRequest.RequestBase,
			ContainerConfig: hostedSystemEscapedBytes,
		}

		buf, err := json.Marshal(createContainerRequestModified)
		log.G(ctx).Tracef("marshaled request buffer: %s", string(buf))
		if err != nil {
			return fmt.Errorf("failed to marshal rpcCreatecontainer: %w", err)
		}
		var newRequest request
		newRequest.ctx = req.ctx
		newRequest.header = req.header
		newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
		newRequest.message = buf
		req = &newRequest
	} else {
		return fmt.Errorf("invalid request to createContainer")
	}

	b.forwardRequestToGcs(req)
	return nil
}

// splitRegistryChanges separates a container's requested registry changes into
// the pre-approved default add values (which bypass policy) and the changes
// that must be validated against policy: the non-default add values plus all
// delete keys, which have no default allowance.
func splitRegistryChanges(changes *hcsschema.RegistryChanges) (defaultValues []hcsschema.RegistryValue, nonDefaultChanges *hcsschema.RegistryChanges) {
	var nonDefaultValues []hcsschema.RegistryValue
	for _, value := range changes.AddValues {
		if isDefaultRegistryValue(value) {
			defaultValues = append(defaultValues, value)
		} else {
			nonDefaultValues = append(nonDefaultValues, value)
		}
	}
	return defaultValues, &hcsschema.RegistryChanges{
		AddValues:  nonDefaultValues,
		DeleteKeys: changes.DeleteKeys,
	}
}

// mergeKeptRegistryChanges combines the pre-approved default registry values
// with the policy-kept subset returned by EnforceRegistryChangesPolicy. Because
// the policy uses dropping semantics, it may authorize only a subset of the
// requested non-default values and delete keys; the returned slices are what
// the guest should apply (defaults plus the kept non-default values, and the
// kept delete keys).
func mergeKeptRegistryChanges(defaultValues []hcsschema.RegistryValue, kept interface{}) ([]hcsschema.RegistryValue, []hcsschema.RegistryKey) {
	var keptNonDefault []hcsschema.RegistryValue
	var keptDeleteKeys []hcsschema.RegistryKey
	if k, ok := kept.(*hcsschema.RegistryChanges); ok && k != nil {
		keptNonDefault = k.AddValues
		keptDeleteKeys = k.DeleteKeys
	}

	newValues := make([]hcsschema.RegistryValue, 0, len(defaultValues)+len(keptNonDefault))
	newValues = append(newValues, defaultValues...)
	newValues = append(newValues, keptNonDefault...)
	return newValues, keptDeleteKeys
}

// namedPipePrefix is the prefix used for Windows named pipe paths. A mount
// whose OCI destination starts with this prefix becomes a MappedPipe in the
// HostedSystem, with ContainerPipeName set to the destination minus this
// prefix (see internal/uvm.ParseNamedPipe and internal/hcsoci/hcsdoc_wcow.go).
const namedPipePrefix = `\\.\pipe\`

// isPipeDestination reports whether an OCI mount destination refers to a named
// pipe (and would therefore become a MappedPipe rather than a MappedDirectory).
func isPipeDestination(dest string) bool {
	return strings.HasPrefix(dest, namedPipePrefix)
}

// pipeNameFromDestination derives the ContainerPipeName that the host sets for
// a pipe mount from its OCI destination, mirroring ParseNamedPipe.
func pipeNameFromDestination(dest string) string {
	return strings.TrimPrefix(dest, namedPipePrefix)
}

// mountReadOnly reports whether an OCI mount's options request a read-only
// mount, mirroring how the host derives MappedDirectory.ReadOnly in
// internal/hcsoci/hcsdoc_wcow.go (an "ro" option, case-insensitive).
func mountReadOnly(options []string) bool {
	for _, o := range options {
		if strings.EqualFold(o, "ro") {
			return true
		}
	}
	return false
}

// reconcileHostedSystemMounts verifies that every MappedDirectory and
// MappedPipe the host forwards in the HostedSystem corresponds to an enforced
// spec mount. The spec mounts have already been validated against policy, so
// this binds the forwarded HostedSystem to that enforced view and rejects any
// host-added mount the policy never saw. Note that HostPath is intentionally
// not compared: the spec source is a host-side path while the HostedSystem
// HostPath is a VSMB path, so they legitimately differ and the host controls
// both regardless.
func reconcileHostedSystemMounts(mounts []oci.Mount, container *hcsschema.Container) error {
	if container == nil {
		return nil
	}

	// Every MappedDirectory must correspond to a (non-pipe) spec mount that
	// targets the same container path with the same read-only flag.
	for _, md := range container.MappedDirectories {
		matched := false
		for _, m := range mounts {
			// Pipe mounts are reconciled against MappedPipes below, not here.
			if isPipeDestination(m.Destination) {
				continue
			}
			// Bind on container path (spec destination) + read-only.
			if m.Destination == md.ContainerPath && mountReadOnly(m.Options) == md.ReadOnly {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("mapped directory %q (readOnly=%v) does not match any enforced spec mount", md.ContainerPath, md.ReadOnly)
		}
	}

	// Every MappedPipe must correspond to a pipe spec mount that yields the same
	// pipe name. We match on the pipe name (derived from the spec destination),
	// not the source.
	//
	// NB: for a pipe, the spec mount and the HostedSystem entry hold *different*
	// values for the "same" pipe, which is easy to trip over:
	//   - spec mount source:      "\\.\pipe\<name>"                         (pure name, NO guid)
	//   - MappedPipe.HostPath:    "\\?\VMSMB\VSMB-{guid}\IPC$\<name>"       (host VSMB transport, has guid)
	// The spec source stays the clean "\\.\pipe\<name>"; only the host-side
	// transport path (HostPath) carries the VSMB guid. HostPath is host-controlled
	// and not comparable to the spec source, so we don't compare it here; instead
	// we bind on the pipe name. The clean spec source is enforced separately by
	// policy (windows_mountConstraint_ok in framework.rego).
	for _, mp := range container.MappedPipes {
		matched := false
		for _, m := range mounts {
			// Non-pipe mounts are reconciled against MappedDirectories above.
			if !isPipeDestination(m.Destination) {
				continue
			}
			// Bind on the pipe name (destination minus the \\.\pipe\ prefix).
			if pipeNameFromDestination(m.Destination) == mp.ContainerPipeName {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("mapped pipe %q does not match any enforced spec mount", mp.ContainerPipeName)
		}
	}

	return nil
}

// volumeGUIDFromStoragePath extracts the volume GUID from a Container.Storage
// layer path of the form `\\?\Volume{<guid>}\` (the volume root, as the host
// writes it in the createContainer document). This differs from
// volumeGUIDFromLayerPath, which parses the `...}\Files` form used in the
// CWCOWCombinedLayers modify request.
func volumeGUIDFromStoragePath(path string) (string, bool) {
	if p, ok := strings.CutPrefix(path, `\\?\Volume{`); ok {
		if q, ok := strings.CutSuffix(p, `}\`); ok {
			return q, true
		}
	}
	return "", false
}

// reconcileHostedSystemStorage checks that the host-forwarded Container.Storage
// matches the verified handles the sidecar recorded for this container during
// layer setup:
//   - Storage.Path must equal the combined-layers root that CWCOWCombinedLayers
//     mounted for this container (the scratch that becomes the container root).
//   - Storage.Layers must be the single block-CIM volume whose hashes mount_cims
//     verified for this container.
//
// The bytes at that volume are already verity-verified, so this does not re-check
// content. It closes a cross-wiring gap: without it a host could forward a create
// document that points the container root at a different (even if separately
// verified) volume than the one enforced for this container.
func reconcileHostedSystemStorage(host *Host, containerID string, container *hcsschema.Container) error {
	if container == nil || container.Storage == nil {
		return fmt.Errorf("container storage is missing")
	}
	storage := container.Storage

	wantRootPath, ok := host.containerRootPaths[containerID]
	if !ok {
		return fmt.Errorf("no container root path recorded for container %s", containerID)
	}
	if !strings.EqualFold(storage.Path, wantRootPath) {
		return fmt.Errorf("storage path %q does not match the enforced container root path %q", storage.Path, wantRootPath)
	}

	if len(storage.Layers) != 1 {
		return fmt.Errorf("expected exactly one storage layer, got %d", len(storage.Layers))
	}
	guidStr, ok := volumeGUIDFromStoragePath(storage.Layers[0].Path)
	if !ok {
		return fmt.Errorf("storage layer path %q is not a volume path", storage.Layers[0].Path)
	}
	volGUID, err := guid.FromString(guidStr)
	if err != nil {
		return fmt.Errorf("invalid storage layer volume GUID %q: %w", guidStr, err)
	}
	containers, ok := host.blockCIMVolumeContainers[volGUID]
	if !ok {
		return fmt.Errorf("storage layer volume %s was not verified", volGUID)
	}
	if _, ok := containers[containerID]; !ok {
		return fmt.Errorf("storage layer volume %s was not verified for container %s", volGUID, containerID)
	}
	return nil
}

// stageSnpPspDLL copies the AMD SNP PSP API DLL from the UVM's System32 into the
// container's security-context directory so the workload can fetch SNP
// attestation reports. The directory is exposed to the container via the
// UVM_SECURITY_CONTEXT_DIR environment variable. If the DLL is not present in
// the UVM (e.g. a non-SNP UVM), staging is skipped without error.
func stageSnpPspDLL(ctx context.Context, securityContextDir string) error {
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("failed to get system directory: %w", err)
	}

	srcPath := filepath.Join(sysDir, amdSnpPspDLLName)
	staged, err := stageDLL(ctx, srcPath, securityContextDir)
	if err != nil {
		return err
	}
	if staged {
		log.G(ctx).Debugf("staged %s into %s", amdSnpPspDLLName, securityContextDir)
	} else {
		log.G(ctx).Debugf("%s not found in %s; skipping staging", amdSnpPspDLLName, sysDir)
	}
	return nil
}

// containerIDRegex matches the identifier format used for container IDs: one
// or more alphanumeric segments joined by single '.', '_' or '-' separators
// (the same shape containerd enforces for identifiers). GUIDs and hex digests
// both satisfy it. It rejects empty strings, path separators, ".." and
// absolute paths, so a host-supplied container ID cannot be used to escape an
// intended directory if it is later joined into a filesystem path.
var containerIDRegex = regexp.MustCompile(`^[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*$`)

func validateContainerID(id string) error {
	if !containerIDRegex.MatchString(id) {
		return fmt.Errorf("invalid container ID %q", id)
	}
	return nil
}

// denyUnsupportedContainerFields rejects HostedSystem Container fields that the
// sidecar does not yet enforce a policy over. They may be needed in the future,
// but until we have enforcement for them we block them rather than forward
// host-controlled values unchecked.
//
// Memory, Processor and Networking are deliberately not checked: the host
// controls the UVM's resources and networking regardless, so there is nothing
// we can meaningfully enforce over them here.
func denyUnsupportedContainerFields(container *hcsschema.Container) error {
	if container == nil {
		return nil
	}
	if container.GuestOs != nil {
		return fmt.Errorf("GuestOs is not supported")
	}
	if container.HvSocket != nil {
		return fmt.Errorf("HvSocket is not supported")
	}
	if container.ContainerCredentialGuard != nil {
		return fmt.Errorf("ContainerCredentialGuard is not supported")
	}
	if len(container.AssignedDevices) > 0 {
		return fmt.Errorf("AssignedDevices is not supported")
	}
	if container.AdditionalDeviceNamespace != nil {
		return fmt.Errorf("AdditionalDeviceNamespace is not supported")
	}
	return nil
}

// stageDLL copies the DLL at srcPath into dstDir. If the source DLL does not
// exist it is a no-op and returns false without error, so callers can tolerate
// environments where the DLL is not present.
func stageDLL(ctx context.Context, srcPath, dstDir string) (bool, error) {
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", srcPath, err)
	}

	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	if err := copyfile.CopyFile(ctx, srcPath, dstPath, true); err != nil {
		return false, fmt.Errorf("failed to copy %s to %s: %w", srcPath, dstPath, err)
	}

	return true, nil
}

// processParamEnvToOCIEnv converts an Environment field from ProcessParameters
// (a map from environment variable to value) into an array of environment
// variable assignments (where each is in the form "<variable>=<value>") which
// can be used by an oci.Process.
func processParamEnvToOCIEnv(environment map[string]string) []string {
	environmentList := make([]string, 0, len(environment))
	for k, v := range environment {
		// TODO: Do we need to escape things like quotation marks in
		// environment variable values?
		environmentList = append(environmentList, fmt.Sprintf("%s=%s", k, v))
	}
	return environmentList
}

// ociEnvToProcessParamEnv is the inverse of processParamEnvToOCIEnv. It converts
// an OCI-style env list (["KEY=VALUE", ...]) back to a ProcessParameters
// Environment map.
func ociEnvToProcessParamEnv(envs []string) map[string]string {
	paramEnv := make(map[string]string, len(envs))
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			paramEnv[parts[0]] = parts[1]
		}
	}
	return paramEnv
}

// escapeArgs builds a Windows-style escaped command line from a set of OCI
// process args. This mirrors how the host shim constructs the init process'
// ProcessParameters.CommandLine (internal/cmd.escapeArgs), so the sidecar can
// reconstruct the expected command line from the enforced spec and compare it
// against what the host actually sends in executeProcess.
func escapeArgs(args []string) string {
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = windows.EscapeArg(a)
	}
	return strings.Join(escaped, " ")
}

// rewriteExecRequest re-marshals an execute process request with updated
// ProcessParameters (e.g., after env filtering by policy).
func rewriteExecRequest(req *request, r prot.ContainerExecuteProcess, params hcsschema.ProcessParameters) (*request, error) {
	r.Settings.ProcessParameters.Value = &params

	buf, err := json.Marshal(&r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated exec request: %w", err)
	}

	newReq := &request{
		ctx:     req.ctx,
		header:  req.header,
		message: buf,
	}
	newReq.header.Size = uint32(len(buf)) + prot.HdrSize
	return newReq, nil
}

// enforceStdioParams applies a stdio-access policy decision. When denied, a
// process that requires a console is rejected (there is no console without
// stdio); otherwise the stdio pipe flags are cleared. Returns whether params
// changed so callers can skip an unnecessary rewrite.
func enforceStdioParams(allowStdio bool, params *hcsschema.ProcessParameters) (bool, error) {
	if allowStdio {
		return false, nil
	}

	// A console can't be honored without stdio, so reject rather than silently
	// dropping EmulateConsole and running a non-interactive process the caller
	// didn't ask for.
	if params.EmulateConsole {
		return false, errors.New("process that requires console access denied due to policy not allowing stdio access")
	}

	changed := params.CreateStdInPipe || params.CreateStdOutPipe || params.CreateStdErrPipe
	params.CreateStdInPipe = false
	params.CreateStdOutPipe = false
	params.CreateStdErrPipe = false
	return changed, nil
}

func (b *Bridge) startContainer(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::startContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// We don't need any enforcement here because the container has already been created and
	// this request is just to start the container.

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal startContainer: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) shutdownGraceful(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::shutdownGraceful")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal shutdownGraceful: %w", err)
	}

	err = b.hostState.securityOptions.PolicyEnforcer.EnforceShutdownContainerPolicy(req.ctx, r.ContainerID)
	if err != nil {
		return fmt.Errorf("rpcShudownGraceful operation not allowed: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) shutdownForced(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::shutdownForced")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal shutdownForced: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) executeProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::executeProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerExecuteProcess
	var processParamSettings json.RawMessage
	r.Settings.ProcessParameters.Value = &processParamSettings
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal executeProcess: %w", err)
	}
	containerID := r.ContainerID
	var processParams hcsschema.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult(processParamSettings, &processParams); err != nil {
		return fmt.Errorf("executeProcess: invalid params type for request: %w", err)
	}

	commandLine := []string{processParams.CommandLine}

	if containerID == UVMContainerID {
		log.G(req.ctx).Tracef("Enforcing policy on external exec process")
		envToKeep, stdioAllowed, err := b.hostState.securityOptions.PolicyEnforcer.EnforceExecExternalProcessPolicy(
			req.ctx,
			commandLine,
			processParamEnvToOCIEnv(processParams.Environment),
			processParams.WorkingDirectory,
		)
		if err != nil {
			return errors.Wrapf(err, "exec is denied due to policy")
		}
		needsRewrite := false
		if envToKeep != nil {
			processParams.Environment = ociEnvToProcessParamEnv(envToKeep)
			needsRewrite = true
		}
		stdioChanged, err := enforceStdioParams(stdioAllowed, &processParams)
		if err != nil {
			return errors.Wrapf(err, "exec is denied due to policy")
		}
		needsRewrite = needsRewrite || stdioChanged
		if needsRewrite {
			req, err = rewriteExecRequest(req, r, processParams)
			if err != nil {
				return fmt.Errorf("failed to rewrite exec request: %w", err)
			}
		}
		b.forwardRequestToGcs(req)
	} else {
		// fetch the container command line
		c, err := b.hostState.GetCreatedContainer(req.ctx, containerID)
		if err != nil {
			log.G(req.ctx).Tracef("Container not found during exec: %v", containerID)
			return fmt.Errorf("failed to get created container: %w", err)
		}

		c.processesMutex.Lock()
		isCreateExec := c.commandLine && !c.commandLineExec
		if isCreateExec {
			// if this is an exec of Container command line, then it's already enforced
			// during container creation.
			// We use the result of enforcement from container creation to
			// validate the exec command line and drop environment variable if necessary.

			c.commandLineExec = true

		}
		c.processesMutex.Unlock()
		if !isCreateExec {
			user := securitypolicy.IDName{
				Name: processParams.User,
			}
			log.G(req.ctx).Tracef("Enforcing policy on exec in container")
			envToKeep, _, stdioAllowed, err := b.hostState.securityOptions.PolicyEnforcer.
				EnforceExecInContainerPolicyV2(
					req.ctx,
					containerID,
					commandLine,
					processParamEnvToOCIEnv(processParams.Environment),
					processParams.WorkingDirectory,
					user,
					nil,
				)
			if err != nil {
				return errors.Wrapf(err, "exec in container denied due to policy")
			}
			needsRewrite := false
			if envToKeep != nil {
				processParams.Environment = ociEnvToProcessParamEnv(envToKeep)
				needsRewrite = true
			}
			stdioChanged, err := enforceStdioParams(stdioAllowed, &processParams)
			if err != nil {
				return errors.Wrapf(err, "exec in container denied due to policy")
			}
			needsRewrite = needsRewrite || stdioChanged
			if needsRewrite {
				req, err = rewriteExecRequest(req, r, processParams)
				if err != nil {
					return fmt.Errorf("failed to rewrite exec request: %w", err)
				}
			}
		} else {
			// This is the container's init process. Its command line, working
			// directory, user and environment were already validated against
			// policy in createContainer, and the result is stored in c.spec.
			// The host fully controls this executeProcess request though, so we
			// cross-check it against the enforced spec instead of trusting it:
			// otherwise a host could pass policy with a benign spec at create
			// time and then launch a different init command (e.g.
			// "cmd.exe /c <evil>") or smuggle back environment variables that
			// create-time enforcement dropped.
			if c.spec.Process == nil {
				return errors.New("exec in container denied due to policy: enforced spec has no process")
			}
			enforced := c.spec.Process

			expectedCmdLine := enforced.CommandLine
			if expectedCmdLine == "" {
				expectedCmdLine = escapeArgs(enforced.Args)
			}
			if processParams.CommandLine != expectedCmdLine {
				return fmt.Errorf("exec in container denied due to policy: init command line %q does not match enforced %q", processParams.CommandLine, expectedCmdLine)
			}
			if enforced.Cwd != "" && processParams.WorkingDirectory != enforced.Cwd {
				return fmt.Errorf("exec in container denied due to policy: init working directory %q does not match enforced %q", processParams.WorkingDirectory, enforced.Cwd)
			}
			if enforced.User.Username != "" && processParams.User != enforced.User.Username {
				return fmt.Errorf("exec in container denied due to policy: init user %q does not match enforced %q", processParams.User, enforced.User.Username)
			}

			// Re-apply the environment that createContainer enforcement
			// produced (dropped variables removed, nothing injected) so the
			// init process runs with exactly the enforced environment.
			processParams.Environment = ociEnvToProcessParamEnv(enforced.Env)

			if _, err = enforceStdioParams(c.allowStdio, &processParams); err != nil {
				return errors.Wrapf(err, "exec in container denied due to policy")
			}

			req, err = rewriteExecRequest(req, r, processParams)
			if err != nil {
				return fmt.Errorf("failed to rewrite init exec request: %w", err)
			}
		}
		headerID := req.header.ID

		// initiate exec process response channel
		procRespCh := make(chan *prot.ContainerExecuteProcessResponse, 1)
		b.pendingMu.Lock()
		b.pending[headerID] = procRespCh
		b.pendingMu.Unlock()

		defer func() {
			b.pendingMu.Lock()
			delete(b.pending, headerID)
			b.pendingMu.Unlock()
		}()

		// forward the request to gcs
		b.forwardRequestToGcs(req)

		// fetch the process ID from response
		select {
		case resp := <-procRespCh:
			// capture the Process details, so that we can later enforce
			// on the allowed signals on the Process
			if resp != nil {
				log.G(req.ctx).Tracef("Got response: %+v", resp)
				c.processesMutex.Lock()
				defer c.processesMutex.Unlock()
				c.processes[resp.ProcessID] = &containerProcess{
					processspec: processParams,
					cid:         c.id,
					pid:         resp.ProcessID,
				}
				return nil
			}
			// Channel closed or received nil, treat as error
			return errors.New("received nil exec response")
		case <-time.After(5 * time.Second):
			return errors.New("timed out waiting for exec response")
		}
	}
	return nil
}

func (b *Bridge) waitForProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::waitForProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal waitForProcess: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) signalProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::signalProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerSignalProcess
	var rawOpts json.RawMessage
	r.Options = &rawOpts
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal signalProcess: %w", err)
	}
	var wcowOptions guestresource.SignalProcessOptionsWCOW
	if rawOpts != nil {
		if err := commonutils.UnmarshalJSONWithHresult(rawOpts, &wcowOptions); err != nil {
			return fmt.Errorf("signalProcess: invalid Options type for request: %w", err)
		}

		log.G(req.ctx).Tracef("RawOpts are not nil")
		containerID := r.ContainerID
		c, err := b.hostState.GetCreatedContainer(req.ctx, containerID)
		if err != nil {
			return fmt.Errorf("failed to get created container: %w", err)
		}

		p, err := c.GetProcess(r.ProcessID)
		if err != nil {
			log.G(req.ctx).Tracef("Process not found %v", r.ProcessID)
			return err
		}
		cmdLine := p.processspec.CommandLine
		commandLine := []string{cmdLine}
		opts := &securitypolicy.SignalContainerOptions{
			IsInitProcess:  false,
			WindowsSignal:  wcowOptions.Signal,
			WindowsCommand: commandLine,
		}
		err = b.hostState.securityOptions.PolicyEnforcer.EnforceSignalContainerProcessPolicyV2(req.ctx, containerID, opts)
		if err != nil {
			return err
		}

	}
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) resizeConsole(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::resizeConsole")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal resizeConsole: %v", req)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) getProperties(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::getProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if err := b.hostState.securityOptions.PolicyEnforcer.EnforceGetPropertiesPolicy(req.ctx); err != nil {
		return errors.Wrapf(err, "get properties denied due to policy")
	}

	var getPropReqV2 prot.ContainerGetPropertiesV2
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &getPropReqV2); err != nil {
		return fmt.Errorf("failed to unmarshal getProperties: %v: %w", string(req.message), err)
	}
	log.G(req.ctx).Tracef("getProperties query: %v", getPropReqV2.Query.PropertyTypes)

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) negotiateProtocol(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::negotiateProtocol")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.NegotiateProtocolRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal negotiateProtocol")
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) dumpStacks(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::dumpStacks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.DumpStacksRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal dumpStacks: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) deleteContainerState(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::deleteContainerState")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// Refuse to delete container state once the UVM has been marked inconsistent
	// by a failed forwarded mount/unmount (cf. LCOW Host.checkState).
	if err := b.hostState.checkState(); err != nil {
		return fmt.Errorf("deleteContainerState denied: %w", err)
	}

	var r prot.DeleteContainerStateRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal deleteContainerState: %w", err)
	}

	// Refuse to delete the state of a container that is still running, or whose
	// combined-layers root is still mounted, so the host can't wipe a live
	// container's rootfs (cf. LCOW Host.DeleteContainerState).
	c, err := b.hostState.GetCreatedContainer(req.ctx, r.ContainerID)
	if err != nil {
		log.G(req.ctx).Tracef("Container not found during deleteContainerState: %v", r.ContainerID)
		return fmt.Errorf("container not found: %w", err)
	}
	if !c.terminated.Load() {
		return fmt.Errorf("deleteContainerState denied: container %s is still running", r.ContainerID)
	}
	if b.hostState.IsContainerRootMountedForContainer(r.ContainerID) {
		return fmt.Errorf("deleteContainerState denied: container %s combined-layers root is still mounted", r.ContainerID)
	}

	if err = b.hostState.RemoveContainer(req.ctx, r.ContainerID); err != nil {
		log.G(req.ctx).Tracef("Container not found during deleteContainerState: %v", r.ContainerID)
		return fmt.Errorf("container not found: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) updateContainer(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::updateContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// No callers in the code for rpcUpdateContainer
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) lifecycleNotification(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::lifecycleNotification")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// No callers in the code for rpcLifecycleNotification
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) modifyServiceSettings(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::modifyServiceSettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// Todo: Add policy enforcement for modifying service settings
	modifyRequest, err := unmarshalModifyServiceSettings(req)
	if err != nil {
		return fmt.Errorf("failed to unmarshal modifyServiceSettings request: %w", err)
	}

	switch modifyRequest.PropertyType {
	case string(prot.LogForwardService):
		if modifyRequest.Settings != nil {
			log.G(req.ctx).Tracef("modifyServiceSettings for LogForwardService with RPCModifyServiceSettings, enforcing policy for log sources")
			settings := modifyRequest.Settings.(*guestrequest.LogForwardServiceRPCRequest)

			switch settings.RPCType {
			case guestrequest.RPCModifyServiceSettings, guestrequest.RPCStartLogForwarding, guestrequest.RPCStopLogForwarding:
				log.G(req.ctx).Tracef("%v request received for LogForwardService, proceeding with policy enforcement for log sources", settings.RPCType)
				// Enforce the policy for log sources in the request and update the settings with allowed log sources.
				// For cwcow, the sidecar-GCS will verify the allowed log sources against policy and append the necessary GUIDs to the ones allowed. Rest are dropped.
				// The Enforcer will have to unmarshal the log sources, enforce the policy and then marshal it back to a Base64 encoded JSON string which is what inbox GCS expects.
				// It can query etw.GetDefaultLogSources to get the default log sources if the policy allows, and allow providers matching the default list during policy enforcement.
				// This is because the log sources can be a combination of default and user specified log sources for which GUIDs need to be appended based on the policy enforcement.
				if settings.Settings != "" {
					// <EXAMPLE CALL>
					// allowedLogSources, err := b.hostState.securityOptions.PolicyEnforcer.EnforceLogForwardServiceSettingsPolicy(req.ctx, settings.LogSources)

					// For now, we are skipping the policy enforcement and allowing all log sources as the policy enforcer implementation is in progress. We will add the enforcement back once it's implemented.
					allowedLogSources := settings.Settings // This is Base64 encoded JSON string of log sources
					log.G(req.ctx).Tracef("Allowed log sources after policy enforcement: %v", allowedLogSources)

					// Update the allowed log sources in the settings. This will be forwarded to inbox GCS which expects the log sources in a JSON string format with GUIDs for providers included.
					allowedLogSources, err := etw.UpdateLogSources(allowedLogSources, false, true)
					if err != nil {
						return fmt.Errorf("failed to update log sources: %w", err)
					}
					settings.Settings = allowedLogSources
				}
			default:
				log.G(req.ctx).Warningf("modifyServiceSettings for LogForwardService with RPCType: %v, skipping policy enforcement", settings.RPCType)
			}
			modifyRequest.Settings = settings
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal modifyServiceSettings request: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest
		} else {
			log.G(req.ctx).Warningf("modifyServiceSettings for LogForwardService with empty settings, skipping policy enforcement")
		}
	default:
		log.G(req.ctx).Warningf("modifyServiceSettings with PropertyType: %v, skipping policy enforcement", modifyRequest.PropertyType)
	}
	b.forwardRequestToGcs(req)
	return nil
}

func volumeGUIDFromLayerPath(path string) (string, bool) {
	if p, ok := strings.CutPrefix(path, `\\?\Volume{`); ok {
		if q, ok := strings.CutSuffix(p, `}\Files`); ok {
			return q, true
		}
	}
	return "", false
}

func (b *Bridge) modifySettings(req *request) (err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::modifySettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	log.G(ctx).Tracef("modifySettings: MsgType: %v, Payload: %v", req.header.Type, string(req.message))
	modifyRequest, err := unmarshalContainerModifySettings(req)
	if err != nil {
		return err
	}
	modifyGuestSettingsRequest := modifyRequest.Request.(*guestrequest.ModificationRequest)
	guestResourceType := modifyGuestSettingsRequest.ResourceType
	guestRequestType := modifyGuestSettingsRequest.RequestType
	log.G(ctx).Tracef("modifySettings: resourceType: %v, requestType: %v", guestResourceType, guestRequestType)

	if guestRequestType == "" {
		guestRequestType = guestrequest.RequestTypeAdd
	}

	switch guestRequestType {
	case guestrequest.RequestTypeAdd:
	case guestrequest.RequestTypeRemove:
	case guestrequest.RequestTypePreAdd:
	case guestrequest.RequestTypeUpdate:
	default:
		return fmt.Errorf("invald guestRequestType %v", guestRequestType)
	}

	// If a previously forwarded mount/unmount operation failed in the inbox GCS,
	// the sidecar's policy state may be out of sync with what is actually mounted
	// and cannot be safely recovered, so refuse all further settings changes
	// (cf. LCOW checkState gating in internal/guest/runtime/hcsv2/uvm.go).
	if err := b.hostState.checkState(); err != nil {
		return fmt.Errorf("modifySettings denied: %w", err)
	}

	// monitorResponse is set for forwarded combined-layers / mapped-directory
	// operations whose real work happens in the inbox GCS. Their inbox response
	// is watched (see monitorInboxResponse) so a failure fails the UVM closed,
	// since the sidecar cannot revert the policy state it staged for them.
	monitorResponse := false

	// Question: should we enforce policy for each type? Maybe just reject if we don't implement policy?
	if guestResourceType != "" {
		switch guestResourceType {
		case guestresource.ResourceTypeCombinedLayers:
			// This is for non-confidential WCOW.
			// Ideally gcs-sidecar supports it with policy enforcement,
			// but for now we just reject it because
			// we don't have a policy enforcer for it.
			settings := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWCombinedLayers)
			log.G(ctx).Tracef("WCOWCombinedLayers: {%v}", settings)
			return fmt.Errorf("WCOWCombinedLayers is not supported.")

		case guestresource.ResourceTypeNetworkNamespace:
			// Forwarded to inbox GCS without enforcement, by design: the host
			// controls the UVM's networking regardless of what is configured here,
			// so there is nothing meaningful for the guest to enforce.
			// LCOW does the same (see modifyNetwork in internal\guest\runtime\hcsv2\uvm.go).
			settings := modifyGuestSettingsRequest.Settings.(*hcn.HostComputeNamespace)
			log.G(ctx).Tracef("HostComputeNamespaces { %v}", settings)

		case guestresource.ResourceTypeNetwork:
			// Forwarded without enforcement for the same reason as
			// ResourceTypeNetworkNamespace above: networking is host-controlled.
			settings := modifyGuestSettingsRequest.Settings.(*guestrequest.NetworkModifyRequest)
			log.G(ctx).Tracef("NetworkModifyRequest { %v}", settings)

		case guestresource.ResourceTypeMappedVirtualDisk:
			settings := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWMappedVirtualDisk)
			log.G(ctx).Tracef("WCOWMappedVirtualDisk: {%v}", settings)
			return fmt.Errorf("MappedVirtualDisk is not supported")

		case guestresource.ResourceTypeHvSocket:
			// Forwarded without enforcement: this is just for configuration
			// to help guest to resolve hvsocket targets.
			settings := modifyGuestSettingsRequest.Settings.(*hcsschema.HvSocketAddress)
			log.G(ctx).Tracef("HvSocketAddress { %v }", settings)

		case guestresource.ResourceTypeMappedDirectory:
			// We don't have hostpath enforcement because anyway contents of the dir can be changed by the host.
			settings := modifyGuestSettingsRequest.Settings.(*hcsschema.MappedDirectory)
			log.G(ctx).Tracef("hcsschema.MappedDirectory { %v }", settings)
			switch modifyGuestSettingsRequest.RequestType {
			case guestrequest.RequestTypeAdd:
				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceMappedDirectoryMountPolicy(
					ctx, settings.ContainerPath, settings.ReadOnly); err != nil {
					return fmt.Errorf("mapped directory mount is denied by policy: %w", err)
				}
			case guestrequest.RequestTypeRemove:
				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceMappedDirectoryUnmountPolicy(
					ctx, settings.ContainerPath); err != nil {
					return fmt.Errorf("mapped directory unmount is denied by policy: %w", err)
				}
			default:
				return fmt.Errorf("unsupported request type %v for MappedDirectory", modifyGuestSettingsRequest.RequestType)
			}
			// The sidecar enforced policy here but the actual VSMB mount/unmount
			// happens in the inbox GCS, so watch its response and fail closed on
			// failure (the staged policy metadata cannot be reverted).
			monitorResponse = true

		case guestresource.ResourceTypeSecurityPolicy:
			securityPolicyRequest := modifyGuestSettingsRequest.Settings.(*guestresource.ConfidentialOptions)
			log.G(ctx).Tracef("WCOWConfidentialOptions: { %v}", securityPolicyRequest)
			err := b.hostState.securityOptions.SetConfidentialOptions(ctx,
				securityPolicyRequest.EnforcerType,
				securityPolicyRequest.EncodedSecurityPolicy,
				securityPolicyRequest.EncodedUVMReference,
				securityPolicyRequest.EncodedUVMHashEnvelopeReference)
			if err != nil {
				return errors.Wrap(err, "Failed to set Confidentia UVM Options")
			}
			// Send response back to shim
			resp := &prot.ResponseBase{
				Result:     0, // 0 means success
				ActivityID: req.activityID,
			}
			err = b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)
			if err != nil {
				return fmt.Errorf("error sending response to hcsshim: %w", err)
			}
			return nil
		case guestresource.ResourceTypePolicyFragment:
			r, ok := modifyGuestSettingsRequest.Settings.(*guestresource.SecurityPolicyFragment)
			if !ok {
				return errors.New("the request settings are not of type SecurityPolicyFragment")
			}
			if err := b.hostState.securityOptions.InjectFragment(ctx, r); err != nil {
				return err
			}
			resp := &prot.ResponseBase{
				Result:     0,
				ActivityID: req.activityID,
			}
			return b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)

		case guestresource.ResourceTypeWCOWBlockCims:
			// This is request to mount the merged cim at given volumeGUID
			switch modifyGuestSettingsRequest.RequestType {
			case guestrequest.RequestTypeAdd:
				wcowBlockCimMounts := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWBlockCIMMounts)
				containerID := wcowBlockCimMounts.ContainerID
				log.G(ctx).Tracef("WCOWBlockCIMMounts Add { %v}", wcowBlockCimMounts)

				var layerCIMs []*cimfs.BlockCIM
				layerHashes := make([]string, len(wcowBlockCimMounts.BlockCIMs))
				layerDigests := make([][]byte, len(wcowBlockCimMounts.BlockCIMs))
				for i, blockCimDevice := range wcowBlockCimMounts.BlockCIMs {
					// Get the scsi device path for the blockCim lun
					// The block device takes some time to show up. Retry for up to 2 seconds.
					var devNumber uint32
					waitStartTime := time.Now()
					logTime := waitStartTime.Add(time.Second)
					logged := false
					for {
						devNumber, err = windevice.GetDeviceNumberFromControllerLUN(
							req.ctx,
							0, /* controller is always 0 for wcow */
							uint8(blockCimDevice.Lun))
						if err == nil {
							break
						}

						// Check if we've exceeded max wait time
						if time.Since(waitStartTime) >= 2*time.Second {
							return fmt.Errorf("err getting scsiDevPath after 2s: %w", err)
						}

						// Log if taking longer than expected
						if !logged && logTime.Before(time.Now()) {
							log.G(ctx).WithFields(map[string]interface{}{
								"lun":     blockCimDevice.Lun,
								"elapsed": time.Since(waitStartTime),
							}).Warn("waiting for block CIM device to show up")
							logged = true
						}

						time.Sleep(10 * time.Millisecond)
					}
					physicalDevPath := fmt.Sprintf(devPathFormat, devNumber)
					layerCim := cimfs.BlockCIM{
						Type:      cimfs.BlockCIMTypeDevice,
						BlockPath: physicalDevPath,
						CimName:   blockCimDevice.CimName,
					}
					cimRootDigestBytes, err := cimfs.GetVerificationInfo(physicalDevPath)
					if err != nil {
						return fmt.Errorf("failed to get CIM verification info: %w", err)
					}
					layerDigests[i] = cimRootDigestBytes
					layerHashes[i] = hex.EncodeToString(cimRootDigestBytes)
					layerCIMs = append(layerCIMs, &layerCim)

					log.G(ctx).Debugf("block CIM layer digest %s, path: %s\n", layerHashes[i], physicalDevPath)
				}

				// Top layer is the merged layer that will also be verified
				hashesToVerify := layerHashes
				mountedCim := []string{layerHashes[0]}
				if len(layerHashes) > 1 {
					hashesToVerify = layerHashes[1:]
				}

				// Volume GUID from request.
				volGUID := wcowBlockCimMounts.VolumeGUID

				// Enforce policy, mount, then record the verified state as a single
				// transaction: if the real mount fails after the policy check,
				// WithMetadataRollback reverts the policy metadata and we skip the
				// sidecar caches, so policy state can't desync from what is mounted.
				if rberr := b.hostState.securityOptions.PolicyEnforcer.WithMetadataRollback(func() error {
					if err := b.hostState.securityOptions.PolicyEnforcer.EnforceVerifiedCIMsPolicy(req.ctx, containerID, hashesToVerify, mountedCim, volGUID.String()); err != nil {
						return errors.Wrap(err, "CIM mount is denied by policy")
					}

					if len(layerCIMs) > 1 {
						if _, merr := cimfs.MountMergedVerifiedBlockCIMs(layerCIMs[0], layerCIMs[1:], wcowBlockCimMounts.MountFlags, wcowBlockCimMounts.VolumeGUID, layerDigests[0]); merr != nil {
							return fmt.Errorf("error mounting multilayer block cims: %w", merr)
						}
					} else {
						if _, merr := cimfs.MountVerifiedBlockCIM(layerCIMs[0], wcowBlockCimMounts.MountFlags, wcowBlockCimMounts.VolumeGUID, layerDigests[0]); merr != nil {
							return fmt.Errorf("error mounting verified block cim: %w", merr)
						}
					}

					// Real mount succeeded: record the verified state.
					b.hostState.blockCIMVolumeHashes[volGUID] = layerHashes
					if _, ok := b.hostState.blockCIMVolumeContainers[volGUID]; !ok {
						b.hostState.blockCIMVolumeContainers[volGUID] = make(map[string]struct{})
					}
					b.hostState.blockCIMVolumeContainers[volGUID][containerID] = struct{}{}
					log.G(ctx).Tracef("Cached %d verified CIM layer hashes for volume %s (container %s)", len(hashesToVerify), volGUID, containerID)
					return nil
				}); rberr != nil {
					return rberr
				}

			case guestrequest.RequestTypeRemove:
				log.G(ctx).Tracef("WCOWBlockCIMMounts: Remove")
				wcowBlockCimMounts := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWBlockCIMMounts)
				volGUID := wcowBlockCimMounts.VolumeGUID

				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceCIMUnmountPolicy(req.ctx, volGUID.String()); err != nil {
					return fmt.Errorf("CIM unmount is denied by policy: %w", err)
				}

				volumePath := fmt.Sprintf(cimfs.VolumePathFormat, volGUID.String())
				err := cimfs.Unmount(volumePath)
				if err != nil {
					return fmt.Errorf("error unmounting block cim: %w", err)
				}

				// Drop the cached mount state now that the volume is gone.
				delete(b.hostState.blockCIMVolumeHashes, volGUID)
				delete(b.hostState.blockCIMVolumeContainers, volGUID)
			default:
				return fmt.Errorf("unsupported request type %v for WCOWBlockCims", modifyGuestSettingsRequest.RequestType)
			}
			// Send response back to shim
			resp := &prot.ResponseBase{
				Result:     0, // 0 means success
				ActivityID: req.activityID,
			}
			err = b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)
			if err != nil {
				return fmt.Errorf("error sending response to hcsshim: %w", err)
			}
			return nil

		case guestresource.ResourceTypeMappedVirtualDiskForContainerScratch:
			// It doesn't have an enforcement point within this case block, but it has EnforceScratchMountPolicy
			// in ResourceTypeCWCOWCombinedLayers.
			wcowMappedVirtualDisk := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWMappedVirtualDisk)
			log.G(ctx).Tracef("ResourceTypeMappedVirtualDiskForContainerScratch: { %v }", wcowMappedVirtualDisk)

			// Validate the scratch disk mount path matches the expected pattern
			if wcowMappedVirtualDisk.ContainerPath != "" {
				matched, merr := regexp.MatchString(`(?i)^[Cc]:\\mounts\\scsi\\m[0-9]+$`, wcowMappedVirtualDisk.ContainerPath)
				if merr != nil || !matched {
					return fmt.Errorf("scratch disk mount path %q does not match expected pattern c:\\mounts\\scsi\\m<N>",
						wcowMappedVirtualDisk.ContainerPath)
				}
			}

			// This will return the volume path of the mounted scratch.
			// Scratch disk should be >= 30 GB for refs formatter to work.
			// fsFormatter understands only virtualDevObjectPathFormat. Therefore fetch the
			// disk number for the corresponding lun
			var devNumber uint32
			// It could take a few seconds for the attached scsi disk
			// to show up inside the UVM. Therefore adding retry logic
			// with delay here.
			for try := 0; try < 5; try++ {
				time.Sleep(1 * time.Second)
				devNumber, err = windevice.GetDeviceNumberFromControllerLUN(req.ctx,
					0, /* Only one controller allowed in wcow hyperv */
					uint8(wcowMappedVirtualDisk.Lun))
				if err != nil {
					if try == 4 {
						// bail out
						return fmt.Errorf("error getting diskNumber for LUN %d: %w", wcowMappedVirtualDisk.Lun, err)
					}
					continue
				} else {
					log.G(ctx).Tracef("DiskNumber of lun %d is:  %d", wcowMappedVirtualDisk.Lun, devNumber)
					break
				}
			}
			diskPath := fmt.Sprintf(fsformatter.VirtualDevObjectPathFormat, devNumber)
			log.G(ctx).Tracef("diskPath: %v, diskNumber: %v ", diskPath, devNumber)
			mountedVolumePath, err := fsformatter.InvokeFsFormatter(req.ctx, diskPath)
			if err != nil {
				return fmt.Errorf("failed to invoke refsFormatter: %w", err)
			}
			log.G(ctx).Tracef("mountedVolumePath returned from InvokeFsFormatter: %v", mountedVolumePath)

			// Forward the req as is to inbox gcs and let it retreive the volume.
			// While forwarding request to inbox gcs, make sure to replace the
			// resourceType to ResourceTypeMappedVirtualDisk that inbox GCS
			// understands.
			modifyGuestSettingsRequest.ResourceType = guestresource.ResourceTypeMappedVirtualDisk
			modifyRequest.Request = modifyGuestSettingsRequest
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal WCOWMappedVirtualDisk: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest
		case guestresource.ResourceTypeCWCOWCombinedLayers:
			settings := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWCombinedLayers)
			switch modifyGuestSettingsRequest.RequestType {
			case guestrequest.RequestTypeAdd:
				containerID := settings.ContainerID
				log.G(ctx).Tracef("CWCOWCombinedLayers:: ContainerID: %v, ContainerRootPath: %v, Layers: %v, ScratchPath: %v",
					containerID, settings.CombinedLayers.ContainerRootPath, settings.CombinedLayers.Layers, settings.CombinedLayers.ScratchPath)

				// Combined layers are set up once per container. Reject a repeated
				// Add for the same container: otherwise a second Add with a
				// different root would overwrite containerRootPaths[containerID]
				// and leak the previous root's mounted-root entry.
				if b.hostState.HasContainerRoot(containerID) {
					return fmt.Errorf("combined layers already set up for container %q", containerID)
				}

				if matched, merr := regexp.MatchString(`(?i)^[Cc]:\\mounts\\scsi\\m[0-9]+$`, settings.CombinedLayers.ContainerRootPath); merr != nil || !matched {
					return fmt.Errorf("combined-layers container root path %q does not match expected pattern c:\\mounts\\scsi\\m<N>",
						settings.CombinedLayers.ContainerRootPath)
				}

				// The layers size is only one, as this is the volume path
				if len(settings.CombinedLayers.Layers) != 1 {
					return fmt.Errorf("expected exactly one layer in CWCOWCombinedLayers, got %d", len(settings.CombinedLayers.Layers))
				}
				layerPath := settings.CombinedLayers.Layers[0].Path
				guidStr, ok := volumeGUIDFromLayerPath(layerPath)
				if !ok {
					return fmt.Errorf("invalid volumeGUID %s", containerID)
				}
				volGUID, err := guid.FromString(guidStr)
				if err != nil {
					return fmt.Errorf("failed to parse volume GUID %s: %w", guidStr, err)
				}

				// Enforce policy and set up the scratch as a single transaction: if a
				// later step (e.g. mkdir) fails, WithMetadataRollback reverts the
				// policy metadata and we skip the sidecar caches, so policy state
				// can't desync from reality.
				if rberr := b.hostState.securityOptions.PolicyEnforcer.WithMetadataRollback(func() error {
					hashes, haveHashes := b.hostState.blockCIMVolumeHashes[volGUID]
					markVolumeContainer := false
					if haveHashes {
						// Only re-verify if this container hasn't been seen for this volume.
						containers := b.hostState.blockCIMVolumeContainers[volGUID]
						if _, seen := containers[containerID]; !seen {
							hashesToVerify := hashes
							mountedCim := []string{hashes[0]}
							if len(hashes) > 1 {
								hashesToVerify = hashes[1:]
							}
							if err := b.hostState.securityOptions.PolicyEnforcer.EnforceVerifiedCIMsPolicy(ctx, containerID, hashesToVerify, mountedCim, volGUID.String()); err != nil {
								return fmt.Errorf("CIM mount is denied by policy for this container: %w", err)
							}
							log.G(ctx).Tracef("Verified CIM hashes for reused mount volume %s (container %s)", volGUID.String(), containerID)
							markVolumeContainer = true
						}
					}

					if err := b.hostState.securityOptions.PolicyEnforcer.EnforceScratchMountPolicy(ctx, settings.CombinedLayers.ContainerRootPath, true); err != nil {
						return fmt.Errorf("scratch mounting denied by policy: %w", err)
					}

					// The following two folders are expected to be present in the
					// scratch. Since we just formatted it, create them manually.
					sandboxStateDirectory := filepath.Join(settings.CombinedLayers.ContainerRootPath, sandboxStateDirName)
					if err := os.Mkdir(sandboxStateDirectory, 0777); err != nil {
						return fmt.Errorf("failed to create sandboxStateDirectory: %w", err)
					}
					hivesDirectory := filepath.Join(settings.CombinedLayers.ContainerRootPath, hivesDirName)
					if err := os.Mkdir(hivesDirectory, 0777); err != nil {
						return fmt.Errorf("failed to create hivesDirectory: %w", err)
					}

					// Everything succeeded: record the sidecar state. containerRootPaths
					// lets createContainer cross-check the forwarded Storage.Path, and
					// the mounted-root flag lets deleteContainerState refuse deletion
					// until the root is unmounted.
					if markVolumeContainer {
						b.hostState.blockCIMVolumeContainers[volGUID][containerID] = struct{}{}
					}
					b.hostState.containerRootPaths[containerID] = settings.CombinedLayers.ContainerRootPath
					b.hostState.SetContainerRootMounted(settings.CombinedLayers.ContainerRootPath, true)
					return nil
				}); rberr != nil {
					return rberr
				}

			case guestrequest.RequestTypeRemove:
				log.G(ctx).Tracef("CWCOWCombinedLayers: Remove")
				// Refuse to unmount the combined-layers root while a running
				// container still uses it as its rootfs, so the host can't swap a
				// live container's rootfs (cf. LCOW Host.IsOverlayInUse).
				if b.hostState.IsContainerRootInUse(settings.CombinedLayers.ContainerRootPath) {
					return fmt.Errorf("combined-layers unmount denied: container root %q is in use by a running container", settings.CombinedLayers.ContainerRootPath)
				}
				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceScratchUnmountPolicy(ctx, settings.CombinedLayers.ContainerRootPath); err != nil {
					return fmt.Errorf("scratch unmounting denied by policy: %w", err)
				}
				b.hostState.SetContainerRootMounted(settings.CombinedLayers.ContainerRootPath, false)
			default:
				return fmt.Errorf("unsupported request type %v for CWCOWCombinedLayers", modifyGuestSettingsRequest.RequestType)
			}

			// The sidecar enforced policy and staged the scratch here, but the
			// actual union mount/unmount happens in the inbox GCS, so watch its
			// response and fail closed on failure (the staged policy metadata and
			// sidecar caches cannot be reverted).
			monitorResponse = true

			// Reconstruct WCOWCombinedLayers{} req before forwarding to GCS
			// as GCS does not understand ResourceTypeCWCOWCombinedLayers
			modifyGuestSettingsRequest.ResourceType = guestresource.ResourceTypeCombinedLayers
			modifyGuestSettingsRequest.Settings = settings.CombinedLayers
			modifyRequest.Request = modifyGuestSettingsRequest
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal rpcModifySettings: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest

		default:
			// Invalid request
			return fmt.Errorf("invalid modifySettingsRequest: %v", guestResourceType)
		}
	}

	if monitorResponse {
		b.monitorInboxResponse(req.header.ID)
	}
	b.forwardRequestToGcs(req)
	return nil
}
