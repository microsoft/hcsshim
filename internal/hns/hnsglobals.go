package hns

type HNSGlobals struct {
	Version HNSVersion `json:"Version"`
}

type HNSVersion struct {
	Major int `json:"Major"`
	Minor int `json:"Minor"`
}

var (
	HNSVersion1803 = HNSVersion{Major: 7, Minor: 2}
	// default namespace ID used for all template and clone VMs. Please see the
	// description of the SetupNetworkNamespaceForClones function for more details.
	CLONING_DEFAULT_NETWORK_NAMESPACE_ID = "89EB8A86-E253-41FD-9800-E6D88EB2E18A"
)

func GetHNSGlobals() (*HNSGlobals, error) {
	var version HNSVersion
	err := hnsCall("GET", "/globals/version", "", &version)
	if err != nil {
		return nil, err
	}

	globals := &HNSGlobals{
		Version: version,
	}

	return globals, nil
}
