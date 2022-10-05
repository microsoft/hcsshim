// This package augments the default "flags" package with functionality similar
// to that in "github.com/urfave/cli", since the two packages do not mix easily
// and the "testing" package uses a default flagset that we cannot easily update.
package flag

import (
	"flag"
	"strings"
)

const FeatureFlagName = "feature"

func NewFeatureFlag(all []string) *StringSlice {
	return NewStringSlice(FeatureFlagName,
		"the sets of functionality to test; can be set multiple times, or separated with commas. "+
			"Supported features: "+strings.Join(all, ", "),
	)
}

// StringSlice is a type to be used with the standard library's flag.Var
// function as a custom flag value, similar to "github.com/urfave/cli".StringSlice.
// It takes either a comma-separated list of strings, or repeated invocations.
type StringSlice struct {
	S StringSet
}

var _ flag.Value = &StringSlice{}

// NewStringSetFlag returns a new StringSetFlag with an empty set.
func NewStringSlice(name, usage string) *StringSlice {
	ss := &StringSlice{
		S: make(StringSet),
	}
	flag.Var(ss, name, usage)
	return ss
}

// Strings returns a string slice of the flags provided to the flag
func (ss *StringSlice) Strings() []string {
	return ss.S.Strings()
}

func (ss *StringSlice) String() string {
	return ss.S.String()
}

// Set is called by `flag` each time the flag is seen when parsing the
// command line.
func (ss *StringSlice) Set(s string) error {
	for _, f := range strings.Split(s, ",") {
		f = Standardize(f)
		ss.S[f] = struct{}{}
	}

	return nil
}

type StringSet map[string]struct{}

func (ss StringSet) Strings() []string {
	a := make([]string, 0, len(ss))
	for k := range ss {
		a = append(a, k)
	}

	return a
}

func (ss StringSet) String() string {
	return "[" + strings.Join(ss.Strings(), ", ") + "]"
}

// Standardize formats the feature flag s to be consistent (ie, trim and to lowercase)
func Standardize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
