package flag

import (
	"fmt"
	"testing"

	"golang.org/x/exp/slices"
)

// tests for testing fixtures ...

// calling New(IncludeExclude)?StringSet will add it to the default flag set,
// which may cause problems since [testing] already defines flags

func Test_ExcludeStringSetFlag(t *testing.T) {
	all := []string{"one", "two", "three", "four", "zero"}
	es := &IncludeExcludeStringSet{
		inc: &StringSet{
			s:  make(map[string]struct{}),
			cs: false,
		},
		exc: &StringSet{
			s:  make(map[string]struct{}),
			cs: false,
		},
		def: slices.Clone(all),
	}

	orderlessEq(t, all, es.Strings())
	for _, s := range all {
		assert(t, es.IsSet(s), s+" is expected to be set, but is not")
	}

	for i, tc := range []struct {
		set   []string
		unset []string
		exp   []string
	}{
		{
			unset: []string{"one", "five"},
			exp:   []string{"two", "three", "four", "zero"},
		},
		{
			unset: []string{"one, three"},
			exp:   []string{"two", "four", "zero"},
		},
		{
			set: []string{"two,one"},
			exp: []string{"two"},
		},
		{
			set: []string{"three,one", " zero "},
			exp: []string{"two", "zero"},
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			for _, s := range tc.set {
				must(t, es.inc.Set(s))
			}
			for _, s := range tc.unset {
				must(t, es.exc.Set(s))
			}

			orderlessEq(t, tc.exp, es.Strings())

			for _, s := range all {
				if slices.Contains(tc.exp, s) {
					assert(t, es.IsSet(s), s+" is expected to be set, but is not")
				} else {
					assert(t, !es.IsSet(s), s+" is not expected to be set, but is")
				}
			}
		})
	}
}

func Test_StringSetFlag(t *testing.T) {
	ss := &StringSet{
		s:  make(map[string]struct{}),
		cs: false,
	}

	must(t, ss.Set("hi,bye,HI"))
	must(t, ss.Set("Bye"))
	must(t, ss.Set("not a word"))
	exp := []string{"hi", "bye", "not a word"}

	orderlessEq(t, exp, ss.Strings())
	for _, s := range exp {
		assert(t, ss.IsSet(s), s+"is expected to be set, but is not")
	}
	for _, s := range []string{"HI", "bYe", "BYE", "  not A wOrD"} {
		assert(t, ss.IsSet(s), s+"is expected to be set, but is not")
	}
	for _, s := range []string{"hello", "goodbye", "also not a word"} {
		assert(t, !ss.IsSet(s), s+"is not expected to be set, but is")
	}
}

func orderlessEq(tb testing.TB, exp, got []string) {
	tb.Helper()
	if len(exp) != len(got) {
		tb.Fatalf("expected length %d (%s), got %d (%s)", len(exp), exp, len(got), got)
	}
	ss := stringSet{}
	for _, s := range exp {
		ss.set(s)
	}
	for _, s := range got {
		assert(tb, ss.isSet(s), "unexpected value: "+s)
	}
}

func assert(tb testing.TB, b bool, msg any) {
	tb.Helper()
	if !b {
		tb.Fatal("assertion failed", msg)
	}
}

func must(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}
