package networking

type Network struct {
	NetworkName string
	Settings    *NetworkSettings
}

type NetworkSettings struct {
	Name string
}

func NewNetwork(settings *NetworkSettings) (*Network, error) {
	return &Network{
		NetworkName: settings.Name,
		Settings:    settings,
	}, nil
}
