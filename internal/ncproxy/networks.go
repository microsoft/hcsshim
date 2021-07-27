package ncproxy

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO katiewasnothere: update this

// Package metadata stores all labels and object specific metadata by namespace.
// This package also contains the main garbage collection logic for cleaning up
// resources consistently and atomically. Resources used by backends will be
// tracked in the metadata store to be exposed to consumers of this package.
//
// The layout where a "/" delineates a bucket is described in the following
// section. Please try to follow this as closely as possible when adding
// functionality. We can bolster this with helpers and more structure if that
// becomes an issue.
//
// Generically, we try to do the following:
//
// 	<version>/<namespace>/<object>/<key> -> <field>
//
// version: Currently, this is "v1". Additions can be made to v1 in a backwards
// compatible way. If the layout changes, a new version must be made, along
// with a migration.
//
// namespace: the namespace to which this object belongs.
//
// object: defines which object set is stored in the bucket. There are two
// special objects, "labels" and "indexes". The "labels" bucket stores the
// labels for the parent namespace. The "indexes" object is reserved for
// indexing objects, if we require in the future.
//
// key: object-specific key identifying the storage bucket for the objects
// contents.
//
// Below is the current database schema. This should be updated each time
// the structure is changed in addition to adding a migration and incrementing
// the database version. Note that `╘══*...*` refers to maps with arbitrary
// keys.
//  ├──version : <varint>                        - Latest version, see migrations
//  └──v1                                                - Schema version bucket // TODO katiewasnothere: I don't think I need a namespace
//        ├──shims
//        |  ╘══ container id: <compute agent service>
//        ├──endpoints
//        │  ╘══*endpoint name*
//        |    ├── macaddress: <string>
//        |    ├── ipaddress: <string>
//        |    ├── ipaddress_prefixlength: <string>
//        |    ├── network_name: <string>
//        |    ├── default_gateway: <string>
//        |    ├── container_id: <string>
//        |    └── settings: <endpoint settings>
//        |        ├── portname_settings
//        |        |   └── port_name: <string>
//        │        ├── iov_settings
//        |        |   ├── iov_offload_weight: <uint32>
//        |        |   ├── queue_pairs_requested: <uint32>
//        |        |   └── interrupt_moderation: <uint32>
//        |        └── dns_settings
//        |            ├── server_ip_addrs: <slice of strings>
//        |            ├── domain: <string>
//        |            └── search: <slice of strings>
//        └──network
//           ╘══*network name*
//               ├── container_id: <string>
//               └── settings:
//                   ├── hcn_settings:
//                   |   ├── name: <string>
//                   |   ├── mode: <enum>
//                   |   ├── switch_name: <string>
//                   |   ├── ipam_type: <enum>
//                   |   ├── subnet_ipaddress_prefix: <slice of strings>
//                   |   └── default_gateway: <string>
//                   └── custom_network_settings:
//                       └── name: <string>
type NetworkStore struct {
	DB *bolt.DB
}

func NewNetworkStore(db *bolt.DB) *NetworkStore {
	return &NetworkStore{DB: db}
}

const (
	hcnNetworkType = iota
	customNetworkType
)

type internalNetwork struct {
	Type int32
	Data json.RawMessage
}

func (i *internalNetwork) unmarshalNetworkData() (Network, error) {
	switch i.Type {
	case hcnNetworkType:
		network := &hcnNetwork{}
		if err := json.Unmarshal(i.Data, network); err != nil {
			return nil, err
		}
		return network, nil
	case customNetworkType:
		network := &customNetwork{}
		if err := json.Unmarshal(i.Data, network); err != nil {
			return nil, err
		}
		return network, nil
	}
	return nil, errors.Errorf("invalid network type %v", i.Type)
}

func (n *NetworkStore) Get(ctx context.Context, key string) (result Network, err error) {
	if err := n.DB.View(func(tx *bolt.Tx) error {
		bkt := getNetworkBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "network bucket %v", bucketKeyNetwork)
		}
		data := bkt.Get([]byte(key))
		if data == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "network %v", key)
		}
		internalData := &internalNetwork{}
		if err := json.Unmarshal(data, internalData); err != nil {
			return errors.Wrapf(err, "data is %v", string(data))
		}
		result, err = internalData.unmarshalNetworkData()
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (n *NetworkStore) GetAll(ctx context.Context) (results []Network, err error) {
	if err := n.DB.View(func(tx *bolt.Tx) error {
		bkt := getEndpointBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errdefs.ErrNotFound, "Endpoint bucket %v", bucketKeyEndpoint)
		}
		err := bkt.ForEach(func(k, v []byte) error {
			data := bkt.Get([]byte(k))
			if data == nil {
				return errors.Wrapf(errdefs.ErrNotFound, "Network %v", k)
			}
			internalData := &internalNetwork{}
			if err := json.Unmarshal(data, internalData); err != nil {
				return errors.Wrapf(err, "data is %v", string(data))
			}
			network, err := internalData.unmarshalNetworkData()
			if err != nil {
				return err
			}
			results = append(results, network)
			return nil
		})
		return err
	}); err != nil {
		return nil, err
	}

	return results, nil
}

