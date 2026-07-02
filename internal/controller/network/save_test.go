//go:build windows && (lcow || wcow)

package network

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Microsoft/hcsshim/hcn"
	netsave "github.com/Microsoft/hcsshim/internal/controller/network/save"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
)

// mustEnvelope marshals a payload and wraps it in an envelope with the
// well-known type URL, matching what Save emits.
func mustEnvelope(t *testing.T, p *netsave.Payload) *anypb.Any {
	t.Helper()
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &anypb.Any{TypeUrl: netsave.TypeURL, Value: b}
}

// configuredController returns a fully-configured controller carrying the
// supplied endpoint bindings, ready to be saved.
func configuredController(eps map[string]*hcn.HostComputeEndpoint) *Controller {
	return &Controller{
		namespaceID:                 "ns-1",
		policyBasedRouting:          true,
		isNamespaceSupportedByGuest: true,
		vmEndpoints:                 eps,
		netState:                    StateConfigured,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Save
// ─────────────────────────────────────────────────────────────────────────────

// TestSave_RejectsUnstableState verifies that a snapshot is only produced from
// a fully-configured network; every other state yields an error and no payload.
func TestSave_RejectsUnstableState(t *testing.T) {
	for _, st := range []State{StateNotConfigured, StateInvalid, StateTornDown, StateDestinationMigrating, StateSourceMigrating} {
		t.Run(st.String(), func(t *testing.T) {
			c := configuredController(map[string]*hcn.HostComputeEndpoint{})
			c.netState = st

			env, err := c.Save(t.Context())
			if err == nil {
				t.Fatalf("expected error saving from state %s, got nil", st)
			}
			if env != nil {
				t.Errorf("expected nil envelope on failure, got %+v", env)
			}
			if !strings.Contains(err.Error(), st.String()) {
				t.Errorf("expected error to mention state %s, got: %v", st, err)
			}
		})
	}
}

// TestSave_NilEndpointBinding verifies that a configured network holding a nil
// endpoint cannot be saved, since the destination could not re-create the NIC.
func TestSave_NilEndpointBinding(t *testing.T) {
	c := configuredController(map[string]*hcn.HostComputeEndpoint{"nic-1": nil})

	env, err := c.Save(t.Context())
	if err == nil {
		t.Fatal("expected error saving nil endpoint, got nil")
	}
	if env != nil {
		t.Errorf("expected nil envelope on failure, got %+v", env)
	}
}

// TestSave_Success verifies the produced envelope is self-describing and that
// its decoded payload reproduces the controller's scalar config and every
// endpoint binding.
func TestSave_Success(t *testing.T) {
	c := configuredController(map[string]*hcn.HostComputeEndpoint{
		"nic-1": {Id: "ep-1", MacAddress: "aa:bb:cc:dd:ee:01", Name: "eth0"},
		"nic-2": {Id: "ep-2", MacAddress: "aa:bb:cc:dd:ee:02", Name: "eth1"},
	})

	env, err := c.Save(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.GetTypeUrl() != netsave.TypeURL {
		t.Errorf("expected type URL %q, got %q", netsave.TypeURL, env.GetTypeUrl())
	}

	got := &netsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), got); err != nil {
		t.Fatalf("unmarshal saved payload: %v", err)
	}
	if got.GetSchemaVersion() != netsave.SchemaVersion {
		t.Errorf("expected schema version %d, got %d", netsave.SchemaVersion, got.GetSchemaVersion())
	}
	if got.GetNamespaceID() != "ns-1" || !got.GetPolicyBasedRouting() || !got.GetIsNamespaceSupportedByGuest() {
		t.Errorf("scalar config not preserved: %+v", got)
	}
	if len(got.GetVmEndpoints()) != 2 {
		t.Fatalf("expected 2 endpoint bindings, got %d", len(got.GetVmEndpoints()))
	}
	b1 := got.GetVmEndpoints()["nic-1"]
	if b1.GetEndpointID() != "ep-1" || b1.GetMacAddress() != "aa:bb:cc:dd:ee:01" || b1.GetEndpointName() != "eth0" {
		t.Errorf("nic-1 binding not preserved: %+v", b1)
	}

	// A successful save freezes the source until it is resumed or torn down.
	if c.netState != StateSourceMigrating {
		t.Errorf("expected state SourceMigrating after Save, got %s", c.netState)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Import
// ─────────────────────────────────────────────────────────────────────────────

// TestImport_Rejects verifies that a destination refuses any envelope it cannot
// safely interpret: a missing envelope, a foreign type, undecodable bytes, an
// unknown schema version, or a binding missing its NIC key.
func TestImport_Rejects(t *testing.T) {
	cases := []struct {
		name string
		env  *anypb.Any
	}{
		{"NilEnvelope", nil},
		{"WrongTypeURL", &anypb.Any{TypeUrl: "type.microsoft.com/bogus", Value: nil}},
		{"CorruptPayload", &anypb.Any{TypeUrl: netsave.TypeURL, Value: []byte{0x08}}},
		{"SchemaMismatch", mustEnvelope(t, &netsave.Payload{SchemaVersion: netsave.SchemaVersion + 1})},
		{"EmptyNICKey", mustEnvelope(t, &netsave.Payload{
			SchemaVersion: netsave.SchemaVersion,
			VmEndpoints:   map[string]*netsave.EndpointBinding{"": {EndpointID: "ep-1"}},
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Import(t.Context(), tc.env)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if c != nil {
				t.Errorf("expected nil controller on failure, got %+v", c)
			}
		})
	}
}

// TestImport_Success verifies a valid envelope rehydrates into a migrating
// controller (not yet operational) with all scalar config and bindings restored.
func TestImport_Success(t *testing.T) {
	env := mustEnvelope(t, &netsave.Payload{
		SchemaVersion:               netsave.SchemaVersion,
		NamespaceID:                 "ns-1",
		PolicyBasedRouting:          true,
		IsNamespaceSupportedByGuest: true,
		VmEndpoints: map[string]*netsave.EndpointBinding{
			"nic-1": {EndpointID: "ep-1", MacAddress: "aa:bb:cc:dd:ee:01", EndpointName: "eth0"},
		},
	})

	c, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.netState != StateDestinationMigrating {
		t.Errorf("expected state DestinationMigrating, got %s", c.netState)
	}
	if c.namespaceID != "ns-1" || !c.policyBasedRouting || !c.isNamespaceSupportedByGuest {
		t.Errorf("scalar config not restored: %+v", c)
	}
	ep, ok := c.vmEndpoints["nic-1"]
	if !ok || ep.Id != "ep-1" || ep.MacAddress != "aa:bb:cc:dd:ee:01" || ep.Name != "eth0" {
		t.Errorf("nic-1 binding not restored: %+v (present=%v)", ep, ok)
	}
}

// TestSaveImport_RoundTrip verifies that the destination reconstructs exactly
// what the source saved, leaving the rehydrated controller non-operational
// until it is resumed.
func TestSaveImport_RoundTrip(t *testing.T) {
	src := configuredController(map[string]*hcn.HostComputeEndpoint{
		"nic-1": {Id: "ep-1", MacAddress: "aa:bb:cc:dd:ee:01", Name: "eth0"},
		"nic-2": {Id: "ep-2", MacAddress: "aa:bb:cc:dd:ee:02", Name: "eth1"},
	})

	env, err := src.Save(t.Context())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	dst, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if dst.netState != StateDestinationMigrating {
		t.Errorf("expected state DestinationMigrating after import, got %s", dst.netState)
	}
	if dst.namespaceID != src.namespaceID ||
		dst.policyBasedRouting != src.policyBasedRouting ||
		dst.isNamespaceSupportedByGuest != src.isNamespaceSupportedByGuest {
		t.Errorf("scalar config drifted across round-trip: src=%+v dst=%+v", src, dst)
	}
	if len(dst.vmEndpoints) != len(src.vmEndpoints) {
		t.Fatalf("expected %d endpoints, got %d", len(src.vmEndpoints), len(dst.vmEndpoints))
	}
	for nicID, want := range src.vmEndpoints {
		got, ok := dst.vmEndpoints[nicID]
		if !ok || got.Id != want.Id || got.MacAddress != want.MacAddress || got.Name != want.Name {
			t.Errorf("binding %s drifted: want %+v got %+v (present=%v)", nicID, want, got, ok)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Patch / Resume
// ─────────────────────────────────────────────────────────────────────────────

// TestPatch_RecordsDestinationNamespace verifies the destination namespace
// supplied for rebinding is retained for the later migration reset.
func TestPatch_RecordsDestinationNamespace(t *testing.T) {
	c := &Controller{netState: StateDestinationMigrating}

	c.Patch(t.Context(), "dst-ns")

	if c.migratedNamespaceID != "dst-ns" {
		t.Errorf("expected migrated namespace dst-ns, got %q", c.migratedNamespaceID)
	}
}

// TestResume_TransitionsToConfigured verifies that resuming returns a migrating
// controller to the operational state — binding the live VM/guest on the
// destination and rolling the snapshot back on the source.
func TestResume_TransitionsToConfigured(t *testing.T) {
	for _, st := range []State{StateDestinationMigrating, StateSourceMigrating} {
		t.Run(st.String(), func(t *testing.T) {
			c := &Controller{
				netState:    st,
				vmEndpoints: map[string]*hcn.HostComputeEndpoint{},
			}

			c.Resume(t.Context(), (*vmmanager.UtilityVM)(nil), (*guestmanager.Guest)(nil))

			if c.netState != StateConfigured {
				t.Errorf("expected state Configured after resume, got %s", c.netState)
			}
		})
	}
}
