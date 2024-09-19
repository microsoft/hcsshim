# Security Policy

This package contains the logic for enabling users to express an attested
security policy. This policy provides a series of enforcement points. Each
enforcement point contrains one action that the host requests of the guest.
The security policies are expressed in
[Rego](https://www.openpolicyagent.org/docs/latest/policy-language/),
a policy language designed for use in scenarios like this one.

We provide a [framework](./framework.rego) that users can employ to make
writing policies easier, but there is no requirement for this framework
to be used. Valid policies only need to define the enforcement points which
are enumerated in the [API](./api.rego) namespace.

## Adding a New Enforcement Point

When adding a new enforcement point, care must be taken to ensure that it is
correctly connected to the rest of the codebase and properly supported.
Here is a helpful checklist:

1.  Add the enforcment point to the
    [`SecurityPolicyEnforcer`](./securitypolicyenforcer.go) interface.
2.  Add stub implementations of the enforcement point to all classes which
    implement the interface. Some files to look at:
    - [`mountmonitoringsecuritypolicyenforcer.go`](../../internal/guest/storage/test/policy/mountmonitoringsecuritypolicyenforcer.go)
    - [`securitypolicyenforcer.go`](./securitypolicyenforcer.go)
    - [`securitypolicyenforcer_rego.go`](./securitypolicyenforcer_rego.go)
3.  Wrap the call in [`uvm.go`](../../internal/guest/runtime/hcsv2/uvm.go)
    so that it will not happen unless the security policy says it is OK.
4.  Add the enforcement point to [`api.rego`](./api.rego) and bump one minor
    version.
5.  Add the enforcement point rule to [`policy.rego`](./policy.rego) and
    [`open_door.rego`](./open_door.rego).
6.  Add the enforcement point rule logic to [`framework.rego`](./framework.rego)
7.  Add useful error messages to [`framework.rego`](./framework.rego). Be sure
    to gate them with the rule name.
8.  Update the internal representations of the policy in
    [`securitypolicy_internal.go`](./securitypolicy_internal.go) to contain any
    constraint objects which are needed by the framework logic.
9.  Update the Rego marshalling code in
    [`securitypolicy_marshal.go`](./securitypolicy_marshal.go) to emit the
    constraint objects which you added in the previous step.
10.  In [`securitypolicyenforcer_rego.go`](./securitypolicyenforcer_rego.go), fill
    out the stub with the input needed for the framework logic.
11. Add tests to [`regopolicy_test.go`](./regopolicy_test.go). As a rule, you
    should add one test which verifies that the rule enforces things correctly,
    and then at least one test per error condition. Be sure to test that the
    error messages you are emitting are present in the error message.
