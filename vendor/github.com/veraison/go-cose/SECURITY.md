# Security Process and Policy

This document provides the details on the veraison/go-cose security policy and details the processes surrounding security handling.

## Supported Versions

The current stable release of [go-cose][go-cose] is [v1.0.0][v1.0.0-release]. Please upgrade to [v1.0.0][v1.0.0-release] if you are using a pre-release version.

| Version | Supported          |
| ------- | ------------------ |
| [v1.0.0][v1.0.0-release]  | Yes |

## Report A Vulnerability

We’re extremely grateful for security researchers and users who report vulnerabilities
to the [veraison/go-cose][go-cose] community. All reports are thoroughly investigated by a set of [veraison/go-cose maintainers][go-cose-maintainers].

To make a report please email the private security list at <a href="mailto:go-cose-security@googlegroups.com?subject=go-cose Security Notification">go-cose-security@googlegroups.com</a> with details using the following template:

### Reporting Template

```console
[TO:]:     go-cose-security@googlegroups.com
[SUBJECT]: go-cose Security Notification
[BODY]:
Release: v1.0.0

Summary:
A quick summary of the issue

Impact:
Details on how to reproduce the security issue.

Contact:
Information on who to contact for additional information
```

### When To Send a Report

You think you have found a vulnerability in the [veraison/go-cose][go-cose] project.

### Security Vulnerability Response

Each report will be reviewed and receipt acknowledged in a timely manner. This will set off the security review process detailed below.

Any vulnerability information shared with the security team stays within the [veraison/go-cose][go-cose] project and will not be shared with others unless it is necessary to fix the issue. Information is shared only on a need to know basis.

We ask that vulnerability reporter(s) act in good faith by not disclosing the issue to others. And we strive to act in good faith by acting swiftly, and by justly crediting the vulnerability reporter(s) in writing (see [Public Disclosure](#public-disclosure)).

As the security issue moves through triage, identification, and release the reporter of the security vulnerability will be notified. Additional questions about the vulnerability map may also be asked from the reporter.

### Public Disclosure

A public disclosure of security vulnerabilities is released alongside release updates or details that fix the vulnerability. We try to fully disclose vulnerabilities once a mitigation strategy is available. Our goal is to perform a release and public disclosure quickly and in a timetable that works well for users. For example, a release may be ready on a Friday but for the sake of users may be delayed to a Monday.

When needed, CVEs will be assigned to vulnerabilities. Due to the process and time it takes to obtain a CVE ID, disclosures will happen first. Once the disclosure is public the process will begin to obtain a CVE ID. Once the ID has been assigned the disclosure will be updated.

If the vulnerability reporter would like their name and details shared as part of the disclosure process we are happy to. We will ask permission and for the way the reporter would like to be identified. We appreciate vulnerability reports and would like to credit reporters if they would like the credit.

## Security Team Membership

The security team is made up of a subset of the Veraison project maintainers who are willing and able to respond to vulnerability reports.

### Responsibilities

* Members MUST be active project maintainers on active (non-deprecated) Veraison projects as defined in the [governance](https://github.com/veraison/community/blob/main/GOVERNANCE.md)
* Members SHOULD engage in each reported vulnerability, at a minimum to make sure it is being handled
* Members MUST keep the vulnerability details private and only share on a need to know basis

### Membership

New members are required to be active maintainers of Veraison projects who are willing to perform the responsibilities outlined above. The security team is a subset of the maintainers across Veraison sub-projects. Members can step down at any time and may join at any time.

If at any time a security team member is found to be no longer an active maintainer on active Veraison sub-projects, this individual will be removed from the security team.

## Patch and Release Team

When a vulnerability comes in and is acknowledged, a team - including maintainers of the Veraison project affected - will be assembled to patch the vulnerability, release an update, and publish the vulnerability disclosure. This may expand beyond the security team as needed but will stay within the pool of Veraison project maintainers.

## Disclosures

Vulnerability disclosures are published to [security-advisories][security-advisories]. The disclosures will contain an overview, details about the vulnerability, a fix for the vulnerability that will typically be an update, and optionally a workaround if one is available.

Disclosures will be published on the same day as a release fixing the vulnerability after the release is published.

[go-cose]:                https://github.com/veraison/go-cose
[security-advisories]:    https://github.com/veraison/go-cose/security/advisories
[v1.0.0-release]:  https://github.com/veraison/go-cose/releases/tag/v1.0.0
[go-cose-maintainers]:    https://github.com/veraison/community/blob/main/OWNERS
