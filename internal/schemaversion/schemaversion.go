// +build windows

package schemaversion

import (
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/osversion"
	"github.com/sirupsen/logrus"
)

type SchemaVersion struct {
	Major int32 `json:"Major"`
	Minor int32 `json:"Minor"`
}

// SchemaV10 makes it easy for callers to get a v1.0 schema version object
func SchemaV10() *SchemaVersion {
	return &SchemaVersion{Major: 1, Minor: 0}
}

// SchemaV20 makes it easy for callers to get a v2.0 schema version object
func SchemaV20() *SchemaVersion {
	return &SchemaVersion{Major: 2, Minor: 0}
}

// isSupported determines if a given schema version is supported
func (sv *SchemaVersion) IsSupported() error {
	if sv.IsV10() {
		return nil
	}
	if sv.IsV20() {
		if osversion.Get().Build < osversion.RS5 {
			return fmt.Errorf("unsupported on this Windows build")
		}
		return nil
	}
	return fmt.Errorf("unknown schema version %s", sv.String())
}

// IsV10 determines if a given schema version object is 1.0. This was the only thing
// supported in RS1..3. It lives on in RS5, but will be deprecated in a future release.
func (sv *SchemaVersion) IsV10() bool {
	if sv.Major == 1 && sv.Minor == 0 {
		return true
	}
	return false
}

// IsV20 determines if a given schema version object is 2.0. This was introduced in
// RS4, but not fully implemented. Recommended for applications using HCS in RS5
// onwards.
func (sv *SchemaVersion) IsV20() bool {
	if sv.Major == 2 && sv.Minor == 0 {
		return true
	}
	return false
}

// String returns a JSON encoding of a schema version object
func (sv *SchemaVersion) String() string {
	b, err := json.Marshal(sv)
	if err != nil {
		return ""
	}
	return string(b[:])
}

// DetermineSchemaVersion works out what schema version to use based on build and
// requested option.
func DetermineSchemaVersion(requestedSV *SchemaVersion) *SchemaVersion {
	sv := SchemaV10()
	if osversion.Get().Build >= osversion.RS5 {
		sv = SchemaV10() // TODO: When do we flip this to V2 for RS5? Answer - when functionally complete. Templating. CredSpecs. Networking. LCOW...
	}
	if requestedSV != nil {
		if err := requestedSV.IsSupported(); err == nil {
			sv = requestedSV
		} else {
			logrus.Warnf("Ignoring unsupported requested schema version %+v", requestedSV)
		}
	}
	return sv
}
