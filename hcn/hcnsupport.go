package hcn

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	// featuresOnce handles assigning the supported features and printing the supported info to stdout only once to avoid unnecessary work
	// multiple times.
	featuresOnce      sync.Once
	supportedFeatures SupportedFeatures
)

// SupportedFeatures are the features provided by the Service.
type SupportedFeatures struct {
	Acl                      AclFeatures `json:"ACL"`
	Api                      ApiSupport  `json:"API"`
	RemoteSubnet             bool        `json:"RemoteSubnet"`
	HostRoute                bool        `json:"HostRoute"`
	DSR                      bool        `json:"DSR"`
	Slash32EndpointPrefixes  bool        `json:"Slash32EndpointPrefixes"`
	AclSupportForProtocol252 bool        `json:"AclSupportForProtocol252"`
	SessionAffinity          bool        `json:"SessionAffinity"`
	IPv6DualStack            bool        `json:"IPv6DualStack"`
	SetPolicy                bool        `json:"SetPolicy"`
	VxlanPort                bool        `json:"VxlanPort"`
	L4Proxy                  bool        `json:"L4Proxy"`    // network policy that applies VFP rules to all endpoints on the network to redirect traffic
	L4WfpProxy               bool        `json:"L4WfpProxy"` // endpoint policy that applies WFP filters to redirect traffic to/from that endpoint
	TierAcl                  bool        `json:"TierAcl"`
	NetworkACL               bool        `json:"NetworkACL"`
	NestedIpSet              bool        `json:"NestedIpSet"`
}

// AclFeatures are the supported ACL possibilities.
type AclFeatures struct {
	AclAddressLists       bool `json:"AclAddressLists"`
	AclNoHostRulePriority bool `json:"AclHostRulePriority"`
	AclPortRanges         bool `json:"AclPortRanges"`
	AclRuleId             bool `json:"AclRuleId"`
}

// ApiSupport lists the supported API versions.
type ApiSupport struct {
	V1 bool `json:"V1"`
	V2 bool `json:"V2"`
}

// GetSupportedFeatures returns the features supported by the Service.
func GetSupportedFeatures() SupportedFeatures {
	// Only fetch the supported features and print the HCN version and features once, instead of everytime this is invoked.
	// The logs are useful to debug incidents where there's confusion on if a feature is supported on the host machine. The sync.Once
	// helps to avoid redundant spam of these anytime a check needs to be made for if an HCN feature is supported. This is a common
	// occurrence in kubeproxy for example.
	featuresOnce.Do(func() {
		globals, err := GetGlobals()
		if err != nil {
			// Expected on pre-1803 builds, all features will be false/unsupported
			logrus.Debugf("Unable to obtain globals: %s", err)
		} else {
			supportedFeatures.Acl = AclFeatures{
				AclAddressLists:       isFeatureSupported(globals.Version, HNSVersion1803),
				AclNoHostRulePriority: isFeatureSupported(globals.Version, HNSVersion1803),
				AclPortRanges:         isFeatureSupported(globals.Version, HNSVersion1803),
				AclRuleId:             isFeatureSupported(globals.Version, HNSVersion1803),
			}

			supportedFeatures.Api = ApiSupport{
				V2: isFeatureSupported(globals.Version, V2ApiSupport),
				V1: true, // HNSCall is still available.
			}

			supportedFeatures.RemoteSubnet = isFeatureSupported(globals.Version, RemoteSubnetVersion)
			supportedFeatures.HostRoute = isFeatureSupported(globals.Version, HostRouteVersion)
			supportedFeatures.DSR = isFeatureSupported(globals.Version, DSRVersion)
			supportedFeatures.Slash32EndpointPrefixes = isFeatureSupported(globals.Version, Slash32EndpointPrefixesVersion)
			supportedFeatures.AclSupportForProtocol252 = isFeatureSupported(globals.Version, AclSupportForProtocol252Version)
			supportedFeatures.SessionAffinity = isFeatureSupported(globals.Version, SessionAffinityVersion)
			supportedFeatures.IPv6DualStack = isFeatureSupported(globals.Version, IPv6DualStackVersion)
			supportedFeatures.SetPolicy = isFeatureSupported(globals.Version, SetPolicyVersion)
			supportedFeatures.VxlanPort = isFeatureSupported(globals.Version, VxlanPortVersion)
			supportedFeatures.L4Proxy = isFeatureSupported(globals.Version, L4ProxyPolicyVersion)
			supportedFeatures.L4WfpProxy = isFeatureSupported(globals.Version, L4WfpProxyPolicyVersion)
			supportedFeatures.TierAcl = isFeatureSupported(globals.Version, TierAclPolicyVersion)
			supportedFeatures.NetworkACL = isFeatureSupported(globals.Version, NetworkACLPolicyVersion)
			supportedFeatures.NestedIpSet = isFeatureSupported(globals.Version, NestedIpSetVersion)

			logrus.WithFields(logrus.Fields{
				"version":           fmt.Sprintf("%+v", globals.Version),
				"supportedFeatures": fmt.Sprintf("%+v", supportedFeatures),
			}).Info("HCN feature check")
		}
	})

	return supportedFeatures
}

func isFeatureSupported(currentVersion Version, versionsSupported VersionRanges) bool {
	isFeatureSupported := false

	for _, versionRange := range versionsSupported {
		isFeatureSupported = isFeatureSupported || isFeatureInRange(currentVersion, versionRange)
	}

	return isFeatureSupported
}

func isFeatureInRange(currentVersion Version, versionRange VersionRange) bool {
	if currentVersion.Major < versionRange.MinVersion.Major {
		return false
	}
	if currentVersion.Major > versionRange.MaxVersion.Major {
		return false
	}
	if currentVersion.Major == versionRange.MinVersion.Major && currentVersion.Minor < versionRange.MinVersion.Minor {
		return false
	}
	if currentVersion.Major == versionRange.MaxVersion.Major && currentVersion.Minor > versionRange.MaxVersion.Minor {
		return false
	}
	return true
}
