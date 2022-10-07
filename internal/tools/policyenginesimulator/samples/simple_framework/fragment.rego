package fragment

svn := "1.0.0"

containers := [
    {
        "command": ["python3","WebAttestationReport.py"],
        "env_rules": [
            {
                "pattern": "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                "strategy": "string",
                "required": false
            },
            {
                "pattern": "PYTHONUNBUFFERED=1",
                "strategy": "string",
                "required": false
            },
            {
                "pattern": "TERM=xterm",
                "strategy": "string",
                "required": false
            }
        ],
        "layers": [
            "37e9dcf799048b7d35ce53584e0984198e1bc3366c3bb5582fd97553d31beb4e",
            "97112ba1d4a2c86c1c15a3e13f606e8fcc0fb1b49154743cadd1f065c42fee5a",
            "1e66649e162d99c4d675d8d8c3af90ece3799b33d24671bc83fe9ea5143daf2f",
            "3413e98a178646d4703ea70b9bff2d4410e606a22062046992cda8c8aedaa387",
            "b99a9ced77c45fc4dc96bac8ea1e4d9bc1d2a66696cc057d3f3cca79dc999702",
            "e7fbe653352d546497c534c629269c4c04f1997f6892bd66c273f0c9753a4de3",
            "04c110e9406d2b57079f1eac4c9c5247747caa3bcaab6d83651de6e7da97cb40",
            "f65ec804a63b85f507ac11d187434ea135a18cdc16202551d8dff292f942fdf0",
            "998fe7a12356e0de0f2ffb4134615b42c9510e281c0ecfc7628c121442544309"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "allow_elevated": true,
        "working_dir": "/demo-attestion"
    }
]