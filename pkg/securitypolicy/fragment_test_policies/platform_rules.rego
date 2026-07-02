package fragment

svn := "1"
framework_version := "0.5.0"

platform_rules := [
  {
    "env_rules": [
      {
        "name": "(?i)(FABRIC)_.+",
        "name_strategy": "re2",
        "value": ".+",
        "value_strategy": "re2"
      }
    ],
    "mounts": [
      {
        "destination": "/var/run/secrets/kubernetes.io/serviceaccount",
        "options": [
          "rbind",
          "rshared",
          "ro"
        ],
        "source": "sandbox:///tmp/atlas/emptydir/.+",
        "type": "bind"
      }
    ]
  }
]
