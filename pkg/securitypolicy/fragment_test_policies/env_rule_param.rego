package fragment

svn := "1"
framework_version := "0.5.0"

parameters_api := {
    "env_param": {
        "default": {
            "value": "default_value",
            "value_strategy": "string"
        }
    },
    "env_param_nodefault": {
    },
    "env_string_param": {
        "default": "default_string_value"
    },
    "env_string_param_nodefault": {
    }
}

containers := [
  {
    @@CONTAINER_COMMON@@
    "command": [
      "init"
    ],
    "env_rules": [
      {
        "name": "ENV_VAR_FIXED",
        "name_strategy": "string",
        "value": "fixed_value",
        "value_strategy": "string"
      },
      {
        "name": "ENV_VAR_PARAMETER",
        "name_strategy": "string",
        "value": parameter("env_param").value,
        "value_strategy": parameter("env_param").value_strategy
      },
      {
        "name": "ENV_VAR_PARAMETER_NODEFAULT",
        "name_strategy": "string",
        "value": parameter("env_param_nodefault").value,
        "value_strategy": parameter("env_param_nodefault").value_strategy
      },
      {
        "name": "ENV_STRING_PARAM",
        "name_strategy": "string",
        "value": parameter("env_string_param"),
        "value_strategy": "string"
      },
      {
        "name": "ENV_STRING_PARAM_NODEFAULT",
        "name_strategy": "string",
        "value": parameter("env_string_param_nodefault"),
        "value_strategy": "string"
      }
    ],
    "exec_processes": [],
    "layers": [
      "0000000000000000000000000000000000000000000000000000000000000000"
    ],
    "mounts": []
  }
]
