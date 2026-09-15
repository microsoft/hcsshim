//go:build windows
// +build windows

package securitypolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils/etw"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

//nolint:unused
const osType = "windows"

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return ""
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return ""
}

func GetAllUserInfo(process *oci.Process, rootPath string) (IDName, []IDName, string, error) {
	return IDName{}, []IDName{}, "", nil
}

// DefaultCRIMounts returns default mounts added to windows spec by containerD.
func DefaultCRIMounts() []oci.Mount {
	return []oci.Mount{}
}

// DefaultCRIPrivilegedMounts returns a slice of mounts which are added to the
// windows container spec when a container runs in a privileged mode.
func DefaultCRIPrivilegedMounts() []oci.Mount {
	return []oci.Mount{}
}

// WCOWContainerPolicyState contains the sidecar state needed to bind the
// host-provided HostedSystem storage to the layers enforced for a container.
type WCOWContainerPolicyState struct {
	ContainerRootPath        string
	VerifiedLayerVolumeGUIDs []string
}

// HasSecurityPolicy reports whether an enforcer has an encoded security policy.
func HasSecurityPolicy(enforcer SecurityPolicyEnforcer) bool {
	return len(enforcer.EncodedSecurityPolicy()) > 0
}

// EnforceWCOWCreateContainerPolicy applies the policy decisions specific to a
// WCOW create request and updates the spec and HostedSystem with kept values.
func EnforceWCOWCreateContainerPolicy(
	ctx context.Context,
	enforcer SecurityPolicyEnforcer,
	containerID string,
	spec *oci.Spec,
	container *hcsschema.Container,
	state WCOWContainerPolicyState,
) (bool, error) {
	if HasSecurityPolicy(enforcer) {
		if err := validateWCOWContainerID(containerID); err != nil {
			return false, err
		}
		if err := denyUnsupportedWCOWContainerFields(container); err != nil {
			return false, err
		}
	}

	if container != nil && container.RegistryChanges != nil {
		defaultValues, nonDefaultChanges := splitWCOWRegistryChanges(container.RegistryChanges)
		if len(nonDefaultChanges.AddValues) > 0 || len(nonDefaultChanges.DeleteKeys) > 0 {
			kept, err := enforcer.EnforceRegistryChangesPolicy(ctx, containerID, nonDefaultChanges)
			if err != nil {
				return false, fmt.Errorf("registry changes are denied by policy: %w", err)
			}
			container.RegistryChanges.AddValues, container.RegistryChanges.DeleteKeys =
				mergeKeptWCOWRegistryChanges(defaultValues, kept)
		}
	}

	user := IDName{Name: spec.Process.User.Username}
	envToKeep, _, allowStdio, err := enforcer.EnforceCreateContainerPolicyV2(
		ctx,
		containerID,
		spec.Process.Args,
		spec.Process.Env,
		spec.Process.Cwd,
		spec.Mounts,
		user,
		&CreateContainerOptions{},
	)
	if err != nil {
		return false, fmt.Errorf("create container is denied by policy: %w", err)
	}
	if envToKeep != nil {
		spec.Process.Env = []string(envToKeep)
	}

	if HasSecurityPolicy(enforcer) {
		if err := reconcileWCOWHostedSystemMounts(spec.Mounts, container); err != nil {
			return false, err
		}
		if err := reconcileWCOWHostedSystemStorage(containerID, container, state); err != nil {
			return false, err
		}
	}

	return allowStdio, nil
}

// EnforceWCOWLogProviders validates and filters the host-provided ETW provider
// model, returning only providers approved by the configured enforcer.
func EnforceWCOWLogProviders(
	ctx context.Context,
	enforcer SecurityPolicyEnforcer,
	sources etw.LogSourcesInfo,
) (etw.LogSourcesInfo, error) {
	if HasSecurityPolicy(enforcer) {
		if err := validateWCOWLogProviders(sources.LogConfig.Sources); err != nil {
			return etw.LogSourcesInfo{}, err
		}
	}

	var requestedNames []string
	for _, source := range sources.LogConfig.Sources {
		for _, provider := range source.Providers {
			requestedNames = append(requestedNames, provider.ProviderName)
		}
	}

	keptNames, err := enforcer.EnforceLogProviderPolicy(ctx, requestedNames)
	if err != nil {
		return etw.LogSourcesInfo{}, err
	}
	return filterWCOWLogSourcesToAllowed(ctx, sources, requestedNames, keptNames), nil
}

