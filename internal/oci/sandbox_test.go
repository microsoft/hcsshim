package oci

import (
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func Test_GetSandboxTypeAndID_TypeContainer_NoID_Failure(t *testing.T) {
	a := map[string]string{
		annotations.KubernetesContainerType: "container",
	}
	ct, id, err := GetSandboxTypeAndID(a)
	if err == nil {
		t.Fatal("should have failed with error")
	}
	if ct != KubernetesContainerTypeNone {
		t.Fatal("should of returned KubernetesContainerTypeNone")
	}
	if id != "" {
		t.Fatal("should of returned empty id")
	}
}

func Test_GetSandboxTypeAndID_TypeSandbox_NoID_Failure(t *testing.T) {
	a := map[string]string{
		annotations.KubernetesContainerType: "sandbox",
	}
	ct, id, err := GetSandboxTypeAndID(a)
	if err == nil {
		t.Fatal("should have failed with error")
	}
	if ct != KubernetesContainerTypeNone {
		t.Fatal("should of returned KubernetesContainerTypeNone")
	}
	if id != "" {
		t.Fatal("should of returned empty id")
	}
}

func Test_GetSandboxTypeAndID_NoType_ValidID_Failure(t *testing.T) {
	a := map[string]string{
		annotations.KubernetesSandboxID: t.Name(),
	}
	ct, id, err := GetSandboxTypeAndID(a)
	if err == nil {
		t.Fatal("should have failed with error")
	}
	if ct != KubernetesContainerTypeNone {
		t.Fatal("should of returned KubernetesContainerTypeNone")
	}
	if id != "" {
		t.Fatal("should of returned empty id")
	}
}

func Test_GetSandboxTypeAndID_NoAnnotations_Success(t *testing.T) {
	ct, id, err := GetSandboxTypeAndID(nil)
	if err != nil {
		t.Fatalf("should not of failed with error: %v", err)
	}
	if ct != KubernetesContainerTypeNone {
		t.Fatal("should of returned KubernetesContainerTypeNone")
	}
	if id != "" {
		t.Fatal("should of returned empty id")
	}
}

func Test_GetSandboxTypeAndID_TypeContainer_ValidID_Success(t *testing.T) {
	a := map[string]string{
		annotations.KubernetesContainerType: "container",
		annotations.KubernetesSandboxID:     t.Name(),
	}
	ct, id, err := GetSandboxTypeAndID(a)
	if err != nil {
		t.Fatalf("should not of failed with error: %v", err)
	}
	if ct != KubernetesContainerTypeContainer {
		t.Fatal("should of returned KubernetesContainerTypeContainer")
	}
	if id != t.Name() {
		t.Fatalf("should of returned valid id got: %s", id)
	}
}

func Test_GetSandboxTypeAndID_TypeSandbox_ValidID_Success(t *testing.T) {
	a := map[string]string{
		annotations.KubernetesContainerType: "sandbox",
		annotations.KubernetesSandboxID:     t.Name(),
	}
	ct, id, err := GetSandboxTypeAndID(a)
	if err != nil {
		t.Fatalf("should not of failed with error: %v", err)
	}
	if ct != KubernetesContainerTypeSandbox {
		t.Fatal("should of returned KubernetesContainerTypeSandbox")
	}
	if id != t.Name() {
		t.Fatalf("should of returned valid id got: %s", id)
	}
}
