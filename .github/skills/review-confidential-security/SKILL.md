---
name: review-confidential-security
description: Review a commit, PR, or local diff for confidential (C-LCOW / C-WCOW) enforcement security regressions. Use when the user asks to "audit", "security review", "check for confidentiality / enforcement bugs", or otherwise vet changes that touch GCS, gcs-sidecar, the security policy, OCI spec handling, or any host-driven request path in hcsshim.
---

# Reviewing a change for confidential-containers security bugs

Before starting, read
[.github/instructions/about-confidential-containers.instructions.md](../../.github/instructions/about-confidential-containers.instructions.md)
for the trust model and the existing hardening conventions you must not
regress.  Note that this skill is only about confidential enforcement. If you're
doing a general PR review, consider other bugs too, including on the host.

## 1. Get the diff

- If a commit/PR/range is named, fetch it (`git show <sha>`,
  `git log -p <range>`, or the GitHub PR tools). Otherwise diff the working
  tree (`git diff` / `git diff main...HEAD`).
- Read the commit message(s) and PR description first — they often state
  intent that the code does not.

## 2. Classify the change

Decide which trust boundary the change touches. Anything in this list is
**in scope** for a confidential review:

- `internal/guest/**`, `cmd/gcs*` — C-LCOW trusted code.
- `internal/gcs-sidecar/**`, `cmd/gcs-sidecar/**` — C-WCOW trusted code.
- `pkg/securitypolicy/**` — Rego policy, framework, enforcer, interpreter
  bindings.
- `internal/regopolicyinterpreter/**` — Rego interpreter glue.
- `internal/guest/bridge/**` — request dispatch and sequential-mode
  enforcement.
- `internal/protocol/**`, `internal/guestresource/**`,
  `internal/guestrequest/**` — shapes of host-driven messages.
- Any handler that consumes an OCI spec, mount/unmount request, exec
  request, network/hostname config, fragment, or attestation input.

Out-of-scope (host-only, or guest code that only executes on non-confidential
mode) changes do not have to be covered by this review because we assume the
host can be malicious anyway.

## 3. What counts as a confidential enforcement bug

A finding is only a confidential enforcement bug if a **malicious host** can
cause an outcome that the **policy author did not sanction**. The trust
model (see the instructions file) puts the policy author and the container
inside the boundary, so:

- Bug: host bypasses or weakens a policy check, reads/modifies data in the
  container in a way the user does not intend, or tricks the container into doing
  something it wouldn't otherwise do.
- Not a bug: the policy can allow something dangerous; a privileged container
  can escape into the rest of the UVM, etc.

When something looks like a policy bypass, ask: *does this work without the user
needing to open doors in the security policy?* If no, it's not a confidential
bug.

(Obviously, a non-confidential-security-policy-enforcement-related bug is still
a bug and can be severe, like unprivileged containers gaining privilege with a
malicious image / OCI spec, and worse case, escaping to the host, etc)

## 4. Checklist — apply to every changed file in scope

Treat **everything** coming from the host as adversarial: bridge message
fields, OCI spec fields (annotations, env, args, mounts, hooks, devices,
process.user, capabilities, seccomp, hostname, …), mount options, share
names, paths, request types, settings payloads, all of it.

A lot of this code predates the C-ACI project, so when modifying or extending an
existing handler **do not assume the existing pattern is safe just because it's
there** — re-derive whether each host-controlled field is constrained.

For each new or modified host-facing input field, ask:

1. **Source of trust.** Where does this value originate? If it comes from
   the bridge, an OCI spec, a `ModificationRequest.Settings`, an annotation,
   or any RPC payload, treat it as adversarial.
2. **Validation.** Is it constrained? Look for:
   - A Rego rule that gates the field (added to `framework.rego` /
     `api.rego` / a customer's `policy.rego`).
   - A Go-side validator (regex, length, path equality, `checkValidContainerID`,
     `checkContainerSettings`, allowlist of mount options, …).
   - For paths: is the canonical form checked, or is the value used to build a path
     that escapes the intended directory?
3. **Order.** Does validation happen **before** any irreversible side
   effect (mount, unmount, runc exec, file write, fragment load, network
   change)? A common bug is `os.MkdirAll(hostControlledPath, ...)` before
   the policy check.
4. **Opaque forwarding.** Is the change passing a struct, map, or JSON
   blob through to a component outside GCS — runc, the Windows inbox GCS,
   `exec`, the kernel mount syscall, an external process? If yes, every
   field of that blob needs to be either validated, sanitized, or
   explicitly known-safe. The runc-hooks bug (`b6907ec6c`) is the
   canonical example.
5. **Enforcement coverage.** If the change adds a new bridge resource
   type, request type, or modify settings case, is there a corresponding
   enforcement point (or an explicit reason it's safe without one)?
   Pass-through to the inbox GCS without enforcement on C-WCOW is a bug.
6. **Metadata consistency.** If the handler mutates state, is it inside a
   revertable section (`StartRevertableSection` +
   `commitOrRollbackPolicyRevSection`)? Does the underlying operation
   clean up its own partial state on error? Does an unmount-style failure
   call `setMountsBrokenIfConfidential`?
7. **Concurrency.** Is the change re-introducing parallelism on a
   confidential-mode code path? Sequential dispatch (`8b3d250b1`) is load
   bearing — concurrent mount + create was an exploit.
8. **Lifecycle invariants.** Does the change weaken any of these:
   - Unmount overlay before unmount layers/scratch.
   - No unmount of an in-use overlay (`IsOverlayInUse`).
   - No `DeleteContainerState` while the container runs or its overlay is
     mounted.
   - `hostMounts` lock held by caller across check + mutation.
9. **Error/log content.** Does the change log or return host-controlled
   data without going through `redactSensitiveData` /
   `replaceCapabilitiesWithPlaceholders`? Env values especially must be
   redacted.
10. **Rego changes.**
    - Did `version_framework` / `version_api` get bumped if behavior
      observable to existing policies changed?
    - Are new container/external_process/fragment fields fronted with
      `apply_defaults` / `check_*` shims so older policies still load?
    - Does every narrowing step have a matching `errors[...]` rule so
      denials are debuggable?
11. **C-WCOW symmetry.** For changes to shared code, did both osType
    branches in the Rego enforcer get updated? Does the gcs-sidecar dispatch
    cover the same cases as the GCS?

## 5. Produce the review

For confirmed or very likely vulnerability:
Include the file, line range, what unintended action can the host achieve, if
feasible, code snippets modifying the host-side component to demonstrate an
attack, and a concrete fix (e.g., add the Rego rule, add a validation in Go
code, move a check earlier, or ignore / reset a host-controlled field).

Do not produce nits if they are not related to security.
