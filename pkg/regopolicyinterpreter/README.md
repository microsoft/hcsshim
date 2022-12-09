# Rego Policy Interpreter

This module provides a general purpose Rego Policy interpreter. This is
used both by the [security_policy](../securitypolicy/) package, as well
as the [policy engine simulator](../../internal/tools/policyenginesimulator/).

## Metadata

Each rule in a policy can optionally return a series of metadata commands in addition to
`allowed` which will then be made available in the `data.metadata` namespace
for use by the policy in future rule evaluations. A metadata command has the
following format:

``` json
{
    {
        "name": "<metadata key>",
        "action": "<add|update|remove>",
        "key": "<key>",
        "value": "<optional value>"
    }
}
```

Metadata values can be any Rego object, *i.e.* arbitrary JSON. Importantly,
the Go code does not need to understand what they are or what they contain, just
place them in the specified point in the hierarchy such that the policy can find
them in later rule evaluations. To give a sense of how this works, here are a
sequence of rule results and the resulting metadata state:

**Initial State**
``` json
{
    "metadata": {}
}
```

**Result 1**
``` json
{
    "allowed": true,
    "metadata": [{
        "name": "devices",
        "action": "add",
        "key": "/dev/layer0",
        "value": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
    }]
}
```

**State 1**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        }
    }
}
```

**Result 2**
``` json
{
    "allowed": true,
    "metadata": [{
        "name": "matches",
        "action": "add",
        "key": "container1",
        "value": [{<container>}, {<container>}, {<container>}]
    }]
}
```

**State 2**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        },
        "matches": {
            "container1": [{<container>}, {<container>}, {<container>}]
        }
    }
}
```

**Result 3**
``` json
{
    "allowed": true,
    "metadata": [{
        "name": "matches",
        "action": "update",
        "key": "container1",
        "value": [{<container>}]
    }]
}
```

**State 3**
``` json
{
    "metadata": {
        "devices": {
            "/dev/layer0": "5c5d1ae1aff5e1f36d5300de46592efe4ccb7889e60a4b82bbaf003c2248f2a7"
        },
        "matches": {
            "container1": [{<container>}]
        }
    }
}
```

**Result 4**
``` json
{
    "allowed": true,
    "metadata": [{
        "name": "devices",
        "action": "remove",
        "key": "/dev/layer0"
    }]
}
```

**State 4**
``` json
{
    "metadata": {
        "devices": {},
        "matches": {
            "container1": [{<container>}]
        }
    }
}
```
