package fragment

svn := "1"
framework_version := "0.3.0"

containers := [
    {
        "command": ["rustc","--version"],
        "env_rules": [{"pattern": `PATH=/usr/local/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `RUSTUP_HOME=/usr/local/rustup`, "strategy": "string", "required": true},{"pattern": `CARGO_HOME=/usr/local/cargo`, "strategy": "string", "required": true},{"pattern": `RUST_VERSION=1.52.1`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false}],
        "layers": ["fe84c9d5bfddd07a2624d00333cf13c1a9c941f3a261f13ead44fc6a93bc0e7a","4dedae42847c704da891a28c25d32201a1ae440bce2aecccfa8e6f03b97a6a6c","41d64cdeb347bf236b4c13b7403b633ff11f1cf94dbc7cf881a44d6da88c5156","eb36921e1f82af46dfe248ef8f1b3afb6a5230a64181d960d10237a08cd73c79","e769d7487cc314d3ee748a4440805317c19262c7acd2fdbdb0d47d2e4613a15c","1b80f120dbd88e4355d6241b519c3e25290215c469516b49dece9cf07175a766"],
        "mounts": [],
        "exec_processes": [{"command": ["bash"], "signals": []}],
        "signals": [],
        "user": {
            "user_idname": {"pattern": ``, "strategy": "any"},
            "group_idnames": [{"pattern": ``, "strategy": "any"}],
            "umask": "0022"
        },
        "capabilities": null,
        "seccomp_profile_sha256": "",
        "allow_elevated": false,
        "working_dir": "/home/fragment",
        "allow_stdio_access": false,
        "no_new_privileges": true,
    },
]
