package fragment

svn := "1"
framework_version := "0.5.0"

parameters_api := {
  "env_param": {
    "default": {
      "value": ".+",
      "value_strategy": "re2"
    }
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
        "name": "ENV_VAR_PARAMETER",
        "name_strategy": "string",
        "value": parameter("env_param").value,
        "value_strategy": parameter("env_param").value_strategy
      }
    ],
    "exec_processes": [],
    "layers": [
      "0000000000000000000000000000000000000000000000000000000000000000"
    ],
    "mounts": []
  }
]
