package fragment

svn := "1"
framework_version := "0.5.0"

parameters := {
  "l1_param": {
    "default": "l1param_default"
  }
}

fragments := [
  {
    "feed": "nested_fragment",
    "includes": [
      "containers",
      "fragments"
    ],
    "issuer": "nested:issuer",
    "minimum_svn": "1",
    "parameters": {
      "l1_param": parameter("l1_param"),
      "l2_param": "l2param_from_l1_1"
    }
  },
  {
    "feed": "nested_fragment",
    "includes": [
      "containers",
      "fragments"
    ],
    "issuer": "nested:issuer",
    "minimum_svn": "1",
    "parameters": {
      "l1_param": parameter("l1_param"),
      "l2_param": "l2param_from_l1_2"
    }
  }
]

containers := []
