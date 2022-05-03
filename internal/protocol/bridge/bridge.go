// This package defines the GCS protocol over a bridge between a host and guest.
package bridge

import (
	"encoding/json"

	"github.com/Microsoft/go-winio/pkg/guid"
)

const NullContainerID = "00000000-0000-0000-0000-000000000000"

// UVMContainerID is the ContainerID for messages targeted at the UVM itself in
// the V2 protocol.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// LinuxGcsVsockPort is the vsock port number that the Linux GCS will
// connect to.
const LinuxGcsVsockPort = 0x40000000

// WindowsGcsHvsockServiceID is the hvsock service ID that the Windows GCS
// will connect to.
var WindowsGcsHvsockServiceID = guid.GUID{
	Data1: 0xacef5661,
	Data2: 0x84a1,
	Data3: 0x4e44,
	Data4: [8]uint8{0x85, 0x6b, 0x62, 0x45, 0xe6, 0x9f, 0x46, 0x20},
}

// WindowsGcsHvHostID is the hvsock address for the parent of the VM running the GCS
var WindowsGcsHvHostID = guid.GUID{
	Data1: 0x894cc2d6,
	Data2: 0x9d79,
	Data3: 0x424f,
	Data4: [8]uint8{0x93, 0xfe, 0x42, 0x96, 0x9a, 0xe6, 0xd8, 0xd1},
}

type Any struct {
	Value interface{}
}

func (a *Any) MarshalText() ([]byte, error) {
	return json.Marshal(a.Value)
}

func (a *Any) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, &a.Value)
}