func (n *NetworkStore) Update(ctx context.Context, key string, network Network) error {
	if err := n.DB.Update(func(tx *bolt.Tx) error {
		bkt, err := createNetworkBucket(tx)
		if err != nil {
			return err
		}
		data, err := json.Marshal(network)
		if err != nil {
			return err
		}
		internalNetwork := &internalNetwork{
			Type: network.Type(),
			Data: data,
		}
		internalData, err := json.Marshal(internalNetwork)
		if err != nil {
			return err
		}
		return bkt.Put([]byte(key), internalData)
	}); err != nil {
		return err
	}
	return nil
}

type Network interface {
	Delete(context.Context) error
	ID() string
	Name() string
	Type() int32
	Settings() *ncproxygrpc.Network
}

var _ = (Network)(&hcnNetwork{})
var _ = (Network)(&customNetwork{})

type hcnNetwork struct {
	NetworkName string
	Id          string
	HcnSettings *ncproxygrpc.HostComputeNetworkSettings
}

func CreateHcnNetwork(settings *ncproxygrpc.HostComputeNetworkSettings) (*hcnNetwork, error) {
	if settings.Name == "" || settings.Mode.String() == "" || settings.IpamType.String() == "" || settings.SwitchName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", settings)
	}

	// Check if the network already exists, and if so return error.
	_, err := hcn.GetNetworkByName(settings.Name)
	if err == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "network with name %q already exists", settings.Name)
	}

	// Get the layer ID from the external switch. HNS will create a transparent network for
	// any external switch that is created not through HNS so this is what we're
	// searching for here. If the network exists, the vSwitch with this name exists.
	extSwitch, err := hcn.GetNetworkByName(settings.SwitchName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network/switch with name `%s` found", settings.SwitchName)
		}
		return nil, errors.Wrapf(err, "failed to get network/switch with name %q", settings.SwitchName)
	}

	// Get layer ID and use this as the basis for what to layer the new network over.
	if extSwitch.Health.Extra.LayeredOn == "" {
		return nil, status.Errorf(codes.NotFound, "no layer ID found for network %q found", extSwitch.Id)
	}

	layerPolicy := hcn.LayerConstraintNetworkPolicySetting{LayerId: extSwitch.Health.Extra.LayeredOn}
	data, err := json.Marshal(layerPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal layer policy")
	}

	netPolicy := hcn.NetworkPolicy{
		Type:     hcn.LayerConstraint,
		Settings: data,
	}

	subnets := make([]hcn.Subnet, len(settings.SubnetIpaddressPrefix))
	for i, addrPrefix := range settings.SubnetIpaddressPrefix {
		subnet := hcn.Subnet{
			IpAddressPrefix: addrPrefix,
			Routes: []hcn.Route{
				{
					NextHop:           settings.DefaultGateway,
					DestinationPrefix: "0.0.0.0/0",
				},
			},
		}
		subnets[i] = subnet
	}

	ipam := hcn.Ipam{
		Type:    settings.IpamType.String(),
		Subnets: subnets,
	}

	network := &hcn.HostComputeNetwork{
		Name:     settings.Name,
		Type:     hcn.NetworkType(settings.Mode.String()),
		Ipams:    []hcn.Ipam{ipam},
		Policies: []hcn.NetworkPolicy{netPolicy},
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 2,
		},
	}

	network, err = network.Create()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create HNS network %q", settings.Name)
	}

	return &hcnNetwork{
		NetworkName: settings.Name,
		Id:          network.Id,
		HcnSettings: settings,
	}, nil
}

func (h *hcnNetwork) Delete(ctx context.Context) error {
	network, err := hcn.GetNetworkByName(h.NetworkName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return status.Errorf(codes.NotFound, "no network with name `%s` found", h.NetworkName)
		}
		return errors.Wrapf(err, "failed to get network with name %q", h.NetworkName)
	}

	if err = network.Delete(); err != nil {
		return errors.Wrapf(err, "failed to delete network with name %q", h.NetworkName)
	}
	return nil
}

func (h *hcnNetwork) Name() string {
	return h.NetworkName
}

func (h *hcnNetwork) ID() string {
	return h.Id
}

func (h *hcnNetwork) Type() int32 {
	return hcnNetworkType
}

func (h *hcnNetwork) Settings() *ncproxygrpc.Network {
	return &ncproxygrpc.Network{
		Settings: &ncproxygrpc.Network_HcnSettings{
			HcnSettings: h.HcnSettings,
		},
	}
}

type customNetwork struct {
	NetworkName    string
	CustomSettings *ncproxygrpc.CustomNetworkSettings
}

func CreateCustomNetwork(settings *ncproxygrpc.CustomNetworkSettings) (*customNetwork, error) {
	return &customNetwork{
		NetworkName:    settings.Name,
		CustomSettings: settings,
	}, nil
}

func (c *customNetwork) Delete(ctx context.Context) error {
	return nil
}

func (c *customNetwork) Name() string {
	return c.NetworkName
}

func (c *customNetwork) ID() string {
	return c.NetworkName
}

func (c *customNetwork) Type() int32 {
	return customNetworkType
}

func (c *customNetwork) Settings() *ncproxygrpc.Network {
	return &ncproxygrpc.Network{
		Settings: &ncproxygrpc.Network_CustomNetworkSettings{
			CustomNetworkSettings: c.CustomSettings,
		},
	}
}
