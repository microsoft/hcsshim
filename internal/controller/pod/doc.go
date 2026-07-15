//go:build windows && (lcow || wcow)

// Package pod provides a controller for managing a single pod
// running inside a Utility VM (UVM). It owns the network controller and
// tracks all container controllers belonging to the pod.
//
// # Responsibilities
//
//   - Setting up and tearing down the pod-level network namespace via
//     the [network.Controller].
//   - Creating, retrieving, listing, and deleting container controllers
//     within the pod.
//
// # Migration
//
// Taking a snapshot blocks the pod's operations until migration is resumed,
// so its live state cannot diverge from the captured snapshot during handoff.
// A pod reconstructed from a snapshot on the destination is likewise blocked
// until resumed.
package pod
