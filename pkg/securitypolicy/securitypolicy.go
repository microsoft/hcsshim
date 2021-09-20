package securitypolicy

import (
	"encoding/base64"
	"encoding/json"

	"github.com/pkg/errors"
)

type EnvVarRule string

const (
	EnvVarRuleString EnvVarRule = "string"
	EnvVarRuleRegex  EnvVarRule = "re2"
)

// Internal version of SecurityPolicyContainer
type securityPolicyContainer struct {
	// The command that we will allow the container to execute
	Command []string `json:"command"`
	// The rules for determining if a given environment variable is allowed
	EnvRules []securityPolicyEnvironmentVariableRule `json:"env_rules"`
	// An ordered list of dm-verity root hashes for each layer that makes up
	// "a container". Containers are constructed as an overlay file system. The
	// order that the layers are overlayed is important and needs to be enforced
	// as part of policy.
	Layers []string `json:"layers"`
}

// Internal versino of SecurityPolicyEnvironmentVariableRule
type securityPolicyEnvironmentVariableRule struct {
	Strategy EnvVarRule `json:"type"`
	Rule     string     `json:"rule"`
}

// SecurityPolicyState is a structure that holds user supplied policy to enforce
// we keep both the encoded representation and the unmarshalled representation
// because different components need to have access to either of these
type SecurityPolicyState struct {
	EncodedSecurityPolicy EncodedSecurityPolicy `json:"EncodedSecurityPolicy,omitempty"`
	SecurityPolicy        `json:"SecurityPolicy,omitempty"`
}

// EncodedSecurityPolicy is a JSON representation of SecurityPolicy that has
// been base64 encoded for storage in an annotation embedded within another
// JSON configuration
type EncodedSecurityPolicy struct {
	SecurityPolicy string `json:"SecurityPolicy,omitempty"`
}

// JSON transport version
type SecurityPolicy struct {
	// Flag that when set to true allows for all checks to pass. Currently used
	// to run with security policy enforcement "running dark"; checks can be in
	// place but the default policy that is created on startup has AllowAll set
	// to true, thus making policy enforcement effectively "off" from a logical
	// standpoint. Policy enforcement isn't actually off as the policy is "allow
	// everything:.
	AllowAll bool `json:"allow_all"`
	// Total number of containers in our map
	NumContainers int `json:"num_containers"`
	// One or more containers that are allowed to run
	Containers map[string]SecurityPolicyContainer `json:"containers"`
}

// SecurityPolicyContainer contains information about a container that should be
// allowed to run. "Allowed to run" is a bit of misnomer. For example, we
// enforce that when an overlay file system is constructed that it must be a
// an ordering of layers (as seen through dm-verity root hashes of devices)
// that match a listing from Layers in one of any valid SecurityPolicyContainer
// entries. Once that overlay creation is allowed, the command could not match
// policy and running the command would be rejected.
type SecurityPolicyContainer struct {
	// Number of entries that should be in the "Command" map
	NumCommands int `json:"num_commands"`
	// The command that we will allow the container to execute
	Command map[string]string `json:"command"`
	// Number of entries that should be in the "EnvRules" map
	NumEnvRules int `json:"num_env_rules"`
	// The rules for determining if a given environment variable is allowed
	EnvRules map[string]SecurityPolicyEnvironmentVariableRule `json:"env_rules"`
	// Number of entries that should in the "Layers" map
	NumLayers int `json:"num_layers"`
	// An "ordered list" of dm-verity root hashes for each layer that makes up
	// "a container". Containers are constructed as an overlay file system. The
	// order that the layers are overlayed is important and needs to be enforced
	// as part of policy. The map is interpreted as an ordered list by arranging
	// the keys of the map as indexes like 0,1,2,3 to establish the order.
	Layers map[string]string `json:"layers"`
}

type SecurityPolicyEnvironmentVariableRule struct {
	Strategy EnvVarRule `json:"strategy"`
	Rule     string     `json:"rule"`
}

// Constructs SecurityPolicyState from base64Policy string. It first decodes
// base64 policy and returns the structs security policy struct and encoded
// security policy for given policy. The security policy is transmitted as json
// in an annotation, so we first have to remove the base64 encoding that allows
// the JSON based policy to be passed as a string. From there, we decode the
// JSONand setup our security policy struct
func NewSecurityPolicyState(base64Policy string) (*SecurityPolicyState, error) {
	// construct an encoded security policy that holds the base64 representation
	encodedSecurityPolicy := EncodedSecurityPolicy{
		SecurityPolicy: base64Policy,
	}

	// base64 decode the incoming policy string
	// its base64 encoded because it is coming from an annotation
	// annotations are a map of string to string
	// we want to store a complex json object so.... base64 it is
	jsonPolicy, err := base64.StdEncoding.DecodeString(base64Policy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode policy from Base64 format")
	}

	// json unmarshall the decoded to a SecurityPolicy
	securityPolicy := SecurityPolicy{}
	err = json.Unmarshal(jsonPolicy, &securityPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal JSON policy")
	}

	return &SecurityPolicyState{
		SecurityPolicy:        securityPolicy,
		EncodedSecurityPolicy: encodedSecurityPolicy,
	}, nil
}
