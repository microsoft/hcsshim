//go:build windows && !wcow

// Package plan9 provides a manager for managing Plan9 file-share devices
// attached to a Utility VM (UVM).
//
// It handles adding and removing Plan9 shares on the host side via HCS modify
// calls. Guest-side mount operations (mapped-directory requests) are handled
// separately by the mount manager.
//
// # Deduplication and Reference Counting
//
// [Manager] deduplicates shares: if two callers add a share with identical
// [AddOptions], the second call reuses the existing share and increments an
// internal reference count rather than issuing a second HCS call. The share is
// only removed from the VM when the last caller invokes [Manager.RemoveFromVM].
//
// # Lifecycle
//
// Each share progresses through the states below.
// The happy path runs down the left column; the error path is on the right.
//
//	Allocate entry for the share
//	            │
//	            ▼
//	┌─────────────────────┐
//	│    sharePending     │
//	└──────────┬──────────┘
//	           │
//	   ┌───────┴────────────────────────────────┐
//	   │ AddPlan9 succeeds                      │ AddPlan9 fails
//	   ▼                                        ▼
//	┌─────────────────────┐         ┌──────────────────────┐
//	│     shareAdded      │         │    shareInvalid      │
//	└──────────┬──────────┘         └──────────────────────┘
//	           │ RemovePlan9 succeeds   (auto-removed from map)
//	           ▼
//	┌─────────────────────┐
//	│    shareRemoved     │  ← terminal; entry removed from map
//	└─────────────────────┘
//
// State descriptions:
//
//   - [sharePending]: entered when a new entry is allocated (by [Manager.ResolveShareName]
//     or the first [Manager.AddToVM] call). No HCS call has been made yet.
//   - [shareAdded]: entered once [vmPlan9Manager.AddPlan9] succeeds;
//     the share is live on the VM.
//   - [shareInvalid]: entered when [vmPlan9Manager.AddPlan9] fails;
//     the map entry is removed immediately so the next call can retry.
//   - [shareRemoved]: terminal state entered once [vmPlan9Manager.RemovePlan9] succeeds.
//
// Method summary:
//
//   - [Manager.ResolveShareName] pre-allocates a share name for the given [AddOptions]
//     without issuing any HCS call. If a matching share is already tracked,
//     the existing name is returned. This is useful for resolving downstream
//     resource paths (e.g., guest mount paths) before the share is live.
//   - [Manager.AddToVM] attaches the share, driving the HCS AddPlan9 call on
//     the first caller and incrementing the reference count on subsequent ones.
//     If the HCS call fails, the entry is removed so the next call can retry.
//   - [Manager.RemoveFromVM] decrements the reference count and tears down the
//     share only when the count reaches zero.
package plan9
