package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

var ErrAnnotationExpansionConflict = errors.New("annotation expansion conflict")
var ErrGenericAnnotationConflict = errors.New("specified annotations conflict")

// ProcessAnnotations expands annotations into their corresponding annotation groups.
func ProcessAnnotations(ctx context.Context, specAnnotations map[string]string) error {
	// Named `Process` and not `Expand` since this function may be expanded (pun intended) to
	// deal with other annotation issues and validation.

	// Rather than give up part of the way through on error, this just emits a warning (similar
	// to the `parseAnnotation*` functions) and continues through, so the spec is not left in a
	// (partially) unusable form.

	// expand annotations
	var errs []error
	for key, exps := range annotations.AnnotationExpansionMap() {
		// check if annotation is present
		if val, ok := specAnnotations[key]; ok {
			// ideally, some normalization would occur here (ie, "True" -> "true")
			// but strings may be case-sensitive
			for _, k := range exps {
				if v, ok := specAnnotations[k]; ok && val != v {
					err := fmt.Errorf("%w: %q = %q and %q = %q", ErrAnnotationExpansionConflict, key, val, k, v)
					errs = append(errs, err)
					log.G(ctx).WithFields(logrus.Fields{
						logfields.OCIAnnotation:               key,
						logfields.Value:                       val,
						logfields.OCIAnnotation + "-conflict": k,
						logfields.Value + "-conflict":         v,
					}).WithError(err).Warning("annotation expansion would overwrite conflicting value")
					continue
				}
				specAnnotations[k] = val
			}
		}
	}

	// validate host process containers annotations are not conflicting
	disableHPC := ParseAnnotationsBool(ctx, specAnnotations, annotations.DisableHostProcessContainer, false)
	enableHPC := ParseAnnotationsBool(ctx, specAnnotations, annotations.HostProcessContainer, false)
	if disableHPC && enableHPC {
		err := fmt.Errorf("%w: host process container annotations %q = %q and %q = %q",
			ErrGenericAnnotationConflict,
			annotations.DisableHostProcessContainer, specAnnotations[annotations.DisableHostProcessContainer],
			annotations.HostProcessContainer, specAnnotations[annotations.HostProcessContainer])
		errs = append(errs, err)

		log.G(ctx).WithFields(logrus.Fields{
			logfields.OCIAnnotation:               annotations.DisableHostProcessContainer,
			logfields.Value:                       specAnnotations[annotations.DisableHostProcessContainer],
			logfields.OCIAnnotation + "-conflict": annotations.HostProcessContainer,
			logfields.Value + "-conflict":         specAnnotations[annotations.HostProcessContainer],
		}).WithError(err).Warning("Host process container and disable host process container cannot both be true")
	}

	return errors.Join(errs...)
}

// handle specific annotations

// ParseAnnotationsDisableGMSA searches for the boolean value which specifies
// if providing a gMSA credential should be disallowed. Returns the value found,
// if parsable, otherwise returns false otherwise.
func ParseAnnotationsDisableGMSA(ctx context.Context, s *specs.Spec) bool {
	return ParseAnnotationsBool(ctx, s.Annotations, annotations.WCOWDisableGMSA, false)
}

