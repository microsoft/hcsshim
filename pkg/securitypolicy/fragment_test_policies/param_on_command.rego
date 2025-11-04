package fragment

svn := "1"
framework_version := "0.5.0"

parameters := {
  "command_param": {
    "default": [
      "init"
    ]
  }
}

containers := [
  {
    @@CONTAINER_COMMON@@
    "command": parameter("command_param"),
    "env_rules": [
      {
        "name": "MY_ENV",
        "name_strategy": "string",
        "value": "1",
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
