# Sidecar GCS ETW logging

Sidecar GCS emits runtime diagnostics through the
`Microsoft.Virtualization.GCSSidecar` ETW provider
(`5400e7dc-0f0f-553d-19b5-3672635b5f7b`), using the same Logrus ETW hook as
the host shim. Log Forward Service collects these events and forwards them
through the existing Windows logging socket, not the GCS control bridge.

## Policy

The ETW hook starts disabled. After the guest validates and installs the
confidential policy, `allow_runtime_logging` controls both ordinary runtime
output and ETW emission. The existing Log Forward Service provider policy
separately controls collection. There is no additional forwarding permission.

Example policy excerpt (not a complete policy):

```rego
allow_runtime_logging := true
allowed_log_providers := [
    "Microsoft.Virtualization.GCSSidecar",
]
allow_log_provider_dropping := false
log_provider := data.framework.log_provider
```

Existing versioned policy compatibility defaults still apply. Older policies
can allow collection without an explicit provider list.

## Collection

The provider is included in the default sources at Information level. Enable
forwarding with `io.microsoft.virtualmachine.forwardlogs.enable = true`.
C-WCOW forwarding remains disabled by default. Every requested default provider
must satisfy provider policy: a disallowed provider is dropped if dropping is
enabled, or the request fails otherwise. Under the existing confidential startup
behavior, that failure aborts UVM startup, even if runtime logging is disabled.

To collect only sidecar logs at Verbose level, set
`io.microsoft.virtualmachine.forwardlogs.defaultsources.enable = false` and
set `io.microsoft.virtualmachine.forwardlogs.sources` to the base64 encoding of:

```json
{
  "LogConfig": {
    "sources": [{
      "type": "ETW",
      "providers": [{
        "providerName": "Microsoft.Virtualization.GCSSidecar",
        "level": "Verbose"
      }]
    }]
  }
}
```

Use the same canonical provider name when overriding its default level.
The sidecar Logrus level also filters events; debug and trace map to ETW
Verbose. Existing guest-local file output is retained when runtime logging is
allowed. No events suppressed before policy installation are replayed.

Delivery is best effort. ETW events emitted before collection starts may be
lost; direct stderr writes and Go panic dumps are not forwarded by the hook.
Provider initialization failures are reported locally without terminating the
sidecar. Stopping Log Forward Service stops forwarding, not authorized local
ETW emission.
