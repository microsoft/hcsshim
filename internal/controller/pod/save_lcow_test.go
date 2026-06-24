//go:build windows && lcow

package pod

import (
	"testing"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	lcsave "github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/save"
	netsave "github.com/Microsoft/hcsshim/internal/controller/network/save"
	"github.com/Microsoft/hcsshim/internal/controller/pod/mocks"
	podsave "github.com/Microsoft/hcsshim/internal/controller/pod/save"
	procsave "github.com/Microsoft/hcsshim/internal/controller/process/save"
)

// migratingContainer restores a real container into StateMigrating with a
// single init process whose IO paths are empty, so Patch needs no live guest.
func migratingContainer(t *testing.T, id string) *linuxcontainer.Controller {
	t.Helper()
	proc := mustAny(t, procsave.TypeURL, &procsave.Payload{SchemaVersion: procsave.SchemaVersion})
	env := mustAny(t, lcsave.TypeURL, &lcsave.Payload{
		SchemaVersion:  lcsave.SchemaVersion,
		ContainerID:    id,
		GcsContainerID: id,
		Processes:      map[string]*anypb.Any{"": proc},
	})
	ctr, err := linuxcontainer.Import(t.Context(), env)
	if err != nil {
		t.Fatalf("import migrating container %q: %v", id, err)
	}
	return ctr
}

// mustAny marshals a message and wraps it in the given typed envelope.
func mustAny(t *testing.T, typeURL string, m proto.Message) *anypb.Any {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal %s: %v", typeURL, err)
	}
	return &anypb.Any{TypeUrl: typeURL, Value: b}
}

// containerEnvelope builds a minimal, importable container envelope.
func containerEnvelope(t *testing.T, id string) *anypb.Any {
	t.Helper()
	return mustAny(t, lcsave.TypeURL, &lcsave.Payload{SchemaVersion: lcsave.SchemaVersion, ContainerID: id})
}

// newNetMock returns a network controller mock wired to a fresh gomock controller.
func newNetMock(t *testing.T) *mocks.MocknetworkController {
	t.Helper()
	return mocks.NewMocknetworkController(gomock.NewController(t))
}

// TestSave covers the snapshot envelope a caller receives, plus the error
// paths surfaced by the network and container children.
func TestSave(t *testing.T) {
	tests := []struct {
		name    string
		build   func(t *testing.T) *Controller
		wantErr bool
		// hasNet asserts whether the serialized payload carries a network envelope.
		hasNet bool
	}{
		{
			name: "no containers, nil network",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, gcsPodID: testPodID, containers: map[string]*linuxcontainer.Controller{}}
			},
		},
		{
			name: "no containers, with network",
			build: func(t *testing.T) *Controller {
				t.Helper()
				net := newNetMock(t)
				net.EXPECT().Save(gomock.Any()).Return(mustAny(t, netsave.TypeURL, &netsave.Payload{SchemaVersion: netsave.SchemaVersion}), nil)
				return &Controller{podID: testPodID, gcsPodID: testPodID, network: net, containers: map[string]*linuxcontainer.Controller{}}
			},
			hasNet: true,
		},
		{
			name: "network save fails",
			build: func(t *testing.T) *Controller {
				t.Helper()
				net := newNetMock(t)
				net.EXPECT().Save(gomock.Any()).Return(nil, errTest)
				return &Controller{podID: testPodID, gcsPodID: testPodID, network: net, containers: map[string]*linuxcontainer.Controller{}}
			},
			wantErr: true,
		},
		{
			name: "container not running",
			build: func(t *testing.T) *Controller {
				t.Helper()
				// A freshly created container is not StateRunning, so its Save fails.
				ctr := linuxcontainer.New("vm-1", testPodID, "container-1", nil, nil, nil, nil)
				return &Controller{podID: testPodID, gcsPodID: testPodID, containers: map[string]*linuxcontainer.Controller{"container-1": ctr}}
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.build(t)
			env, err := c.Save(t.Context())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if env.GetTypeUrl() != podsave.TypeURL {
				t.Errorf("type url = %q, want %q", env.GetTypeUrl(), podsave.TypeURL)
			}
			state := &podsave.Payload{}
			if err := proto.Unmarshal(env.GetValue(), state); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if (state.GetNetwork() != nil) != tt.hasNet {
				t.Errorf("network present = %v, want %v", state.GetNetwork() != nil, tt.hasNet)
			}
		})
	}
}

