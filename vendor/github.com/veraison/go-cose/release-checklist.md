# Release Checklist

## Overview

This document describes the checklist to publish a release via GitHub workflow.

The maintainers may periodically update this checklist based on feedback.

> [!NOTE]
> Make sure the dependencies in `go.mod` file are expected by the release.
> After updating `go.mod` file, run `go mod tidy` to ensure the `go.sum` file is also updated with any potential changes.

## Release Process

1. Determine a [SemVer2](https://semver.org/)-valid version prefixed with the letter `v` for release. For example, `v1.0.0-alpha.1`.
1. Determine the commit to be tagged and released.
1. Create an issue for voting with title similar to `vote: tag v1.0.0-alpha.1` with the proposed commit.
1. Wait for the vote pass.
1. Cut a release branch `release-X.Y` (e.g. `release-1.0`) if it does not exist. The voted commit MUST be the head of the release branch.
   - To cut a release branch directly on GitHub, navigate to `https://github.com/veraison/go-cose/tree/{commit}` and then follow the [creating a branch](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-and-deleting-branches-within-your-repository#creating-a-branch-using-the-branch-dropdown) doc.
1. Draft a new release from the release branch by following the [creating a release](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository#creating-a-release) doc. Set release title to the voted version and create a tag in the **Choose a tag** dropdown menu with the voted version as the tag name.
1. Proofread the draft release, and publish the release.
1. Announce the release in the community.
