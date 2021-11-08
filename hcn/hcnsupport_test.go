//go:build integration
// +build integration

package hcn

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSupportedFeatures(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	jsonString, err := json.Marshal(supportedFeatures)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Supported Features:\n%s \n", jsonString)
}

func TestV2ApiSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := V2ApiSupported()
	if supportedFeatures.Api.V2 && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.Api.V2 && err == nil {
		t.Fatal(err)
	}
}

func TestRemoteSubnetSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := RemoteSubnetSupported()
	if supportedFeatures.RemoteSubnet && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.RemoteSubnet && err == nil {
		t.Fatal(err)
	}
}

func TestHostRouteSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := HostRouteSupported()
	if supportedFeatures.HostRoute && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.HostRoute && err == nil {
		t.Fatal(err)
	}
}

func TestDSRSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := DSRSupported()
	if supportedFeatures.DSR && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.DSR && err == nil {
		t.Fatal(err)
	}
}

func TestSlash32EndpointPrefixesSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := Slash32EndpointPrefixesSupported()
	if supportedFeatures.Slash32EndpointPrefixes && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.Slash32EndpointPrefixes && err == nil {
		t.Fatal(err)
	}
}

func TestAclSupportForProtocol252Support(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := AclSupportForProtocol252Supported()
	if supportedFeatures.AclSupportForProtocol252 && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.AclSupportForProtocol252 && err == nil {
		t.Fatal(err)
	}
}

func TestSessionAffinitySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := SessionAffinitySupported()
	if supportedFeatures.SessionAffinity && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.SessionAffinity && err == nil {
		t.Fatal(err)
	}
}

func TestIPv6DualStackSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := IPv6DualStackSupported()
	if supportedFeatures.IPv6DualStack && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.IPv6DualStack && err == nil {
		t.Fatal(err)
	}
}

func TestSetPolicySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := SetPolicySupported()
	if supportedFeatures.SetPolicy && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.SetPolicy && err == nil {
		t.Fatal(err)
	}
}

func TestNestedIpSetSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := NestedIpSetSupported()
	if supportedFeatures.NestedIpSet && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.NestedIpSet && err == nil {
		t.Fatal(err)
	}
}

func TestNetworkACLPolicySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := NetworkACLPolicySupported()
	if supportedFeatures.NetworkACL && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.NetworkACL && err == nil {
		t.Fatal(err)
	}
}

func TestVxlanPortSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := VxlanPortSupported()
	if supportedFeatures.VxlanPort && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.VxlanPort && err == nil {
		t.Fatal(err)
	}
}

func TestL4ProxyPolicySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := L4proxyPolicySupported()
	if supportedFeatures.L4Proxy && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.L4Proxy && err == nil {
		t.Fatal(err)
	}
}

func TestL4WfpProxyPolicySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := L4WfpProxyPolicySupported()
	if supportedFeatures.L4WfpProxy && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.L4WfpProxy && err == nil {
		t.Fatal(err)
	}
}

func TestTierAclPolicySupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	err := TierAclPolicySupported()
	if supportedFeatures.TierAcl && err != nil {
		t.Fatal(err)
	}
	if !supportedFeatures.TierAcl && err == nil {
		t.Fatal(err)
	}
}

func TestIsFeatureSupported(t *testing.T) {
	// HNSVersion1803 testing (single range tests)
	if isFeatureSupported(Version{Major: 0, Minor: 0}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 should NOT be supported on HNS version 0.0")
	}

	if isFeatureSupported(Version{Major: 7, Minor: 0}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 should NOT be supported on HNS version 7.1")
	}

	if isFeatureSupported(Version{Major: 7, Minor: 1}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 should NOT be supported on HNS version 7.1")
	}

	if !isFeatureSupported(Version{Major: 7, Minor: 2}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 7.2")
	}

	if !isFeatureSupported(Version{Major: 7, Minor: 3}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 7.3")
	}

	if !isFeatureSupported(Version{Major: 8, Minor: 0}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 8.0")
	}

	if !isFeatureSupported(Version{Major: 8, Minor: 2}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 8.2")
	}

	if !isFeatureSupported(Version{Major: 8, Minor: 3}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 8.3")
	}

	if !isFeatureSupported(Version{Major: 255, Minor: 2}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 255.2")
	}

	if !isFeatureSupported(Version{Major: 255, Minor: 0}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 255.0")
	}

	if !isFeatureSupported(Version{Major: 255, Minor: 6}, HNSVersion1803) {
		t.Fatalf("HNSVersion1803 SHOULD be supported on HNS version 255.6")
	}

	// Slash 32 endpoint prefix support testing (multi-ranges tests)
	if isFeatureSupported(Version{Major: 8, Minor: 0}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 8.0")
	}

	if isFeatureSupported(Version{Major: 8, Minor: 2}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 8.2")
	}

	if isFeatureSupported(Version{Major: 8, Minor: 3}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 8.3")
	}

	if isFeatureSupported(Version{Major: 8, Minor: 4}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 8.4")
	}

	if isFeatureSupported(Version{Major: 8, Minor: 5}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 8.5")
	}

	if isFeatureSupported(Version{Major: 9, Minor: 0}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 9.0")
	}

	if isFeatureSupported(Version{Major: 9, Minor: 2}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 9.2")
	}

	//// Beginning of supported range
	if !isFeatureSupported(Version{Major: 9, Minor: 3}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 9.3")
	}

	if !isFeatureSupported(Version{Major: 9, Minor: 4}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 9.4")
	}

	if !isFeatureSupported(Version{Major: 9, Minor: 5}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 9.5")
	}

	if !isFeatureSupported(Version{Major: 9, Minor: 9}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 9.9")
	}

	if !isFeatureSupported(Version{Major: 9, Minor: 372}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 9.372")
	}
	//// End of supported range

	if isFeatureSupported(Version{Major: 10, Minor: 0}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 10.0")
	}

	if isFeatureSupported(Version{Major: 10, Minor: 1}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 10.1")
	}

	if isFeatureSupported(Version{Major: 10, Minor: 2}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 10.2")
	}

	if isFeatureSupported(Version{Major: 10, Minor: 3}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion should NOT be supported on HNS version 10.3")
	}

	//// Beginning of supported range (final range, no end)
	if !isFeatureSupported(Version{Major: 10, Minor: 4}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 10.4")
	}

	if !isFeatureSupported(Version{Major: 10, Minor: 5}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 10.5")
	}

	if !isFeatureSupported(Version{Major: 10, Minor: 9}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 10.9")
	}

	if !isFeatureSupported(Version{Major: 10, Minor: 410}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 10.410")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 0}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.0")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 1}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.1")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 2}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.2")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 3}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.3")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 4}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.4")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 5}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.5")
	}

	if !isFeatureSupported(Version{Major: 11, Minor: 9}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 11.9")
	}

	if !isFeatureSupported(Version{Major: 255, Minor: 2}, Slash32EndpointPrefixesVersion) {
		t.Fatalf("Slash32EndpointPrefixesVersion SHOULD be supported on HNS version 255.2")
	}
}
