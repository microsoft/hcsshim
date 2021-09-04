package hcs

import (
	"strconv"
	"strings"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/pkg/errors"
)

func (uvmb *utilityVMBuilder) SetSerialConsole(port uint32, listenerPath string) error {
	if !strings.HasPrefix(listenerPath, `\\.\pipe\`) {
		return errors.New("listener for serial console is not a named pipe")
	}

	uvmb.doc.VirtualMachine.Devices.ComPorts = map[string]hcsschema.ComPort{
		strconv.Itoa(int(port)): { // "0" would be COM1
			NamedPipe: listenerPath,
		},
	}
	return nil
}
