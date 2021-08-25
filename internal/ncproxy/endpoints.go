package ncproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type EndpointStore struct {
	DB *bolt.DB
}

func NewEndpointStore(db *bolt.DB) *EndpointStore {
	return &EndpointStore{DB: db}
}

const (
	hcnEndpointType = iota
	customEndpointType
)

type internalEndpoint struct {
	EType int32
	Data  json.RawMessage
}

func (i *internalEndpoint) unmarshalEndpointData() (Endpoint, error) {
	switch i.EType {
	case hcnEndpointType:
		endpt := &hcnEndpoint{}
		if err := json.Unmarshal(i.Data, endpt); err != nil {
			return nil, err
		}
		return endpt, nil
	case customEndpointType:
		endpt := &customEndpoint{}
		if err := json.Unmarshal(i.Data, endpt); err != nil {
			return nil, err
		}
		return endpt, nil
	}
	return nil, errors.Errorf("invalid endpoint type %v", i.EType)
}

func (n *EndpointStore) Get(ctx context.Context, key string) (result Endpoint, err error) {
	if err := n.DB.View(func(tx *bolt.Tx) error {
		bkt := getEndpointBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "Endpoint bucket %v", bucketKeyEndpoint)
		}
		jsonData := bkt.Get([]byte(key))
		if jsonData == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "Endpoint %v", key)
		}
		endpt := &internalEndpoint{}
		if err := json.Unmarshal(jsonData, endpt); err != nil {
			return err
		}
		result, err = endpt.unmarshalEndpointData()
		return err
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (n *EndpointStore) GetAll(ctx context.Context) (results []Endpoint, err error) {
	if err := n.DB.View(func(tx *bolt.Tx) error {
		bkt := getEndpointBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "Endpoint bucket %v", bucketKeyEndpoint)
		}
		err := bkt.ForEach(func(k, v []byte) error {
			jsonData := bkt.Get([]byte(k))
			if jsonData == nil {
				return errors.Wrapf(errdefs.ErrNotFound, "Endpoint %v", k)
			}
			endptInternal := &internalEndpoint{}
			if err := json.Unmarshal(jsonData, endptInternal); err != nil {
				return err
			}
			endpt, err := endptInternal.unmarshalEndpointData()
			if err != nil {
				return err
			}
			results = append(results, endpt)
			return nil
		})
		return err
	}); err != nil {
		return nil, err
	}

	return results, nil
}

// TODO katiewasnothere: separate settings into a separate bucket
func (n *EndpointStore) Update(ctx context.Context, key string, endpt Endpoint) error {
	if err := n.DB.Update(func(tx *bolt.Tx) error {
		bkt, err := createEndpointBucket(tx)
		if err != nil {
			return err
		}
		jsonEndptData, err := json.Marshal(endpt)
		if err != nil {
			return err
		}
		data := &internalEndpoint{
			EType: endpt.Type(),
			Data:  jsonEndptData,
		}
		jsonData, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return bkt.Put([]byte(key), jsonData)
	}); err != nil {
		return err
	}
	return nil
}

type Endpoint interface {
	Type() int32
	Add(context.Context, string) error
	Delete(context.Context) error
	ID() string
	Name() string
	Settings() *ncproxygrpc.EndpointSettings
	CreateAddNICRequest(string, string, bool) (*NetworkModifySettings, error)
}

var _ = (Endpoint)(&hcnEndpoint{})
var _ = (Endpoint)(&customEndpoint{})

type hcnEndpoint struct {
	EndpointName string
	Id           string
	NamespaceID  string
	HcnEndpoint  *hcn.HostComputeEndpoint
	HcnSettings  *ncproxygrpc.HcnEndpointSettings
}

func getNetworkModifyRequest(adapterID string, requestType string, settings interface{}) interface{} {
	if osversion.Build() >= osversion.RS5 {
		return guestrequest.NetworkModifyRequest{
			AdapterId:   adapterID,
			RequestType: requestType,
			Settings:    settings,
		}
	}
	return guestrequest.RS4NetworkModifyRequest{
		AdapterInstanceId: adapterID,
		RequestType:       requestType,
		Settings:          settings,
	}
}

type NetworkModifySettings struct {
	PreAddRequet *hcsschema.ModifySettingRequest
	Request      *hcsschema.ModifySettingRequest
}

