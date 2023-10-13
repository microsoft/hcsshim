// This package contains bindings to the Host Compute Schema (HCS) [JSON API].
//
// Several HCS structs take arbitrary objects, repsented in the schema as `Any` (eg, see
// [ModifySettingRequest] and [HcsModifyComputeSystem]).
// Rather than represent them as [interface{}] (or [any]), [json.RawMessage] is used instead.
// The primary use for [json.RawMessage] is to allow delayed unmarshalling, so fields do not
// have to be marshalled back to JSON and then re-unmarshalled to the the correct type.
// (Typically [json] will default to primitive types, such as [map] when unmarshalling [any].)
//
// Defaulting to [json.RawMessag] avoids needing to manually update types, and allows
// reusing the same type definitions for both marshalling (on the host) and unmarshalling
// (in the guest).
//
// [JSON API]: https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference
// [ModifySettingRequest]: https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference#ModifySettingRequest
// [HcsModifyComputeSystem]: https://learn.microsoft.com/en-us/virtualization/api/hcs/reference/hcsmodifycomputesystem#remarks
package hcsschema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ! if modifying an auto-generated file, write its name here and add a `// MODIFICATION: ` comment to the file

// Manually edited files:
// - cpu_group_affinity.go
// - create_group_operation.go
// - iov_settings.go
// - isolation_settings.go
// - process_parameters.go
// - properties.go
// - virtual_smb_share.go

// ! write the name of any additional files created in this package

// Manually created (lookup) files
// - cim_mount_flags.go
// - interrupt_moderation_mode_lookup.go
// - modification_request_extra.go
// - modify_setting_request_extra.go
// - os_type_lookup.go
// - plan_nine_share_flags_lookup.go
// - resolution_type_lookup.go
// - system_type_lookup.go
// - v_smb_share_flags_lookup.go

// ToRawMessage is a convenience function to encode a value into a [json.RawMessage].
func ToRawMessage(v any) (*json.RawMessage, error) {
	// TODO: create a [sync.Pool] of encoders and buffers

	// since we don't know the `omitempty` status of the field the message is going into,
	// return nil for nil-valued inputs, and when the parent struct is marshalled, it can
	// decide how to encode an empty message

	// note: an "empty" JSON message is non-empty: ie, "null", "{}", "[]", or "\"\""
	// if the JSON string is empty, (ie, ""), return nil instead of a pointer
	// to an empty (or nil) slice.

	if v == nil {
		return nil, nil
	}

	// don't re-encode a RawMessage
	switch v := v.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return nil, nil
		}
		return &v, nil
	case *json.RawMessage:
		return v, nil
	}

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	// trim trailing new line
	m := json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	if len(m) == 0 {
		return nil, nil
	}
	return &m, nil
}

// enumLookup is used to simplify functions that look up enum created as a set of const declarations.
// E.g., creating a functions to find the enum value corresponding to a string value requires only
// defining a `map[string]EnumType`.
func enumLookup[M ~map[K]V, K comparable, V any](m M, k K) (V, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}

	v := *new(V)
	return v, fmt.Errorf("failed enum lookup from %T to %T: unknown value: %+v", k, v, k)
}
