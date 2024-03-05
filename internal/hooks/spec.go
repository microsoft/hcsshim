package hooks

import (
	"fmt"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// Note: The below type definition as well as constants have been copied from
// https://github.com/opencontainers/runc/blob/master/libcontainer/configs/config.go.
// This is done to not introduce a direct dependency on runc, which would complicate
// integration with windows.
type HookName string

const (

	// Prestart commands are executed after the container namespaces are created,
	// but before the user supplied command is executed from init.
	// Prestart commands are called in the Runtime namespace.
	//
	// Deprecated: use [CreateRuntime] instead.
	Prestart HookName = "prestart"

	// CreateRuntime commands MUST be called as part of the create operation after
	// the runtime environment has been created but before the pivot_root has been executed.
	// CreateRuntime is called immediately after the deprecated Prestart hook.
	// CreateRuntime commands are called in the Runtime Namespace.
	CreateRuntime HookName = "createRuntime"
)

// NewOCIHook creates a new oci.Hook with given parameters.
func NewOCIHook(path string, args, env []string) oci.Hook {
	return oci.Hook{
		Path: path,
		Args: args,
		Env:  env,
	}
}

// AddOCIHook adds oci.Hook of the given hook name to spec.
func AddOCIHook(spec *oci.Spec, hn HookName, hk oci.Hook) error {
	if spec.Hooks == nil {
		spec.Hooks = &oci.Hooks{}
	}
	switch hn {
	case CreateRuntime:
		spec.Hooks.CreateRuntime = append(spec.Hooks.CreateRuntime, hk)
	default:
		return fmt.Errorf("hook %q is not supported", hn)
	}
	return nil
}
