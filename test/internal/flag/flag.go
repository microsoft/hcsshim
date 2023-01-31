// This package augments the default "flags" package with functionality similar
// to that in "github.com/urfave/cli", since the two packages do not mix easily
// and the "testing" package uses a default flagset that we cannot easily update.
package flag

import (
	"flag"
	"strings"

	"github.com/sirupsen/logrus"
)

const FeatureFlagName = "feature"

func NewFeatureFlag(all []string) *StringSet {
	return NewStringSet(FeatureFlagName,
		"set of `features` to test; can be set multiple times, with a comma-separated list, or both "+
			"(supported features: "+strings.Join(all, ", ")+")",
		false,
	)
}

// StringSet is a type to be used with the standard library's flag.Var
// function as a custom flag value, similar to "github.com/urfave/cli".StringSet,
// but it only tracks unique instances.
// It takes either a comma-separated list of strings, or repeated invocations.
type StringSet struct {
	s map[string]struct{}
	// cs indicates if the set is case sensitive or not
	cs bool
}

var _ flag.Value = &StringSet{}

// NewStringSet returns a new StringSetFlag with an empty set.
func NewStringSet(name, usage string, caseSensitive bool) *StringSet {
	ss := &StringSet{
		s:  make(map[string]struct{}),
		cs: caseSensitive,
	}
	flag.Var(ss, name, usage)
	return ss
}

// Strings returns a string slice of the flags provided to the flag
func (ss *StringSet) Strings() []string {
	a := make([]string, 0, len(ss.s))
	for k := range ss.s {
		a = append(a, k)
	}

	return a
}

func (ss *StringSet) String() string {
	return "[" + strings.Join(ss.Strings(), ", ") + "]"
}

func (ss *StringSet) Len() int { return len(ss.s) }

func (ss *StringSet) IsSet(s string) bool {
	_, ok := ss.s[ss.standardize(s)]
	return ok
}

// Set is called by `flag` each time the flag is seen when parsing the
// command line.
func (ss *StringSet) Set(s string) error {
	for _, f := range strings.Split(s, ",") {
		ss.s[ss.standardize(f)] = struct{}{}
	}
	return nil
}

// Standardize formats the feature flag s to be consistent (ie, trim and to lowercase)
func (ss *StringSet) standardize(s string) string {
	s = strings.TrimSpace(s)
	if !ss.cs {
		s = strings.ToLower(s)
	}
	return s
}

// LogrusLevel is a flag that accepts logrus logging levels, as strings.
type LogrusLevel struct {
	Level logrus.Level
}

var _ flag.Value = &LogrusLevel{}

func NewLogrusLevel(name, value, usage string) *LogrusLevel {
	l := &LogrusLevel{}
	if lvl, err := logrus.ParseLevel(value); err == nil {
		l.Level = lvl
	} else {
		l.Level = logrus.StandardLogger().Level
	}
	flag.Var(l, name, usage)
	return l
}

func (l *LogrusLevel) String() string {
	// may be called ona nil receiver
	// return default level
	if l == nil {
		return logrus.StandardLogger().Level.String()
	}

	return l.Level.String()
}

func (l *LogrusLevel) Set(s string) error {
	lvl, err := logrus.ParseLevel(s)
	if err != nil {
		return err
	}
	l.Level = lvl
	return nil
}
