package securitypolicy

// SecurityPolicy is the user supplied security policy to enforce.
type SecurityPolicy struct {
	// Flag that when set to true allows for all checks to pass. Currently used
	// to run with security policy enforcement "running dark"; checks can be in
	// place but the default policy that is created on startup has AllowAll set
	// to true, thus making policy enforcement effectively "off" from a logical
	// standpoint. Policy enforcement isn't actually off as the policy is "allow
	// everything:.
	AllowAll bool `json:"allow_all"`
	// One or more containers that are allowed to run
	Containers []SecurityPolicyContainer `json:"containers"`
}

// SecurityPolicyContainer contains information about a container that should be
// allowed to run. "Allowed to run" is a bit of misnomer. For example, we
// enforce that when an overlay file system is constructed that it must be a
// an ordering of layers (as seen through dm-verity root hashes of devices)
// that match a listing from Layers in one of any valid SecurityPolicyContainer
// entries. Once that overlay creation is allowed, the command could not match
// policy and running the command would be rejected.
type SecurityPolicyContainer struct {
	// The command that we will allow the container to execute
	Command []string `json:"command"`
	// An ordered list of dm-verity root hashes for each layer that makes up
	// "a container". Containers are constructed as an overlay file system. The
	// order that the layers are overlayed is important and needs to be enforced
	// as part of policy.
	Layers []string `json:"layers"`
}

// EncodedSecurityPolicy is a JSON representation of SecurityPolicy that has
// been base64 encoded for storage in an annotation embedded within another
// JSON configuration
type EncodedSecurityPolicy struct {
	SecurityPolicy string `json:"SecurityPolicy,omitempty"`
}