func (h *hcnEndpoint) CreateAddNICRequest(platform, adapterID string, networkNamespaceSupported bool) (*NetworkModifySettings, error) {
	result := &NetworkModifySettings{}
	hnsEndpoint, err := hns.GetHNSEndpointByName(h.EndpointName)
	if err != nil {
		return nil, err
	}

	// First a pre-add. This is a guest-only request and is only done on Windows.
	if platform == "windows" {
		preAddRequest := &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeNetwork,
				RequestType:  requesttype.Add,
				Settings: getNetworkModifyRequest(
					adapterID,
					requesttype.PreAdd,
					hnsEndpoint,
				),
			},
		}
		result.PreAddRequet = preAddRequest
	}

	// Then the Add itself
	request := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Add,
		ResourcePath: fmt.Sprintf(resourcepaths.NetworkResourceFormat, adapterID),
		Settings: &hcsschema.NetworkAdapter{
			EndpointId: h.Id,
			MacAddress: h.HcnSettings.Macaddress,
		},
	}

	if platform == "windows" {
		request.GuestRequest = &guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeNetwork,
			RequestType:  requesttype.Add,
			Settings: getNetworkModifyRequest(
				adapterID,
				requesttype.Add,
				nil),
		}
	} else {
		// Verify this version of LCOW supports Network HotAdd
		//TODO katiewasnothere: is this even needed anymore?
		if networkNamespaceSupported {
			request.GuestRequest = &guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeNetwork,
				RequestType:  requesttype.Add,
				Settings: &guestrequest.LCOWNetworkAdapter{
					NamespaceID:     h.NamespaceID,
					ID:              adapterID,
					MacAddress:      hnsEndpoint.MacAddress,
					IPAddress:       hnsEndpoint.IPAddress.String(),
					PrefixLength:    hnsEndpoint.PrefixLength,
					GatewayAddress:  hnsEndpoint.GatewayAddress,
					DNSSuffix:       hnsEndpoint.DNSSuffix,
					DNSServerList:   hnsEndpoint.DNSServerList,
					EnableLowMetric: hnsEndpoint.EnableLowMetric,
					EncapOverhead:   hnsEndpoint.EncapOverhead,
				},
			}
		}
	}
	result.Request = request
	return result, nil
}

func (h *hcnEndpoint) Type() int32 {
	return hcnEndpointType
}

func (h *hcnEndpoint) Name() string {
	return h.EndpointName
}

func CreateHcnEndpoint(settings *ncproxygrpc.HcnEndpointSettings) (*hcnEndpoint, error) {
	// get hcn network
	hcnNetwork, err := hcn.GetNetworkByName(settings.NetworkName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", settings.NetworkName)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", settings.NetworkName)
	}

	prefixLen, err := strconv.ParseUint(settings.IpaddressPrefixlength, 10, 8)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert ip address prefix length to uint")
	}

	// Construct ip config.
	ipConfig := hcn.IpConfig{
		IpAddress:    settings.Ipaddress,
		PrefixLength: uint8(prefixLen),
	}

	policies, err := constructEndpointPolicies(settings)
	if err != nil {
		return nil, errors.Wrap(err, "failed to construct endpoint policies")
	}

	endpoint := &hcn.HostComputeEndpoint{
		Name:               settings.Name,
		HostComputeNetwork: hcnNetwork.Id,
		MacAddress:         settings.Macaddress,
		IpConfigurations:   []hcn.IpConfig{ipConfig},
		Policies:           policies,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	if settings.DnsSetting != nil {
		endpoint.Dns = hcn.Dns{
			ServerList: settings.DnsSetting.ServerIpAddrs,
			Domain:     settings.DnsSetting.Domain,
			Search:     settings.DnsSetting.Search,
		}
	}

	endpoint, err = endpoint.Create()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HNS endpoint")
	}

	return &hcnEndpoint{
		EndpointName: settings.Name,
		Id:           endpoint.Id,
		HcnSettings:  settings,
	}, nil
}

