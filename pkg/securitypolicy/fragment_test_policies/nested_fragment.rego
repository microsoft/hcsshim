package fragment

svn := "1"
framework_version := "0.5.0"

parameters := {
  "l1_param": {},
  "l2_param": {
    "default": "l2param_default"
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
        "name": "L1_PARAM",
        "name_strategy": "string",
        "value": parameter("l1_param"),
        "value_strategy": "string"
      },
      {
        "name": "L2_PARAM",
        "name_strategy": "string",
        "value": parameter("l2_param"),
        "value_strategy": "string"
      }
    ],
    "exec_processes": [],
    "layers": [
      "2222222222222222222222222222222222222222222222222222222222222222"
    ],
    "mounts": []
  }
]
