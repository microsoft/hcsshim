package hns

import (
	"github.com/Microsoft/hcsshim/hns/internal"
)

// Type of Request Support in ModifySystem
type PolicyType = internal.PolicyType

// RequestType const
const (
	Nat                  = internal.Nat
	ACL                  = internal.ACL
	PA                   = internal.PA
	VLAN                 = internal.VLAN
	VSID                 = internal.VSID
	VNet                 = internal.VNet
	L2Driver             = internal.L2Driver
	Isolation            = internal.Isolation
	QOS                  = internal.QOS
	OutboundNat          = internal.OutboundNat
	ExternalLoadBalancer = internal.ExternalLoadBalancer
	Route                = internal.Route
	Proxy                = internal.Proxy
)

type ProxyPolicy = internal.ProxyPolicy

type NatPolicy = internal.NatPolicy

type QosPolicy = internal.QosPolicy

type IsolationPolicy = internal.IsolationPolicy

type VlanPolicy = internal.VlanPolicy

type VsidPolicy = internal.VsidPolicy

type PaPolicy = internal.PaPolicy

type OutboundNatPolicy = internal.OutboundNatPolicy

type ActionType = internal.ActionType
type DirectionType = internal.DirectionType
type RuleType = internal.RuleType

const (
	Allow = internal.Allow
	Block = internal.Block

	In  = internal.In
	Out = internal.Out

	Host   = internal.Host
	Switch = internal.Switch
)

type ACLPolicy = internal.ACLPolicy

type Policy = internal.Policy
