---
applyTo:
  - "cmd/containerd-shim-runhcs-v1/**/*.go"
  - "cmd/containerd-shim-lcow-v2/**/*.go"
  - "internal/hcsoci/**/*.go"
  - "internal/uvm/**/*.go"
  - "internal/layers/**/*.go"
  - "internal/hcs/**/*.go"
  - "internal/gcs/**/*.go"
  - "internal/cow/**/*.go"
  - "internal/resources/**/*.go"
  - "internal/jobcontainers/**/*.go"
  - "internal/guest/**/*.go"
  - "internal/shim/**/*.go"
  - "internal/controller/**/*.go"
  - "internal/vm/**/*.go"
---

# V2 Shim — Code Review Rules

Review rules for the containerd shim v2 path: shim binaries, HCS/OCI bridge,
UVM lifecycle, resource management, guest compute service, VM controller,
and container/process abstractions.

The **primary lens** is Go conventions and best practices. The hcsshim-specific
rules below extend — never override — standard Go guidelines.

---

## Go Conventions & Best Practices

### Naming
- Follow [Effective Go](https://go.dev/doc/effective_go) naming: MixedCaps, no underscores in Go names.
- Package names are lowercase, single-word, no plurals. Avoid stutter (`hcs.HCSSystem` -> `hcs.System`).
- Interfaces named after the method when single-method (`io.Reader`, not `io.IReader`).
- Acronyms are all-caps (`ID`, `HTTP`, `UVM`), not `Id`, `Http`.

### Exported vs Unexported
- **If an exported symbol has no callers outside its package, unexport it.**
- Flag new exports that are only used internally — keep the API surface minimal.
- Exported types, functions, and methods MUST have doc comments (`// TypeName ...`).
- Doc comments start with the symbol name and describe *what*, not *how*.

### Error Handling
- Use `%w` for error wrapping with `fmt.Errorf`; flag bare `%v` on error values.
- Return errors, don't panic. Panics are only for truly unrecoverable programmer bugs.
- Check every returned error. Flag `_ = fn()` where `fn` returns an error (unless in cleanup with a log).
- Use `errors.Is` / `errors.As` for sentinel and type checks, never `==`.
- Prefer typed errors or sentinel errors over string matching.

### Resource Management
- Every `io.Closer` must be `Close()`d — prefer `defer obj.Close()` immediately after creation.
- If `Close()` can fail and it matters, capture the error: `defer func() { retErr = obj.Close() }()`.
- Every handle (`os.File`, `syscall.Handle`, `windows.Handle`) must be closed in error paths.
- Flag resources created in a loop without per-iteration cleanup.

### Concurrency
- Always pass `context.Context` as the first parameter; never store it in a struct.
- Goroutines must respect `ctx.Done()`. Flag goroutines without cancellation.
- Protect shared mutable state with `sync.Mutex` or channels. Flag unprotected access.
- Never capture loop variables in goroutine closures without rebinding (pre-Go 1.22).
- Prefer `sync.Once` for lazy initialization over manual bool + mutex patterns.

### Interfaces
- Keep interfaces small (1-3 methods). Accept interfaces, return concrete types.
- Define interfaces at the consumer, not the implementer.
- Flag empty interfaces (`interface{}`/`any`) when a specific type would work.

### Tests
- Test helpers must call `t.Helper()`.
- Use `t.Cleanup()` for teardown instead of manual defers when appropriate.
- Use table-driven tests for repetitive cases.
- Test names: `TestFunctionName_Condition_ExpectedResult`.
- Use `cmp.Diff` for struct comparison; `maps.Clone()` for copying maps.
- Use `functional` build tag for tests needing a live VM or container.

### Packages & Imports
- Group imports: stdlib, external, internal. Use `goimports` ordering.
- No circular dependencies. Flag new cross-package imports that break layering.
- Internal packages should not leak implementation details through exported types.

---

## hcsshim-Specific Rules

### Resource Lifecycle (CRITICAL)

- Every allocated resource MUST be registered with `r.Add(...)` or `r.SetLayers(...)`.
- Flag any `ResourceCloser` returned from a helper that is NOT added to the resource tracker.
- Flag resources allocated inside a loop where early `return` could skip `r.Add(...)`.
- `CreateContainer` and similar orchestrators MUST `defer` a call to
  `resources.ReleaseResources(ctx, r, vm, true)` on error.
- `ResourceCloserList.Release()` releases in REVERSE order. Do not manually reorder.

### HCS / UVM Object Lifecycle

- Every `hcs.System` or `cow.Container` MUST be `Close()`d.
- Every `cow.Process` MUST be `Close()`d after `Wait()` completes.
- `uvm.CreateLCOW` / `uvm.CreateWCOW` returns a UVM that MUST be `Close()`d on error.
- `uvm.Start()` must be called after creation; flag orphaned UVMs.
- Flag SCSI, vSMB, vPMEM, or Plan9 mounts added to a UVM but not tracked for cleanup.

### Memory & Handle Leaks

- Flag `syscall.Handle` or `windows.Handle` values not closed in error paths.
- Flag goroutines that capture a `*cow.Process` or `*hcs.System` without ensuring
  the object outlives the goroutine.

### Concurrency (hcsshim-specific)

- `hcs.System` operations are NOT goroutine-safe. Flag concurrent access without sync.
- UVM device maps (`scsiLocations`, `vsmb`, `plan9`, `vpmem`) are mutex-protected;
  flag direct map access without holding the lock.

### Package Layering

Flag these violations:
- `internal/cow` importing anything above it (must be pure abstraction).
- `internal/hcs` importing from `internal/hcsoci` or `internal/uvm`.
- `internal/resources` importing from shim-level packages.
- `cmd/` directly importing `internal/hcs` instead of going through `internal/hcsoci`.
- `internal/controller/vm` importing from `internal/hcsoci`.

### VM Controller & State Machine

- Every state transition MUST be validated against allowed transitions.
- Flag direct state assignment that bypasses the transition validation function.
- Terminal states (`stopped`, `failed`) must not allow further transitions.
- `vmmanager.LifetimeManager` and `guestmanager.Manager` MUST handle idempotent `Stop()`/`Close()`.
- Flag implementations that panic or return untyped errors on double-close.

### LCOW Shim V2 Service

- Every containerd RPC handler MUST have proper context propagation and cancellation.
- Flag handlers that block indefinitely without respecting `ctx.Done()`.
- Flag missing cleanup in `Delete` — all resources from `Create` must be released.
- The shim MUST NOT leak UVM references across task boundaries.

---

## Review Output

- Max 2 comments per concern; group related items.
- Use **[BLOCKER]** only for resource leaks, correctness, safety, or API-breaking issues.
- Use **[ISSUE]** for likely bugs or pattern deviations.
- Use **[SUGGESTION]** for non-blocking improvements.
