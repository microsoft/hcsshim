# go-cose Release Management

## Overview

This document describes [go-cose][go-cose] project release management, which includes release criteria, versioning, supported releases, and supported upgrades.

The go-cose project maintainers strive to provide a stable go-lang implementation for interacting with [COSE][ietf-cose] constructs.
Stable implies appropriate and measured changes to the library assuring consumers have the necessary functionality to interact with COSE objects.
If you or your project require added functionality, or bug fixes, please open an issue or create a pull request.
The project welcomes all contributions from adding functionality, implementing testing, security reviews to the release management.
Please see [here](https://github.com/veraison#contributing) for how to contribute.

The maintainers may periodically update this policy based on feedback.

## Release Versioning

Consumers of the go-cose module may reference `main` directly, or reference released tags.

All go-cose [releases][releases] follow a go-lang flavored derivation (`v*`) of the [semver][sem-ver] format, with optional pre-release labels.

Logical Progression of a release: `v1.0.0-alpha.1` --> `v1.0.0-alpha.2` --> `v1.0.0-rc.1` --> `v1.0.0`

A new major or minor release will not have an automated release posted until the branch reaches alpha quality.

- All versions use a preface of `v`
- Given a version `vX.Y.Z`,
  - `X` is the [Major](#major-releases) version
  - `Y` is the [Minor](#minor-releases) version
  - `Z` is the [Patch](#patch-releases) version
- _Optional_ `-alpha.n` | `-rc.n` [pre-release](#pre-release) version
  - Each incremental alpha or rc build will bump the suffix (`n`) number.
  - It's not expected to have more than 9 alphas or rcs.
The suffix will be a single digit.
  - If > 9 builds do occur, the format will simply use two digit indicators (`v1.0.0-alpha.10`)

> [!IMPORTANT]
> Pre-releases will NOT use the github pre-release flag.

## Branch Management

To meet the projects stability goals, go-cose does not currently operate with multiple feature branches.
All active development happens in `main`.
For each release, a branch is created for servicing, following the versioning name.
`v1.0.0-alpha-1` would have an associated [v1.0.0-alpha.1](https://github.com/veraison/go-cose/tree/v1.0.0-alpha.1) branch.
All version and branch names are lower case.

### Major Releases

As a best practice, consumers should opt-into new capabilities through major releases.
The go-cose project will not add new functionality to patches or minor releases as this could create a new surface area that may be exploited.
Consumers should make explicit opt-in decisions to upgrade, or possibly downgrade if necessary due to unexpected breaking changes.

The go-cose project will issue major releases when:

- Functionality has changed
- Breaking changes are required

Each major release will go through one or more `-alpha.n` and `-rc.n` pre-release phases.

### Minor Releases

The go-cose project will issue minor releases when incremental improvements, or bug fixes are added to existing functionality.
Minor releases will increment the minor field within the [semver][sem-ver] format.

Each minor release will go through one or more `-alpha.n` and `-rc.n` pre-release phases.

### Patch Releases

Patch Releases include bug and security fixes.
Patches will branch from the released branch being patched.
Fixes completed in main may be ported to a patch release if the maintainers believe the effort is justified by requests from the go-cose community.
If a bug fix requires new incremental, non-breaking change functionality, a new minor release may be issued.

Principals of a patch release:

- Should be a "safe bet" to upgrade to.
- No breaking changes.
- No feature or surface area changes.
- A "bug fix" that may be a breaking change may require a major release.
- Applicable fixes, including security fixes, may be cherry-picked from main into the latest supported minor `release-X.Y` branches.
- Patch releases are cut from a `release-X.Y` branch.

Each patch release will go through one or more `-alpha.n` and `-rc.n` pre-release phases.

### Pre-Release

As builds of `main` become stable, and a pending release is planned, a pre-release build will be made.
Pre-releases go through one or more `-alpha.n` releases, followed by one or more incremental `-rc.n` releases.

- **alpha.n:** `X.Y.Z-alpha.n`
  - alpha release, cut from the branch where development occurs.
To minimize branch management, no additional branches are maintained for each incremental release.
  - Considered an unstable release which should only be used for early development purposes.
  - Released incrementally until no additional issues and prs are made against the release.
  - Once no triaged issues or pull requests (prs) are scoped to the release, a release candidate (`rc`) is cut.
  - To minimize confusion, and the risk of an alpha being widely deployed, alpha branches and released binaries may be removed at the discretion, and a [two-thirds supermajority][super-majority] vote, of the maintainers.
Maintainers will create an Issue, and vote upon it for transparency to the decision to remove a release and/or branch.
  - Not [supported](#supported-releases)
- **rc.n:** `X.Y.Z-rc.n`
  - Released as needed before a final version is released
  - Bugfixes on new features only as reported through usage
  - An `rc` is not expected to revert to an alpha release.
  - Once no triaged issues or PRs are scoped to the release, an final version is cut.
  - A release candidate will typically have at least two weeks of bake time, providing the community time to provide feedback.
  - Release candidates are cut from the branch where the work is done.
  - To minimize confusion, and the risk of an rc being widely deployed, rc branches and released binaries may be removed at the discretion, and a [two-thirds supermajority][super-majority] vote, of the maintainers.
Maintainers will create an Issue, and vote upon it for transparency to the decision to remove a release and/or branch.
  - Not [supported](#supported-releases)

## Supported Releases

The go-cose maintainers expect to "support" n (current) and `n-1` major.minor releases.
"Support" means we expect users to be running that version in production.
For example, when `v1.3.0` comes out, `v1.1.x` will no longer be supported for patches, and the maintainers encourage users to upgrade to a supported version as soon as possible.
Support will be provided best effort by the maintainers via GitHub issues and pull requests from the community.

The go-cose maintainers expect users to stay up-to-date with the versions of go-cose release they use in production, but understand that it may take time to upgrade.
We expect users to be running approximately the latest patch release of a given minor release and encourage users to upgrade as soon as possible.

While pre-releases may be deleted at the discretion of the maintainers, all Major, Minor and Patch releases should be maintained.
Only in extreme circumstances, as agreed upon by a [two-thirds supermajority][super-majority] of the maintainers, shall a release be removed.

Applicable fixes, including security fixes, may be cherry-picked into the release branch, depending on severity and feasibility.
Patch releases are cut from that branch as needed.

## Security Reviews

The go-cose library is an sdk around underlying crypto libraries, tailored to COSE scenarios.
The go-cose library does not implement cryptographic functionality, reducing the potential risk.
To assure go-cose had the proper baseline, two [security reviews](./reports) were conducted prior to the [v1.0.0](https://github.com/veraison/go-cose/releases/tag/v1.0.0) release

For each release, new security reviews are evaluated by the maintainers as required or optional.
The go-cose project welcomes additional security reviews.
See [SECURITY.md](./SECURITY.md) for more information.

## Glossary of Terms

- **X.Y.Z** refers to the version (based on git tag) of go-cose that is released.
This is the version of the go-cose binary.
- **Breaking changes** refer to schema changes, flag changes, and behavior changes of go-cose that may require existing content to be upgraded and may also introduce changes that could break backward compatibility.
- **Milestone** GitHub milestones are used by maintainers to manage each release.
PRs and Issues for each release should be created as part of a corresponding milestone.
- **Patch releases** refer to applicable fixes, including security fixes, may be backported to support releases, depending on severity and feasibility.

## Attribution

This document builds on the ideas and implementations of release processes from the [notation](https://github.com/notaryproject/notation) project.

[go-cose]:        https://github.com/veraison/go-cose
[ietf-cose]:      https://datatracker.ietf.org/group/cose/about/
[sem-ver]:        https://semver.org
[releases]:       https://github.com/veraison/go-cose/releases
[super-majority]: https://en.wikipedia.org/wiki/Supermajority#Two-thirds_vote
