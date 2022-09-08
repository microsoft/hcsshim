//go:build windows
// +build windows

package hcsoci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/clone"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var (
	lcowRootInUVM = guestpath.LCOWRootPrefixInUVM + "/%s"
	wcowRootInUVM = guestpath.WCOWRootPrefixInUVM + "/%s"
)

// CreateOptions are the set of fields used to call CreateContainer().
// Note: In the spec, the LayerFolders must be arranged in the same way in which
// moby configures them: layern, layern-1,...,layer2,layer1,scratch
// where layer1 is the base read-only layer, layern is the top-most read-only
// layer, and scratch is the RW layer. This is for historical reasons only.
type CreateOptions struct {
	// Common parameters
	ID               string             // Identifier for the container
	Owner            string             // Specifies the owner. Defaults to executable name.
	Spec             *specs.Spec        // Definition of the container or utility VM being created
	SchemaVersion    *hcsschema.Version // Requested Schema Version. Defaults to v2 for RS5, v1 for RS1..RS4
	HostingSystem    *uvm.UtilityVM     // Utility or service VM in which the container is to be created.
	NetworkNamespace string             // Host network namespace to use (overrides anything in the spec)

	// This is an advanced debugging parameter. It allows for diagnosability by leaving a containers
	// resources allocated in case of a failure. Thus you would be able to use tools such as hcsdiag
	// to look at the state of a utility VM to see what resources were allocated. Obviously the caller
	// must a) not tear down the utility VM on failure (or pause in some way) and b) is responsible for
	// performing the ReleaseResources() call themselves.
	DoNotReleaseResourcesOnFailure bool

	// ScaleCPULimitsToSandbox indicates that the container CPU limits should be adjusted to account
	// for the difference in CPU count between the host and the UVM.
	ScaleCPULimitsToSandbox bool
}

// createOptionsInternal is the set of user-supplied create options, but includes internal
// fields for processing the request once user-supplied stuff has been validated.
type createOptionsInternal struct {
	*CreateOptions

	actualSchemaVersion    *hcsschema.Version // Calculated based on Windows build and optional caller-supplied override
	actualID               string             // Identifier for the container
	actualOwner            string             // Owner for the container
	actualNetworkNamespace string
	ccgState               *hcsschema.ContainerCredentialGuardState // Container Credential Guard information to be attached to HCS container document
	isTemplate             bool                                     // Are we going to save this container as a template
	templateID             string                                   // Template ID of the template from which this container is being cloned
}

// compares two slices of strings and returns true if they are same, returns false otherwise.
// The elements in the slices don't have to be in the same order for them to be equal.
func cmpSlices(s1, s2 []string) bool {
	equal := (len(s1) == len(s2))
	for i := 0; equal && i < len(s1); i++ {
		found := false
		for j := 0; !found && j < len(s2); j++ {
			found = (s1[i] == s2[j])
		}
		equal = equal && found
	}
	return equal
}

// verifyCloneContainerSpecs compares the container creation spec provided during the template container
// creation and the spec provided during cloned container creation and checks that all the fields match
// (except for the certain fields that are allowed to be different).
func verifyCloneContainerSpecs(templateSpec, cloneSpec *specs.Spec) error {
	// Following fields can be different in the template and clone specs.
	// 1. Process
	// 2. Annotations - Only the template/cloning related annotations can be different.
	// 3. Windows.LayerFolders - Only the last i.e scratch layer can be different.

	if templateSpec.Version != cloneSpec.Version {
		return fmt.Errorf("OCI Runtime Spec version of template (%s) doesn't match with the Spec version of clone (%s)", templateSpec.Version, cloneSpec.Version)
	}

	// for annotations check that the values of memory & cpu annotations are same
	if templateSpec.Annotations[annotations.ContainerMemorySizeInMB] != cloneSpec.Annotations[annotations.ContainerMemorySizeInMB] {
		return errors.New("memory size limit for template and clone containers can not be different")
	}
	if templateSpec.Annotations[annotations.ContainerProcessorCount] != cloneSpec.Annotations[annotations.ContainerProcessorCount] {
		return errors.New("processor count for template and clone containers can not be different")
	}
	if templateSpec.Annotations[annotations.ContainerProcessorLimit] != cloneSpec.Annotations[annotations.ContainerProcessorLimit] {
		return errors.New("processor limit for template and clone containers can not be different")
	}

	// LayerFolders should be identical except for the last element.
	if !cmpSlices(templateSpec.Windows.LayerFolders[:len(templateSpec.Windows.LayerFolders)-1], cloneSpec.Windows.LayerFolders[:len(cloneSpec.Windows.LayerFolders)-1]) {
		return errors.New("layers provided for template container and clone container don't match. Check the image specified in container config")
	}

	if !reflect.DeepEqual(templateSpec.Windows.HyperV, cloneSpec.Windows.HyperV) {
		return errors.New("HyperV spec for template and clone containers can not be different")
	}

	if templateSpec.Windows.Network.AllowUnqualifiedDNSQuery != cloneSpec.Windows.Network.AllowUnqualifiedDNSQuery {
		return errors.New("different values for allow unqualified DNS query can not be provided for template and clones")
	}
	if templateSpec.Windows.Network.NetworkSharedContainerName != cloneSpec.Windows.Network.NetworkSharedContainerName {
		return errors.New("different network shared name can not be provided for template and clones")
	}
	if !cmpSlices(templateSpec.Windows.Network.DNSSearchList, cloneSpec.Windows.Network.DNSSearchList) {
		return errors.New("different DNS search list can not be provided for template and clones")
	}

	return nil
}

