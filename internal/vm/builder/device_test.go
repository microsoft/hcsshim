//go:build windows

package builder

import (
	"strconv"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
)

// administratorsPipePrefix is the protected pipe prefix for administrators.
// It is also covered by pipePrefix since it starts with `\\.\pipe\`.
const administratorsPipePrefix = `\\.\pipe\ProtectedPrefix\Administrators\`

func TestVPCIDevice(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var devices DeviceOptions = b
	device := hcsschema.VirtualPciFunction{DeviceInstancePath: "PCI\\VEN_1234", VirtualFunction: 2}

	vmbusGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("guid.NewV4 error = %v", err)
	}

	if err := devices.AddVPCIDevice(vmbusGUID, device, true); err != nil {
		t.Fatalf("AddVPCIDevice error = %v", err)
	}
	if len(cs.VirtualMachine.Devices.VirtualPci) != 1 {
		t.Fatalf("VirtualPci entries = %d, want 1", len(cs.VirtualMachine.Devices.VirtualPci))
	}
	entry, ok := cs.VirtualMachine.Devices.VirtualPci[vmbusGUID.String()]
	if !ok {
		t.Fatal("VirtualPci entry not found for provided vmbusGUID")
	}
	if len(entry.Functions) != 1 {
		t.Fatalf("VirtualPci Functions = %d, want 1", len(entry.Functions))
	}
	if entry.Functions[0].DeviceInstancePath != device.DeviceInstancePath || entry.Functions[0].VirtualFunction != device.VirtualFunction {
		t.Fatal("VPCI function not applied as expected")
	}
	if entry.PropagateNumaAffinity == nil || !*entry.PropagateNumaAffinity {
		t.Fatal("PropagateNumaAffinity should be true")
	}

	dupGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("guid.NewV4 error = %v", err)
	}

	if err := devices.AddVPCIDevice(dupGUID, device, false); !errors.Is(err, errAlreadySet) {
		t.Fatalf("AddVPCIDevice duplicate error = %v, want %v", err, errAlreadySet)
	}
}

func TestSerialConsoleAndGraphics(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var devices DeviceOptions = b
	if err := devices.SetSerialConsole(1, "not-a-pipe"); err == nil {
		t.Fatal("SetSerialConsole should reject non-pipe path")
	}

	pipePath := `\\.\pipe\serial`
	if err := devices.SetSerialConsole(1, pipePath); err != nil {
		t.Fatalf("SetSerialConsole error = %v", err)
	}
	key := strconv.Itoa(1)
	if cs.VirtualMachine.Devices.ComPorts[key].NamedPipe != pipePath {
		t.Fatal("serial console named pipe not set as expected")
	}

	adminPipePath := administratorsPipePrefix + "serial"
	if err := devices.SetSerialConsole(1, adminPipePath); err != nil {
		t.Fatalf("SetSerialConsole should accept administrators pipe prefix, error = %v", err)
	}
	if cs.VirtualMachine.Devices.ComPorts[key].NamedPipe != adminPipePath {
		t.Fatal("serial console administrators named pipe not set as expected")
	}

	devices.EnableGraphicsConsole()
	if cs.VirtualMachine.Devices.Keyboard == nil || cs.VirtualMachine.Devices.EnhancedModeVideo == nil || cs.VirtualMachine.Devices.VideoMonitor == nil {
		t.Fatal("graphics console devices not enabled")
	}
}