// TestSaveBlocksSourceUntilResume verifies that taking a snapshot marks the
// source pod as migrating so its operations are rejected until Resume.
func TestSaveBlocksSourceUntilResume(t *testing.T) {
	src := &Controller{podID: testPodID, gcsPodID: testPodID, containers: map[string]*linuxcontainer.Controller{}}

	if _, err := src.Save(t.Context()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !src.isMigrating {
		t.Fatal("expected source pod to be migrating after Save")
	}
	// A blocked operation now fails until Resume rebinds the live VM.
	if _, err := src.NewContainer(t.Context(), "container-1"); err == nil {
		t.Fatal("expected NewContainer to fail on a migrating source pod")
	}
}

// TestImport covers envelope validation and the fields a caller can observe
// on the reconstructed controller.
func TestImport(t *testing.T) {
	tests := []struct {
		name    string
		env     func(t *testing.T) *anypb.Any
		wantErr bool
		check   func(t *testing.T, c *Controller)
	}{
		{
			name:    "nil envelope",
			env:     func(t *testing.T) *anypb.Any { t.Helper(); return nil },
			wantErr: true,
		},
		{
			name:    "wrong type url",
			env:     func(t *testing.T) *anypb.Any { t.Helper(); return &anypb.Any{TypeUrl: "type.microsoft.com/bogus"} },
			wantErr: true,
		},
		{
			name: "corrupt payload",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return &anypb.Any{TypeUrl: podsave.TypeURL, Value: []byte{0xff}}
			},
			wantErr: true,
		},
		{
			name: "unsupported schema version",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return mustAny(t, podsave.TypeURL, &podsave.Payload{SchemaVersion: podsave.SchemaVersion + 1, PodID: testPodID})
			},
			wantErr: true,
		},
		{
			name: "network import fails",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return mustAny(t, podsave.TypeURL, &podsave.Payload{
					SchemaVersion: podsave.SchemaVersion,
					PodID:         testPodID,
					Network:       &anypb.Any{TypeUrl: "type.microsoft.com/bogus"},
				})
			},
			wantErr: true,
		},
		{
			name: "container import fails",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return mustAny(t, podsave.TypeURL, &podsave.Payload{
					SchemaVersion: podsave.SchemaVersion,
					PodID:         testPodID,
					Containers:    []*anypb.Any{{TypeUrl: "type.microsoft.com/bogus"}},
				})
			},
			wantErr: true,
		},
		{
			name: "valid, no containers",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return mustAny(t, podsave.TypeURL, &podsave.Payload{
					SchemaVersion: podsave.SchemaVersion,
					PodID:         testPodID,
					GcsPodID:      testPodID,
					Network:       mustAny(t, netsave.TypeURL, &netsave.Payload{SchemaVersion: netsave.SchemaVersion}),
				})
			},
			check: func(t *testing.T, c *Controller) {
				t.Helper()
				if c.podID != testPodID || c.gcsPodID != testPodID {
					t.Errorf("ids = (%q, %q), want (%q, %q)", c.podID, c.gcsPodID, testPodID, testPodID)
				}
				if c.network == nil {
					t.Error("expected non-nil network controller")
				}
				if len(c.containers) != 0 {
					t.Errorf("expected no containers, got %d", len(c.containers))
				}
				if !c.isMigrating {
					t.Error("expected imported pod to be migrating until Resume")
				}
			},
		},
		{
			name: "valid, with container",
			env: func(t *testing.T) *anypb.Any {
				t.Helper()
				return mustAny(t, podsave.TypeURL, &podsave.Payload{
					SchemaVersion: podsave.SchemaVersion,
					PodID:         testPodID,
					GcsPodID:      testPodID,
					Network:       mustAny(t, netsave.TypeURL, &netsave.Payload{SchemaVersion: netsave.SchemaVersion}),
					Containers:    []*anypb.Any{containerEnvelope(t, "container-1")},
				})
			},
			check: func(t *testing.T, c *Controller) {
				t.Helper()
				if _, ok := c.containers["container-1"]; !ok {
					t.Error("expected container-1 to be re-keyed by its restored ID")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Import(t.Context(), tt.env(t))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, c)
		})
	}
}

// TestSaveImportRoundTrip verifies that a saved pod reconstructs to an
// equivalent controller a caller can observe.
func TestSaveImportRoundTrip(t *testing.T) {
	net := newNetMock(t)
	net.EXPECT().Save(gomock.Any()).Return(mustAny(t, netsave.TypeURL, &netsave.Payload{SchemaVersion: netsave.SchemaVersion}), nil)
	src := &Controller{podID: testPodID, gcsPodID: testPodID, network: net, containers: map[string]*linuxcontainer.Controller{}}

	env, err := src.Save(t.Context())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Import(t.Context(), env)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got.podID != testPodID || got.gcsPodID != testPodID {
		t.Errorf("ids = (%q, %q), want (%q, %q)", got.podID, got.gcsPodID, testPodID, testPodID)
	}
	if got.network == nil {
		t.Error("expected non-nil network controller after round trip")
	}
}