func validateContainerConfig(ctx context.Context, coi *createOptionsInternal) error {
	if coi.HostingSystem != nil && coi.HostingSystem.IsTemplate && !coi.isTemplate {
		return fmt.Errorf("only a template container can be created inside a template pod. Any other combination is not valid")
	}

	if coi.HostingSystem != nil && coi.templateID != "" && !coi.HostingSystem.IsClone {
		return fmt.Errorf("a container can not be cloned inside a non cloned POD")
	}

	if coi.templateID != "" {
		// verify that the configurations provided for the template for
		// this clone are same.
		tc, err := clone.FetchTemplateConfig(ctx, coi.HostingSystem.TemplateID)
		if err != nil {
			return fmt.Errorf("config validation failed : %s", err)
		}
		if err := verifyCloneContainerSpecs(&tc.TemplateContainerSpec, coi.Spec); err != nil {
			return err
		}
	}

	if coi.HostingSystem != nil && coi.HostingSystem.IsTemplate {
		if len(coi.Spec.Windows.Devices) != 0 {
			return fmt.Errorf("mapped Devices are not supported for template containers")
		}

		if _, ok := coi.Spec.Windows.CredentialSpec.(string); ok {
			return fmt.Errorf("gmsa specifications are not supported for template containers")
		}

		if coi.Spec.Windows.Servicing {
			return fmt.Errorf("template containers can't be started in servicing mode")
		}

		// check that no mounts are specified.
		if len(coi.Spec.Mounts) > 0 {
			return fmt.Errorf("user specified mounts are not permitted for template containers")
		}
	}

	// check if gMSA is disabled
	if coi.Spec.Windows != nil {
		disableGMSA := oci.ParseAnnotationsDisableGMSA(ctx, coi.Spec)
		if _, ok := coi.Spec.Windows.CredentialSpec.(string); ok && disableGMSA {
			return fmt.Errorf("gMSA credentials are disabled: %w", hcs.ErrOperationDenied)
		}
	}

	return nil
}

func initializeCreateOptions(ctx context.Context, createOptions *CreateOptions) (*createOptionsInternal, error) {
	coi := &createOptionsInternal{
		CreateOptions: createOptions,
		actualID:      createOptions.ID,
		actualOwner:   createOptions.Owner,
	}

	if coi.Spec == nil {
		return nil, fmt.Errorf("spec must be supplied")
	}

	// Defaults if omitted by caller.
	if coi.actualID == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		coi.actualID = g.String()
	}
	if coi.actualOwner == "" {
		coi.actualOwner = filepath.Base(os.Args[0])
	}

	if coi.HostingSystem != nil {
		// By definition, a hosting system can only be supplied for a v2 Xenon.
		coi.actualSchemaVersion = schemaversion.SchemaV21()
	} else {
		coi.actualSchemaVersion = schemaversion.DetermineSchemaVersion(coi.SchemaVersion)
	}

	coi.isTemplate = oci.ParseAnnotationsSaveAsTemplate(ctx, createOptions.Spec)
	coi.templateID = oci.ParseAnnotationsTemplateID(ctx, createOptions.Spec)

	log.G(ctx).WithFields(logrus.Fields{
		"options": fmt.Sprintf("%+v", createOptions),
		"schema":  coi.actualSchemaVersion,
	}).Debug("hcsshim::initializeCreateOptions")

	return coi, nil
}

