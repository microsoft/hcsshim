package oci

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var ErrAnnotationExpansionConflict = errors.New("annotation expansion conflict")

// ProcessAnnotations expands annotations into their corresponding annotation groups
func ProcessAnnotations(ctx context.Context, s *specs.Spec) (err error) {
	// Named `Process` and not `Expand` since this function may be expanded (pun intended) to
	// deal with other annotation issues and validation.

	// Rather than give up part of the way through on error, this just emits a warning (similar
	// to the `parseAnnotation*` functions) and continues through, so the spec is not left in a
	// (partially) unusable form.
	// If multiple different errors are to be raised, they should be combined or, if they
	// are logged, only the last kept, depending on their severity.

	// expand annotations
	for key, exps := range annotations.AnnotationExpansions {
		// check if annotation is present
		if val, ok := s.Annotations[key]; ok {
			// ideally, some normalization would occur here (ie, "True" -> "true")
			// but strings may be case-sensitive
			for _, k := range exps {
				if v, ok := s.Annotations[k]; ok && val != v {
					err = ErrAnnotationExpansionConflict
					log.G(ctx).WithFields(logrus.Fields{
						logfields.OCIAnnotation:               key,
						logfields.Value:                       val,
						logfields.OCIAnnotation + "-conflict": k,
						logfields.Value + "-conflict":         v,
					}).WithError(err).Warning("annotation expansion would overwrite conflicting value")
					continue
				}
				s.Annotations[k] = val
			}
		}
	}

	return err
}

// handle specific annotations

// ParseAnnotationsDisableGMSA searches for the boolean value which specifies
// if providing a gMSA credential should be disallowed. Returns the value found,
// if parsable, otherwise returns false otherwise.
func ParseAnnotationsDisableGMSA(ctx context.Context, s *specs.Spec) bool {
	return ParseAnnotationsBool(ctx, s.Annotations, annotations.WCOWDisableGMSA, false)
}

// general annotation parsing

// ParseAnnotationsBool searches `a` for `key` and if found verifies that the
// value is `true` or `false` in any case. If `key` is not found returns `def`.
func ParseAnnotationsBool(ctx context.Context, a map[string]string, key string, def bool) bool {
	if v, ok := a[key]; ok {
		switch strings.ToLower(v) {
		case "true":
			return true
		case "false":
			return false
		default:
			logAnnotationParseError(ctx, key, v, logfields.Bool, nil)
		}
	}
	return def
}

// ParseAnnotationsNullableBool searches `a` for `key` and if found verifies that the
// value is `true` or `false`. If `key` is not found it returns a null pointer.
// The JSON Marshaller will omit null pointers and will serialise non-null pointers as
// the value they point at.
func ParseAnnotationsNullableBool(ctx context.Context, a map[string]string, key string) *bool {
	if v, ok := a[key]; ok {
		switch strings.ToLower(v) {
		case "true":
			_bool := true
			return &_bool
		case "false":
			_bool := false
			return &_bool
		default:
			err := errors.New("boolean fields must be 'true', 'false', or not set")
			logAnnotationParseError(ctx, key, v, logfields.Bool, err)
		}
	}
	return nil
}

// parseAnnotationsUint32 searches `a` for `key` and if found verifies that the
// value is a 32 bit unsigned integer. If `key` is not found returns `def`.
func parseAnnotationsUint32(ctx context.Context, a map[string]string, key string, def uint32) uint32 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 32)
		if err == nil {
			v := uint32(countu)
			return v
		}
		logAnnotationParseError(ctx, key, v, logfields.Uint32, err)
	}
	return def
}

// parseAnnotationsUint64 searches `a` for `key` and if found verifies that the
// value is a 64 bit unsigned integer. If `key` is not found returns `def`.
func parseAnnotationsUint64(ctx context.Context, a map[string]string, key string, def uint64) uint64 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return countu
		}
		logAnnotationParseError(ctx, key, v, logfields.Uint64, err)
	}
	return def
}

// parseAnnotationsString searches `a` for `key`. If `key` is not found returns `def`.
func parseAnnotationsString(a map[string]string, key string, def string) string {
	if v, ok := a[key]; ok {
		return v
	}
	return def
}

// ParseAnnotationCommaSeparated searches `annotations` for `annotation` corresponding to a
// list of comma separated strings
func ParseAnnotationCommaSeparated(annotation string, annotations map[string]string) []string {
	cs, ok := annotations[annotation]
	if !ok || cs == "" {
		return nil
	}
	results := strings.Split(cs, ",")
	return results
}

func logAnnotationParseError(ctx context.Context, k, v, et string, err error) {
	entry := log.G(ctx).WithFields(logrus.Fields{
		logfields.OCIAnnotation: k,
		logfields.Value:         v,
		logfields.ExpectedType:  et,
	})
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Warning("annotation could not be parsed")
}