var wcowContainerIDRegex = regexp.MustCompile(`^[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*$`)

func validateWCOWContainerID(id string) error {
	if !wcowContainerIDRegex.MatchString(id) {
		return fmt.Errorf("invalid container ID %q", id)
	}
	return nil
}

func denyUnsupportedWCOWContainerFields(container *hcsschema.Container) error {
	if container == nil {
		return nil
	}

	containerJSON, _ := json.Marshal(container)
	if container.HvSocket != nil {
		return fmt.Errorf("HvSocket is not supported. Container: %s", containerJSON)
	}
	if container.ContainerCredentialGuard != nil {
		return fmt.Errorf("ContainerCredentialGuard is not supported. Container: %s", containerJSON)
	}
	if len(container.AssignedDevices) > 0 {
		return fmt.Errorf("AssignedDevices is not supported. Container: %s", containerJSON)
	}
	if container.AdditionalDeviceNamespace != nil {
		return fmt.Errorf("AdditionalDeviceNamespace is not supported. Container: %s", containerJSON)
	}
	return nil
}

var defaultWCOWRegistryValues = []hcsschema.RegistryValue{
	{
		Key: &hcsschema.RegistryKey{
			Hive: hcsschema.RegistryHive_SYSTEM,
			Name: "ControlSet001\\Control",
		},
		Name:        "WaitToKillServiceTimeout",
		StringValue: strconv.Itoa(math.MaxInt32),
		Type_:       hcsschema.RegistryValueType_STRING,
	},
}