func constructEndpointPolicies(settings *ncproxygrpc.HcnEndpointSettings) ([]hcn.EndpointPolicy, error) {
	policies := []hcn.EndpointPolicy{}
	if settings.IovPolicySettings != nil {
		iovSettings := hcn.IovPolicySetting{
			IovOffloadWeight:    settings.IovPolicySettings.IovOffloadWeight,
			QueuePairsRequested: settings.IovPolicySettings.QueuePairsRequested,
			InterruptModeration: settings.IovPolicySettings.InterruptModeration,
		}
		iovJSON, err := json.Marshal(iovSettings)
		if err != nil {
			return []hcn.EndpointPolicy{}, errors.Wrap(err, "failed to marshal IovPolicySettings")
		}
		policy := hcn.EndpointPolicy{
			Type:     hcn.IOV,
			Settings: iovJSON,
		}
		policies = append(policies, policy)
	}

	if settings.PortnamePolicySetting != nil {
		portPolicy := hcn.PortnameEndpointPolicySetting{
			Name: settings.PortnamePolicySetting.PortName,
		}
		portPolicyJSON, err := json.Marshal(portPolicy)
		if err != nil {
			return []hcn.EndpointPolicy{}, errors.Wrap(err, "failed to marshal portname")
		}
		policy := hcn.EndpointPolicy{
			Type:     hcn.PortName,
			Settings: portPolicyJSON,
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// TODO katiewasnothere: namespace seems very hns specific,,,,
func (h *hcnEndpoint) Add(ctx context.Context, namespaceID string) error {
	ep, err := hcn.GetEndpointByName(h.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return status.Errorf(codes.NotFound, "no endpoint with name `%s` found", h.EndpointName)
		}
		return errors.Wrapf(err, "failed to get endpoint with name %q", h.EndpointName)
	}

	if err := hcn.AddNamespaceEndpoint(namespaceID, ep.Id); err != nil {
		return errors.Wrapf(err, "failed to add endpoint with name %q to namespace", h.EndpointName)
	}
	return nil
}

func (h *hcnEndpoint) Delete(ctx context.Context) error {
	ep, err := hcn.GetEndpointByName(h.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return status.Errorf(codes.NotFound, "no endpoint with name `%s` found", h.EndpointName)
		}
		return errors.Wrapf(err, "failed to get endpoint with name %q", h.EndpointName)
	}

	if err = ep.Delete(); err != nil {
		return errors.Wrapf(err, "failed to delete endpoint with name %q", h.EndpointName)
	}
	return nil
}

func (h *hcnEndpoint) ID() string {
	return h.Id
}

func (h *hcnEndpoint) Settings() *ncproxygrpc.EndpointSettings {
	return &ncproxygrpc.EndpointSettings{
		Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
			HcnEndpoint: h.HcnSettings,
		},
	}
}

func (h *hcnEndpoint) Modify() error {

}

type customEndpoint struct {
	EndpointName   string
	NamespaceID    string
	CustomSettings *ncproxygrpc.CustomEndpointSettings
}

func CreateCustomEndpoint(settings *ncproxygrpc.CustomEndpointSettings) (*customEndpoint, error) {
	return &customEndpoint{
		EndpointName:   settings.Name,
		CustomSettings: settings,
	}, nil
}

func (c *customEndpoint) Add(ctx context.Context, namespaceID string) error {
	c.NamespaceID = namespaceID
	return nil
}

func (c *customEndpoint) Delete(ctx context.Context) error {
	return nil
}

func (c *customEndpoint) ID() string {
	return c.EndpointName
}

func (c *customEndpoint) Name() string {
	return c.EndpointName
}

func (c *customEndpoint) Type() int32 {
	return customEndpointType
}

func (c *customEndpoint) Settings() *ncproxygrpc.EndpointSettings {
	return &ncproxygrpc.EndpointSettings{
		Settings: &ncproxygrpc.EndpointSettings_CustomEndpoint{
			CustomEndpoint: c.CustomSettings,
		},
	}
}

func (c *customEndpoint) CreateAddNICRequest(platform, adapterID string, networkNamespaceSupported bool) (*NetworkModifySettings, error) {
	result := &NetworkModifySettings{}
	if platform != "windows" {
		if !networkNamespaceSupported {
			return nil, fmt.Errorf("guest does not support network namespaces %+v", cfg)
		}
		cfg := &guestrequest.LCOWNetworkAdapter{
			NamespaceID:    c.NamespaceID,
			ID:             adapterID,
			IPAddress:      c.CustomSettings.Ipaddress,
			PrefixLength:   uint8(c.CustomSettings.IpaddressPrefixlength),
			GatewayAddress: c.CustomSettings.DefaultGateway,
			AssignedPCI:    true,
		}
		request := &hcsschema.ModifySettingRequest{}
		request.GuestRequest = &guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeNetwork,
			RequestType:  requesttype.Add,
			Settings:     cfg,
		}
		result.Request = request
	}
	return result, nil
}
