package oci

import (
	"fmt"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

// KubernetesContainerType defines the valid types of the
// `annotations.KubernetesContainerType` annotation.
type KubernetesContainerType string

const (
	// KubernetesContainerTypeNone is only valid when
	// `annotations.KubernetesContainerType` is not set.
	KubernetesContainerTypeNone KubernetesContainerType = ""
	// KubernetesContainerTypeContainer is valid when
	// `annotations.KubernetesContainerType == "container"`.
	KubernetesContainerTypeContainer KubernetesContainerType = "container"
	// KubernetesContainerTypeSandbox is valid when
	// `annotations.KubernetesContainerType == "sandbox"`.
	KubernetesContainerTypeSandbox KubernetesContainerType = "sandbox"
)

// GetSandboxTypeAndID parses `specAnnotations` searching for the
// `KubernetesContainerTypeAnnotation` and `KubernetesSandboxIDAnnotation`
// annotations and if found validates the set before returning.
func GetSandboxTypeAndID(specAnnotations map[string]string) (KubernetesContainerType, string, error) {
	var ct KubernetesContainerType
	if t, ok := specAnnotations[annotations.KubernetesContainerType]; ok {
		switch t {
		case string(KubernetesContainerTypeContainer):
			ct = KubernetesContainerTypeContainer
		case string(KubernetesContainerTypeSandbox):
			ct = KubernetesContainerTypeSandbox
		default:
			return KubernetesContainerTypeNone, "", fmt.Errorf("invalid '%s': '%s'", annotations.KubernetesContainerType, t)
		}
	}

	id := specAnnotations[annotations.KubernetesSandboxID]

	switch ct {
	case KubernetesContainerTypeContainer, KubernetesContainerTypeSandbox:
		if id == "" {
			return KubernetesContainerTypeNone, "", fmt.Errorf("cannot specify '%s' without '%s'", annotations.KubernetesContainerType, annotations.KubernetesSandboxID)
		}
	default:
		if id != "" {
			return KubernetesContainerTypeNone, "", fmt.Errorf("cannot specify '%s' without '%s'", annotations.KubernetesSandboxID, annotations.KubernetesContainerType)
		}
	}
	return ct, id, nil
}