// parseAdditionalRegistryValues extracts the additional registry values to set from annotations.
//
// Like the [parseAnnotation*] functions, this logs errors but does not return them.
func parseAdditionalRegistryValues(ctx context.Context, a map[string]string) []hcsschema.RegistryValue {
	// rather than have users deal with nil vs []hcsschema.RegistryValue as returns, always
	// return the latter.
	// this is mostly to make testing easier, since its awkward to have to differentiate between
	// situations where one is returned vs the other.

	k := iannotations.AdditionalRegistryValues
	v := a[k]
	if v == "" {
		return []hcsschema.RegistryValue{}
	}

	t := []hcsschema.RegistryValue{}
	if err := json.Unmarshal([]byte(v), &t); err != nil {
		logAnnotationValueParseError(ctx, k, v, fmt.Sprintf("%T", t), err)
		return []hcsschema.RegistryValue{}
	}

	// basic error checking: warn about and delete invalid registry keys
	rvs := make([]hcsschema.RegistryValue, 0, len(t))
	for _, rv := range t {
		entry := log.G(ctx).WithFields(logrus.Fields{
			logfields.OCIAnnotation: k,
			logfields.Value:         v,
			"registry-value":        log.Format(ctx, rv),
		})

		if rv.Key == nil {
			entry.Warning("registry key is required")
			continue
		}

		if !slices.Contains([]hcsschema.RegistryHive{
			hcsschema.RegistryHive_SYSTEM,
			hcsschema.RegistryHive_SOFTWARE,
			hcsschema.RegistryHive_SECURITY,
			hcsschema.RegistryHive_SAM,
		}, rv.Key.Hive) {
			entry.Warning("invalid registry key hive")
			continue
		}

		if rv.Key.Name == "" {
			entry.Warning("registry key name is required")
			continue
		}

		if rv.Name == "" {
			entry.Warning("registry name is required")
			continue
		}

		if !slices.Contains([]hcsschema.RegistryValueType{
			hcsschema.RegistryValueType_NONE,
			hcsschema.RegistryValueType_STRING,
			hcsschema.RegistryValueType_EXPANDED_STRING,
			hcsschema.RegistryValueType_MULTI_STRING,
			hcsschema.RegistryValueType_BINARY,
			hcsschema.RegistryValueType_D_WORD,
			hcsschema.RegistryValueType_Q_WORD,
			hcsschema.RegistryValueType_CUSTOM_TYPE,
		}, rv.Type_) {
			entry.Warning("invalid registry value type")
			continue
		}

		// multiple values are set
		b2i := map[bool]int{true: 1} // hack to convert bool to int
		if (b2i[rv.StringValue != ""] + b2i[rv.BinaryValue != ""] + b2i[rv.DWordValue != 0] + b2i[rv.QWordValue != 0]) > 1 {
			entry.Warning("multiple values set")
			continue
		}

		// Validate hive/key pair is allowed.
		// We don't want to allow setting all registries, since that can arbitrarily modify uVM behavior.
		// Instead, limit it to services, policies, and software (for now) since matches the
		// typical use cases of enabling bug fixes and changing service and software settings.
		type allowReg struct {
			hive, path, name string
		}
		if !slices.ContainsFunc(
			[]allowReg{
				{
					hive: "System",
					path: `CurrentControlSet\Services`,
				},
				{
					hive: "System",
					path: `CurrentControlSet\Policies`,
				},
				{
					hive: "System",
					path: "Software",
				},
				{
					hive: "Software",
				},
			},
			func(allowed allowReg) bool {
				return (allowed.hive == "" || strings.EqualFold(string(rv.Key.Hive), allowed.hive)) &&
					strings.HasPrefix(strings.ToLower(rv.Key.Name), strings.ToLower(allowed.path)) &&
					(allowed.name == "" || strings.EqualFold(rv.Name, allowed.name))
			},
		) {
			entry.Warning("registry value is not permitted to be set")
			continue
		}

		entry.Trace("parsed additional registry value")
		rvs = append(rvs, rv)
	}

	return slices.Clip(rvs)
}

// ParseHVSocketServiceTable extracts any additional Hyper-V socket service configurations from annotations.
//
// Like the [parseAnnotation*] functions, this logs errors but does not return them.
func ParseHVSocketServiceTable(ctx context.Context, a map[string]string) map[string]hcsschema.HvSocketServiceConfig {
	sc := make(map[string]hcsschema.HvSocketServiceConfig)
	// TODO(go1.23) use range over functions to implement a functional `filter | map $ a`
	for k, v := range a {
		sGUID, found := strings.CutPrefix(k, iannotations.UVMHyperVSocketConfigPrefix)
		if !found {
			continue
		}

		entry := log.G(ctx).WithFields(logrus.Fields{
			logfields.OCIAnnotation: k,
			logfields.Value:         v,
			"guid":                  sGUID,
		})

		g, err := guid.FromString(sGUID)
		if err != nil {
			entry.WithError(err).Warn("invalid GUID string for Hyper-V socket service configuration annotation")
			continue
		}
		sGUID = g.String() // overwrite the GUID string to standardize format (capitalization)

		conf := hcsschema.HvSocketServiceConfig{}
		if err := json.Unmarshal([]byte(v), &conf); err != nil {
			logAnnotationValueParseError(ctx, k, v, fmt.Sprintf("%T", conf), err)
			continue
		}

		if _, found := sc[sGUID]; found {
			entry.WithFields(logrus.Fields{
				"guid": sGUID,
			}).Warn("overwritting existing Hyper-V socket service configuration")
		}

		if entry.Logger.IsLevelEnabled(logrus.TraceLevel) {
			entry.WithField("configuration", log.Format(ctx, conf)).Trace("found Hyper-V socket service configuration annotation")
		}
		sc[sGUID] = conf
	}

	return sc
}

// general annotation parsing

