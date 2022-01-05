package networking

type Endpoint struct {
	EndpointName string
	NamespaceID  string
	Settings     *EndpointSettings
}

type EndpointSettings struct {
	Name                  string
	Macaddress            string
	IPAddress             string
	IPAddressPrefixLength uint32
	NetworkName           string
	DefaultGateway        string
	DeviceDetails         *DeviceDetails
}

type DeviceDetails struct {
	PCIDeviceDetails *PCIDeviceDetails
}

type PCIDeviceDetails struct {
	DeviceID             string
	VirtualFunctionIndex uint32
}

func NewEndpoint(settings *EndpointSettings) (*Endpoint, error) {
	return &Endpoint{
		EndpointName: settings.Name,
		Settings:     settings,
	}, nil
}