// configureSandboxNetwork creates a new network namespace for the pod (sandbox)
// if required and then adds that namespace to the pod.
func configureSandboxNetwork(ctx context.Context, coi *createOptionsInternal, r *resources.Resources, ct oci.KubernetesContainerType) error {
	if coi.NetworkNamespace != "" {
		r.SetNetNS(coi.NetworkNamespace)
	} else {
		err := createNetworkNamespace(ctx, coi, r)
		if err != nil {
			return err
		}
	}
	coi.actualNetworkNamespace = r.NetNS()

	if coi.HostingSystem != nil {
		// Only add the network namespace to a standalone or sandbox
		// container but not a workload container in a sandbox that inherits
		// the namespace.
		if ct == oci.KubernetesContainerTypeNone || ct == oci.KubernetesContainerTypeSandbox {
			if err := coi.HostingSystem.ConfigureNetworking(ctx, coi.actualNetworkNamespace); err != nil {
				// No network setup type was specified for this UVM. Create and assign one here unless
				// we received a different error.
				if err == uvm.ErrNoNetworkSetup {
					if err := coi.HostingSystem.CreateAndAssignNetworkSetup(ctx, "", ""); err != nil {
						return err
					}
					if err := coi.HostingSystem.ConfigureNetworking(ctx, coi.actualNetworkNamespace); err != nil {
						return err
					}
				} else {
					return err
				}
			}
			r.SetAddedNetNSToVM(true)
		}
	}

	return nil
}

// CreateContainer creates a container. It can cope with a  wide variety of
// scenarios, including v1 HCS schema calls, as well as more complex v2 HCS schema
// calls. Note we always return the resources that have been allocated, even in the
// case of an error. This provides support for the debugging option not to
// release the resources on failure, so that the client can make the necessary
// call to release resources that have been allocated as part of calling this function.
func CreateContainer(ctx context.Context, createOptions *CreateOptions) (_ cow.Container, _ *resources.Resources, err error) {
	coi, err := initializeCreateOptions(ctx, createOptions)
	if err != nil {
		return nil, nil, err
	}

	if err := validateContainerConfig(ctx, coi); err != nil {
		return nil, nil, fmt.Errorf("container config validation failed: %s", err)
	}

	r := resources.NewContainerResources(createOptions.ID)
	defer func() {
		if err != nil {
			if !coi.DoNotReleaseResourcesOnFailure {
				_ = resources.ReleaseResources(ctx, r, coi.HostingSystem, true)
			}
		}
	}()

	if coi.HostingSystem != nil {
		if coi.Spec.Linux != nil {
			r.SetContainerRootInUVM(fmt.Sprintf(lcowRootInUVM, createOptions.ID))
		} else {
			n := coi.HostingSystem.ContainerCounter()
			r.SetContainerRootInUVM(fmt.Sprintf(wcowRootInUVM, strconv.FormatUint(n, 16)))
		}
		// install kernel drivers if necessary.
		// do this before network setup in case any of the drivers requested are
		// network drivers
		driverClosers, err := addSpecGuestDrivers(ctx, coi.HostingSystem, coi.Spec.Annotations)
		if err != nil {
			return nil, r, err
		}
		r.Add(driverClosers...)
	}

	ct, _, err := oci.GetSandboxTypeAndID(coi.Spec.Annotations)
	if err != nil {
		return nil, r, err
	}
	isSandbox := ct == oci.KubernetesContainerTypeSandbox

	// Create a network namespace if necessary.
	if coi.Spec.Windows != nil &&
		coi.Spec.Windows.Network != nil &&
		schemaversion.IsV21(coi.actualSchemaVersion) {
		err = configureSandboxNetwork(ctx, coi, r, ct)
		if err != nil {
			return nil, r, fmt.Errorf("failure while creating namespace for container: %s", err)
		}
	}

	var hcsDocument, gcsDocument interface{}
	log.G(ctx).Debug("hcsshim::CreateContainer allocating resources")
	if coi.Spec.Linux != nil {
		if schemaversion.IsV10(coi.actualSchemaVersion) {
			return nil, r, errors.New("LCOW v1 not supported")
		}
		log.G(ctx).Debug("hcsshim::CreateContainer allocateLinuxResources")
		err = allocateLinuxResources(ctx, coi, r, isSandbox)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
			return nil, r, err
		}
		gcsDocument, err = createLinuxContainerDocument(ctx, coi, r.ContainerRootInUVM(), r.LcowScratchPath())
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
			return nil, r, err
		}
	} else {
		err = allocateWindowsResources(ctx, coi, r, isSandbox)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
			return nil, r, err
		}
		log.G(ctx).Debug("hcsshim::CreateContainer creating container document")
		v1, v2, err := createWindowsContainerDocument(ctx, coi)
		if err != nil {
			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
			return nil, r, err
		}

		if schemaversion.IsV10(coi.actualSchemaVersion) {
			// v1 Argon or Xenon. Pass the document directly to HCS.
			hcsDocument = v1
		} else if coi.HostingSystem != nil {
			// v2 Xenon. Pass the container object to the UVM.
			gcsDocument = &hcsschema.HostedSystem{
				SchemaVersion: schemaversion.SchemaV21(),
				Container:     v2,
			}
		} else {
			// v2 Argon. Pass the container object to the HCS.
			hcsDocument = &hcsschema.ComputeSystem{
				Owner:                             coi.actualOwner,
				SchemaVersion:                     schemaversion.SchemaV21(),
				ShouldTerminateOnLastHandleClosed: true,
				Container:                         v2,
			}
		}
	}

	log.G(ctx).Debug("hcsshim::CreateContainer creating compute system")
	if gcsDocument != nil {
		c, err := coi.HostingSystem.CreateContainer(ctx, coi.actualID, gcsDocument)
		if err != nil {
			return nil, r, err
		}
		return c, r, nil
	}

	system, err := hcs.CreateComputeSystem(ctx, coi.actualID, hcsDocument)
	if err != nil {
		return nil, r, err
	}
	return system, r, nil
}

