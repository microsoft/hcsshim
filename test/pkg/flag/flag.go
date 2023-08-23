package flag

import (
	"flag"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	FeatureFlagName        = "feature"
	ExcludeFeatureFlagName = "exclude"
)

// NewFeatureFlag defines two flags, [FeatureFlagName] and [ExcludeFeatureFlagName], to
// allow setting and excluding certain features.
func NewFeatureFlag(features []string) *IncludeExcludeStringSet {
	fs := NewStringSet(FeatureFlagName,
		"`features` to test; can be set multiple times, with a comma-separated list, or both. "+
			"Leave empty to enable all features. "+
			"(supported features: "+strings.Join(features, ", ")+")", false)

	return NewIncludeExcludeStringSet(fs, ExcludeFeatureFlagName,
		"`features` to exclude from tests (see "+FeatureFlagName+" for more details)",
		features)
}

// IncludeExcludeStringSet allows unsetting strings seen in a [StringSet].
type IncludeExcludeStringSet struct {
	// flags explicitly included
	inc *StringSet
	// flags explicitly excluded
	exc *StringSet
	// def value, if no values set
	// we don't error if an unknown value is provided
	def []string
}

// NewIncludeExcludeStringSet returns a new NewIncludeExcludeStringSet.
func NewIncludeExcludeStringSet(include *StringSet, name, usage string, all []string) *IncludeExcludeStringSet {
	es := &IncludeExcludeStringSet{
		inc: include,
		exc: &StringSet{
			s:  make(map[string]struct{}),
			cs: include.cs,
		},
		def: slices.Clone(all),
	}
	flag.Var(es, name, usage)
	return es
}

var _ flag.Value = &IncludeExcludeStringSet{}

func (es *IncludeExcludeStringSet) Set(s string) error { return es.exc.Set(s) }

func (es *IncludeExcludeStringSet) String() string {
	if es == nil { // may be called by flag package on nil receiver
		return ""
	}
	ss := es.strings()
	if len(ss) == 0 {
		return ""
	}
	return "[" + strings.Join(ss, ", ") + "]"
}

func (es *IncludeExcludeStringSet) Strings() []string { return es.strings() }
func (es *IncludeExcludeStringSet) Len() int          { return len(es.strings()) }

func (es *IncludeExcludeStringSet) strings() []string {
	ss := es.def
	set := make([]string, 0, len(ss))
	if es.inc != nil && es.inc.Len() > 0 {
		// include values were explicitly set
		ss = es.inc.Strings()
	}
	for _, s := range ss {
		if !es.exc.IsSet(s) {
			set = append(set, s)
		}
	}
	return set
}

func (es *IncludeExcludeStringSet) IsSet(s string) bool {
	if es.inc == nil || es.inc.Len() == 0 || es.inc.IsSet(s) {
		// either no values were included, or value was explicitly provided
		return !es.exc.IsSet(s)
	}
	return false
}

// StringSet is a type to be used with the standard library's flag.Var
// function as a custom flag value, similar to "github.com/urfave/cli".StringSet,
// but it only tracks unique instances.
//
// It takes either a comma-separated list of strings, or repeated invocations.
type StringSet struct {
	s stringSet
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
func (ss *StringSet) Strings() []string { return maps.Keys(ss.s) }

func (ss *StringSet) String() string {
	if ss == nil || ss.Len() == 0 { // may be called by flag package on nil receiver
		return ""
	}
	return "[" + strings.Join(ss.Strings(), ", ") + "]"
}

func (ss *StringSet) Len() int { return len(ss.s) }

func (ss *StringSet) IsSet(s string) bool { return ss.s.isSet(ss.standardize(s)) }

// Set is called by `flag` each time the flag is seen when parsing the command line.
func (ss *StringSet) Set(s string) error {
	for _, f := range strings.Split(s, ",") {
		ss.s.set(ss.standardize(f))
	}
	return nil
}

// standardize formats the feature flag s to be consistent (ie, trim and to lowercase)
func (ss *StringSet) standardize(s string) string {
	s = strings.TrimSpace(s)
	if !ss.cs {
		s = strings.ToLower(s)
	}
	return s
}

// stringSet is a set of strings.
type stringSet map[string]struct{}

func (ss stringSet) set(s string) { ss[s] = struct{}{} }
func (ss stringSet) isSet(s string) bool {
	_, ok := ss[s]
	return ok
}

// LogrusLevel is a flag that accepts logrus logging levels as strings.
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
