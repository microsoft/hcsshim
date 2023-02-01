# Command Extractor

This tool extracts a list of commands as a JSON file which fully exercise a Rego policy.
The input to the tool is a TOML policy config file, like the following:

``` toml
allow_properties_access = true
allow_runtime_logging = true

[[container]]
image_name = "contoso.azurecr.io/main:latest"
command = ["python3","server.py"]
allow_elevated = true
allow_stdio_access = true

[[container.exec_process]]
command = ["/bin/bash"]
allow_stdio_access = true

[[external_process]]
command = ["/bin/bash"]
allow_stdio_access = true
```

(this is identical to the input format for the [security policy tool](../securitypolicy/))

The resulting commands can then be used by the [policy engine simulator](../policyenginesimulator/).

## Getting Started

The usage for the tool is:

    ./commandextractor -h
    Usage of commandextractor:
      -fragments string
          path to one or more fragment configuration TOML files
      -policy string
          path policy configuration TOML
    
    Example:

    ./commandextractor -policy policy.toml -fragments fragment0.toml fragment1.toml


As an example, say that you had two files: `policy.toml`, which contains the policy configuration,
and `fragment.toml`, which contains the fragment configuration. One can set up an experiment to test
a policy in the following way:

    securitypolicytool -c policy.toml -t rego -r > policy.rego
    securitypolicytool -c fragment.rego -t fragment -n demo -v 1.0.0 -r > fragment.rego
    commandextractor -policy policy.toml -fragments fragment.toml > commands.json
    policyenginesimulator -commands commands.json -policy policy.rego

The example above assumes that you want to use a framework-based policy of the kind produced
by securitypolicytool. If you had a custom policy (*e.g.* one written by hand) you would
still need to produce a `policy.toml` in the same format as above, but otherwise everything
is the same:

    commandextractor -policy custom_container_info.toml
    policyenginesimulator -commands custom_container_info.toml -policy policy.rego


> **Note**
>
> The policy engine simulator loads fragment code from local files. The commands produced
> by commandextractor will contain a placeholder which must be replaced with the actual path.
