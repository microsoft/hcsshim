package remotevm

import (
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func (uvmb *utilityVMBuilder) SetSerialConsole(port uint32, listenerPath string) error {
	config := &vmservice.SerialConfig_Config{
		Port:       port,
		SocketPath: listenerPath,
	}
	uvmb.config.SerialConfig.Ports = []*vmservice.SerialConfig_Config{config}
	return nil
}