// CloneContainer is similar to CreateContainer but it does not add layers or namespace like
// CreateContainer does. Also, instead of sending create container request it sends a modify
// request to an existing container. CloneContainer only works for WCOW.
func CloneContainer(ctx context.Context, createOptions *CreateOptions) (_ cow.Container, _ *resources.Resources, err error) {
	coi, err := initializeCreateOptions(ctx, createOptions)
	if err != nil {
		return nil, nil, err
	}

	if err := validateContainerConfig(ctx, coi); err != nil {
		return nil, nil, err
	}

	if coi.Spec.Windows == nil || coi.HostingSystem == nil {
		return nil, nil, fmt.Errorf("CloneContainer is only supported for Hyper-v isolated WCOW ")
	}

	r := resources.NewContainerResources(createOptions.ID)
	defer func() {
		if err != nil {
			if !coi.DoNotReleaseResourcesOnFailure {
				_ = resources.ReleaseResources(ctx, r, coi.HostingSystem, true)
			}
		}
	}()

	if coi.HostingSystem != nil {
		n := coi.HostingSystem.ContainerCounter()
		if coi.Spec.Linux != nil {
			r.SetContainerRootInUVM(fmt.Sprintf(lcowRootInUVM, createOptions.ID))
		} else {
			r.SetContainerRootInUVM(fmt.Sprintf(wcowRootInUVM, strconv.FormatUint(n, 16)))
		}
	}

	if err = setupMounts(ctx, coi, r); err != nil {
		return nil, r, err
	}

	mounts, err := createMountsConfig(ctx, coi)
	if err != nil {
		return nil, r, err
	}

	c, err := coi.HostingSystem.CloneContainer(ctx, coi.actualID)
	if err != nil {
		return nil, r, err
	}

	// Everything that is usually added to the container during the createContainer
	// request (via the gcsDocument) must be hot added here.
	if err := addMountsToClone(ctx, c, mounts); err != nil {
		return nil, r, err
	}

	return c, r, nil
}

// isV2Xenon returns true if the create options are for a HCS schema V2 xenon container
// with a hosting VM
func (coi *createOptionsInternal) isV2Xenon() bool {
	return schemaversion.IsV21(coi.actualSchemaVersion) && coi.HostingSystem != nil
}

// isV1Xenon returns true if the create options are for a HCS schema V1 xenon container
// with a hosting VM
func (coi *createOptionsInternal) isV1Xenon() bool {
	return schemaversion.IsV10(coi.actualSchemaVersion) && coi.HostingSystem != nil
}

// isV2Argon returns true if the create options are for a HCS schema V2 argon container
// which should have no hosting VM
func (coi *createOptionsInternal) isV2Argon() bool {
	return schemaversion.IsV21(coi.actualSchemaVersion) && coi.HostingSystem == nil
}

// isV1Argon returns true if the create options are for a HCS schema V1 argon container
// which should have no hyperv settings
func (coi *createOptionsInternal) isV1Argon() bool {
	return schemaversion.IsV10(coi.actualSchemaVersion) && coi.Spec.Windows.HyperV == nil
}

func (coi *createOptionsInternal) hasWindowsAssignedDevices() bool {
	return (coi.Spec.Windows != nil) && (coi.Spec.Windows.Devices != nil) &&
		(len(coi.Spec.Windows.Devices) > 0)
}
