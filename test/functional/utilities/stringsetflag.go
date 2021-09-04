package testutilities

// StringSetFlag is a type to be used with the standard library's flag.Var
// function as a custom flag value. It accumulates the arguments passed each
// time the flag is used into a set.
type StringSetFlag struct {
	set map[string]struct{}
}

// NewStringSetFlag returns a new StringSetFlag with an empty set.
func NewStringSetFlag() StringSetFlag {
	return StringSetFlag{
		set: make(map[string]struct{}),
	}
}

func (ssf StringSetFlag) String() string {
	b := "["
	i := 0
	for k := range ssf.set {
		if i > 0 {
			b = b + ", "
		}
		b = b + k
		i++
	}
	b = b + "]"
	return b
}

// Set is called by `flag` each time the flag is seen when parsing the
// command line.
func (ssf StringSetFlag) Set(s string) error {
	ssf.set[s] = struct{}{}
	return nil
}

// ValueSet returns the internal set of what values have been seen.
func (ssf StringSetFlag) ValueSet() map[string]struct{} {
	return ssf.set
}
