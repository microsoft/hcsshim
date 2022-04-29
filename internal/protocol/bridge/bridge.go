// This package defines the GCS protocol over a bridge between a host and guest.
package bridge

import "encoding/json"

const NullContainerID = "00000000-0000-0000-0000-000000000000"

// UVMContainerID is the ContainerID for messages targeted at the UVM itself in
// the V2 protocol.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

type Any struct {
	Value interface{}
}

func (a *Any) MarshalText() ([]byte, error) {
	return json.Marshal(a.Value)
}

func (a *Any) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, &a.Value)
}