// ParseAnnotationsBool searches `a` for `key` and if found verifies that the
// value is `true` or `false` in any case. If `key` is not found returns `def`.
func ParseAnnotationsBool(ctx context.Context, a map[string]string, key string, def bool) bool {
	if v, ok := a[key]; ok {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
		logAnnotationValueParseError(ctx, key, v, logfields.Bool, err)
	}
	return def
}

// ParseAnnotationsNullableBool searches `a` for `key` and if found verifies that the
// value is `true` or `false`. If `key` is not found it returns a null pointer.
// The JSON Marshaller will omit null pointers and will serialize non-null pointers as
// the value they point at.
func ParseAnnotationsNullableBool(ctx context.Context, a map[string]string, key string) *bool {
	if v, ok := a[key]; ok {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return &b
		}
		logAnnotationValueParseError(ctx, key, v, logfields.Bool, err)
	}
	return nil
}

// ParseAnnotationsInt32 searches `a` for `key` and if found verifies that the
// value is a 32-bit signed integer. If `key` is not found returns `def`.
func ParseAnnotationsInt32(ctx context.Context, a map[string]string, key string, def int32) int32 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseInt(v, 10, 32)
		if err == nil {
			v := int32(countu)
			return v
		}
		logAnnotationValueParseError(ctx, key, v, logfields.Int32, err)
	}
	return def
}

// ParseAnnotationsUint32 searches `a` for `key` and if found verifies that the
// value is a 32 bit unsigned integer. If `key` is not found returns `def`.
func ParseAnnotationsUint32(ctx context.Context, a map[string]string, key string, def uint32) uint32 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 32)
		if err == nil {
			v := uint32(countu)
			return v
		}
		logAnnotationValueParseError(ctx, key, v, logfields.Uint32, err)
	}
	return def
}

// ParseAnnotationsUint64 searches `a` for `key` and if found verifies that the
// value is a 64 bit unsigned integer. If `key` is not found returns `def`.
func ParseAnnotationsUint64(ctx context.Context, a map[string]string, key string, def uint64) uint64 {
	if v, ok := a[key]; ok {
		countu, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return countu
		}
		logAnnotationValueParseError(ctx, key, v, logfields.Uint64, err)
	}
	return def
}

// ParseAnnotationCommaSeparated searches `a` for `annotation` corresponding to a
// list of comma separated strings.
func ParseAnnotationCommaSeparatedUint32(_ context.Context, a map[string]string, key string, def []uint32) []uint32 {
	cs, ok := a[key]
	if !ok || cs == "" {
		return def
	}
	sints := strings.Split(cs, ",")
	ints := make([]uint32, len(sints))
	for i := range sints {
		x, err := strconv.ParseUint(sints[i], 10, 32)
		ints[i] = uint32(x)
		if err != nil {
			return def
		}
	}
	return ints
}

func ParseAnnotationCommaSeparatedUint64(ctx context.Context, a map[string]string, key string, def []uint64) []uint64 {
	cs, ok := a[key]
	if !ok || cs == "" {
		return def
	}
	sints := strings.Split(cs, ",")
	ints := make([]uint64, len(sints))
	for i := range sints {
		x, err := strconv.ParseUint(sints[i], 10, 64)
		ints[i] = x
		if err != nil {
			logAnnotationValueParseError(ctx, key, cs, logfields.Uint64, err)
			return def
		}
	}
	return ints
}

// ParseAnnotationsString searches `a` for `key`. If `key` is not found returns `def`.
func ParseAnnotationsString(a map[string]string, key string, def string) string {
	if v, ok := a[key]; ok {
		return v
	}
	return def
}

// ParseAnnotationCommaSeparated searches `a` for `key` corresponding to a
// list of comma separated strings.
func ParseAnnotationCommaSeparated(key string, a map[string]string) []string {
	cs, ok := a[key]
	if !ok || cs == "" {
		return nil
	}
	results := strings.Split(cs, ",")
	return results
}

// ParseAnnotationsGUID searches `a` for `key`. If `key` is found, tries to parse it as guid.GUID, otherwise
// returns `def`.
func ParseAnnotationsGUID(a map[string]string, key string, def *guid.GUID) (*guid.GUID, error) {
	if v, ok := a[key]; ok {
		g, err := guid.FromString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse annotation %q with value %q as GUID: %w", key, v, err)
		}
		return &g, nil
	}
	return def, nil
}

func logAnnotationValueParseError(ctx context.Context, k, v, et string, err error) {
	entry := log.G(ctx).WithFields(logrus.Fields{
		logfields.OCIAnnotation: k,
		logfields.Value:         v,
		logfields.ExpectedType:  et,
	})
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Warning("annotation value could not be parsed")
}