func splitWCOWRegistryChanges(changes *hcsschema.RegistryChanges) (
	defaultValues []hcsschema.RegistryValue,
	nonDefaultChanges *hcsschema.RegistryChanges,
) {
	var nonDefaultValues []hcsschema.RegistryValue
	for _, value := range changes.AddValues {
		if slices.ContainsFunc(defaultWCOWRegistryValues, func(defaultValue hcsschema.RegistryValue) bool {
			return reflect.DeepEqual(defaultValue, value)
		}) {
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

func mergeKeptWCOWRegistryChanges(
	defaultValues []hcsschema.RegistryValue,
	kept interface{},
) ([]hcsschema.RegistryValue, []hcsschema.RegistryKey) {
	var keptNonDefault []hcsschema.RegistryValue
	var keptDeleteKeys []hcsschema.RegistryKey
	if changes, ok := kept.(*hcsschema.RegistryChanges); ok && changes != nil {
		keptNonDefault = changes.AddValues
		keptDeleteKeys = changes.DeleteKeys
	}

	newValues := make([]hcsschema.RegistryValue, 0, len(defaultValues)+len(keptNonDefault))
	newValues = append(newValues, defaultValues...)
	newValues = append(newValues, keptNonDefault...)
	return newValues, keptDeleteKeys
}

const wcowNamedPipePrefix = `\\.\pipe\`

func wcowMountReadOnly(options []string) bool {
	for _, option := range options {
		if strings.EqualFold(option, "ro") {
			return true
		}
	}
	return false
}

func reconcileWCOWHostedSystemMounts(mounts []oci.Mount, container *hcsschema.Container) error {
	if container == nil {
		return nil
	}

	for _, mappedDirectory := range container.MappedDirectories {
		matched := false
		for _, mount := range mounts {
			if strings.HasPrefix(mount.Destination, wcowNamedPipePrefix) {
				continue
			}
			if mount.Destination == mappedDirectory.ContainerPath &&
				wcowMountReadOnly(mount.Options) == mappedDirectory.ReadOnly {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf(
				"mapped directory %q (readOnly=%v) does not match any enforced spec mount",
				mappedDirectory.ContainerPath,
				mappedDirectory.ReadOnly,
			)
		}
	}

	for _, mappedPipe := range container.MappedPipes {
		matched := false
		for _, mount := range mounts {
			if !strings.HasPrefix(mount.Destination, wcowNamedPipePrefix) {
				continue
			}
			if strings.TrimPrefix(mount.Destination, wcowNamedPipePrefix) == mappedPipe.ContainerPipeName {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("mapped pipe %q does not match any enforced spec mount", mappedPipe.ContainerPipeName)
		}
	}

	return nil
}

func reconcileWCOWHostedSystemStorage(
	containerID string,
	container *hcsschema.Container,
	state WCOWContainerPolicyState,
) error {
	if container == nil || container.Storage == nil {
		return fmt.Errorf("container storage is missing")
	}
	if state.ContainerRootPath == "" {
		return fmt.Errorf("no container root path recorded for container %s", containerID)
	}
	if !strings.EqualFold(container.Storage.Path, state.ContainerRootPath) {
		return fmt.Errorf(
			"storage path %q does not match the enforced container root path %q",
			container.Storage.Path,
			state.ContainerRootPath,
		)
	}
	if len(container.Storage.Layers) != 1 {
		return fmt.Errorf("expected exactly one storage layer, got %d", len(container.Storage.Layers))
	}

	layerPath := container.Storage.Layers[0].Path
	volumeID, ok := strings.CutPrefix(layerPath, `\\?\Volume{`)
	if !ok {
		return fmt.Errorf("storage layer path %q is not a volume path", layerPath)
	}
	volumeID, ok = strings.CutSuffix(volumeID, `}\`)
	if !ok {
		return fmt.Errorf("storage layer path %q is not a volume path", layerPath)
	}
	volumeGUID, err := guid.FromString(volumeID)
	if err != nil {
		return fmt.Errorf("invalid storage layer volume GUID %q: %w", volumeID, err)
	}
	for _, verified := range state.VerifiedLayerVolumeGUIDs {
		if strings.EqualFold(verified, volumeGUID.String()) {
			return nil
		}
	}
	return fmt.Errorf("storage layer volume %s was not verified for container %s", volumeGUID, containerID)
}

func validateWCOWLogProviders(sources []etw.Source) error {
	for _, source := range sources {
		for _, provider := range source.Providers {
			if provider.ProviderName == "" {
				return fmt.Errorf("provider with no name is not allowed (GUID %q)", provider.ProviderGUID)
			}
			if provider.ProviderGUID == "" {
				continue
			}
			wellKnownGUID := etw.GetProviderGUIDFromName(provider.ProviderName)
			if wellKnownGUID == "" {
				return fmt.Errorf(
					"provider %q: name not in well-known ETW map; cannot verify supplied GUID %q",
					provider.ProviderName,
					provider.ProviderGUID,
				)
			}
			suppliedGUID := strings.TrimSpace(provider.ProviderGUID)
			suppliedGUID = strings.TrimPrefix(suppliedGUID, "{")
			suppliedGUID = strings.TrimSuffix(suppliedGUID, "}")
			supplied, err := guid.FromString(suppliedGUID)
			if err != nil {
				return fmt.Errorf("provider %q: invalid GUID %q: %w", provider.ProviderName, provider.ProviderGUID, err)
			}
			if !strings.EqualFold(supplied.String(), wellKnownGUID) {
				return fmt.Errorf(
					"provider %q: supplied GUID %q does not match well-known GUID %q",
					provider.ProviderName,
					provider.ProviderGUID,
					wellKnownGUID,
				)
			}
		}
	}
	return nil
}

func filterWCOWLogSourcesToAllowed(
	ctx context.Context,
	sources etw.LogSourcesInfo,
	requestedNames []string,
	keptNames []string,
) etw.LogSourcesInfo {
	keepSet := make(map[string]struct{}, len(keptNames))
	for _, name := range keptNames {
		keepSet[name] = struct{}{}
	}

	dropped := make([]string, 0)
	seenDropped := make(map[string]struct{})
	for i := range sources.LogConfig.Sources {
		source := &sources.LogConfig.Sources[i]
		filtered := make([]etw.EtwProvider, 0, len(source.Providers))
		for _, provider := range source.Providers {
			if _, ok := keepSet[provider.ProviderName]; ok {
				filtered = append(filtered, provider)
				continue
			}
			if _, seen := seenDropped[provider.ProviderName]; !seen {
				seenDropped[provider.ProviderName] = struct{}{}
				dropped = append(dropped, provider.ProviderName)
			}
		}
		source.Providers = filtered
	}

	if len(dropped) > 0 {
		log.G(ctx).WithFields(logrus.Fields{
			"requested": requestedNames,
			"kept":      keptNames,
			"dropped":   dropped,
		}).Warn("log providers trimmed by policy (allow_log_provider_dropping)")
	}
	return sources
}