// TestResume covers binding a live VM and re-wiring the network for an
// imported pod with no containers, on both the destination and source sides.
func TestResume(t *testing.T) {
	tests := []struct {
		name          string
		isDestination bool
		scsiErr       error
		resetErr      error
		wantErr       bool
	}{
		{name: "destination happy path", isDestination: true},
		{name: "scsi controller fails", isDestination: true, scsiErr: errTest, wantErr: true},
		{name: "network reset fails", isDestination: true, resetErr: errTest, wantErr: true},
		{name: "source skips network reset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := gomock.NewController(t)
			vm := mocks.NewMockvmController(mc)
			net := mocks.NewMocknetworkController(mc)

			vm.EXPECT().VM().Return(nil)
			vm.EXPECT().Guest().Return(nil)
			net.EXPECT().Resume(gomock.Any(), gomock.Any(), gomock.Any())
			vm.EXPECT().SCSIController(gomock.Any()).Return(nil, tt.scsiErr)
			// ResetAfterMigration runs only on the destination once SCSI lookup succeeds.
			if tt.isDestination && tt.scsiErr == nil {
				net.EXPECT().ResetAfterMigration(gomock.Any()).Return(tt.resetErr)
			}

			c := &Controller{podID: testPodID, gcsPodID: testPodID, network: net, containers: map[string]*linuxcontainer.Controller{}, isMigrating: true}
			err := c.Resume(t.Context(), vm, nil, tt.isDestination)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resume() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Resume clears the migrating guard so normal ops are allowed again.
			if c.isMigrating {
				t.Error("expected isMigrating to be cleared after Resume")
			}
		})
	}
}

// TestPatch covers request validation, container lookup, ID-collision
// rejection, delegation errors, and the successful retarget (including the
// sandbox identity/namespace adoption) a caller can trigger.
func TestPatch(t *testing.T) {
	tests := []struct {
		name      string
		build     func(t *testing.T) *Controller
		sourceID  string
		isSandbox bool
		request   *task.CreateTaskRequest
		spec      specs.Spec
		wantErr   bool
		check     func(t *testing.T, c *Controller)
	}{
		{
			name: "nil request",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{}}
			},
			request: nil,
			wantErr: true,
		},
		{
			name: "empty request id",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{}}
			},
			request: &task.CreateTaskRequest{ID: ""},
			wantErr: true,
		},
		{
			name: "source container not found",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{}}
			},
			sourceID: "missing",
			request:  &task.CreateTaskRequest{ID: "dst"},
			wantErr:  true,
		},
		{
			name: "destination id already exists",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{
					"src": linuxcontainer.New("vm-1", testPodID, "src", nil, nil, nil, nil),
					"dst": linuxcontainer.New("vm-1", testPodID, "dst", nil, nil, nil, nil),
				}}
			},
			sourceID: "src",
			request:  &task.CreateTaskRequest{ID: "dst"},
			wantErr:  true,
		},
		{
			name: "delegated container patch fails",
			build: func(t *testing.T) *Controller {
				t.Helper()
				// A non-migrating container rejects Patch, surfacing as a wrapped error.
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{
					"src": linuxcontainer.New("vm-1", testPodID, "src", nil, nil, nil, nil),
				}}
			},
			sourceID: "src",
			request:  &task.CreateTaskRequest{ID: "src"},
			wantErr:  true,
		},
		{
			name: "workload container retargeted and re-keyed",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{"src": migratingContainer(t, "src")}}
			},
			sourceID: "src",
			request:  &task.CreateTaskRequest{ID: "dst"},
			check: func(t *testing.T, c *Controller) {
				t.Helper()
				if _, ok := c.containers["dst"]; !ok {
					t.Error("expected container to be re-keyed to dst")
				}
				if _, ok := c.containers["src"]; ok {
					t.Error("expected old src key to be removed")
				}
				if c.podID != testPodID {
					t.Errorf("podID = %q, want unchanged %q for a workload container", c.podID, testPodID)
				}
			},
		},
		{
			name: "sandbox adopts pod id and namespace",
			build: func(t *testing.T) *Controller {
				t.Helper()
				net := newNetMock(t)
				net.EXPECT().Patch(gomock.Any(), "ns-dst")
				return &Controller{podID: testPodID, network: net, containers: map[string]*linuxcontainer.Controller{"src": migratingContainer(t, "src")}}
			},
			sourceID:  "src",
			isSandbox: true,
			request:   &task.CreateTaskRequest{ID: "sbx-dst"},
			spec:      specs.Spec{Windows: &specs.Windows{Network: &specs.WindowsNetwork{NetworkNamespace: "ns-dst"}}},
			check: func(t *testing.T, c *Controller) {
				t.Helper()
				if c.podID != "sbx-dst" {
					t.Errorf("podID = %q, want %q", c.podID, "sbx-dst")
				}
				if _, ok := c.containers["sbx-dst"]; !ok {
					t.Error("expected sandbox container to be re-keyed to sbx-dst")
				}
			},
		},
		{
			name: "sandbox without namespace fails",
			build: func(t *testing.T) *Controller {
				t.Helper()
				return &Controller{podID: testPodID, containers: map[string]*linuxcontainer.Controller{"src": migratingContainer(t, "src")}}
			},
			sourceID:  "src",
			isSandbox: true,
			request:   &task.CreateTaskRequest{ID: "sbx-dst"},
			spec:      specs.Spec{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.build(t)
			err := c.Patch(t.Context(), tt.sourceID, tt.isSandbox, nil, tt.request, tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Patch() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}
