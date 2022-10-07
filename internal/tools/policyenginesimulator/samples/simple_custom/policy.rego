package policy

api_svn := "0.7.0"

overlays := {
    "pause": {
        "deviceHashes": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": []
    },
    "python3": {
        "deviceHashes": [
            "998fe7a12356e0de0f2ffb4134615b42c9510e281c0ecfc7628c121442544309",
            "f65ec804a63b85f507ac11d187434ea135a18cdc16202551d8dff292f942fdf0",
            "04c110e9406d2b57079f1eac4c9c5247747caa3bcaab6d83651de6e7da97cb40",
            "e7fbe653352d546497c534c629269c4c04f1997f6892bd66c273f0c9753a4de3",
            "b99a9ced77c45fc4dc96bac8ea1e4d9bc1d2a66696cc057d3f3cca79dc999702",
            "3413e98a178646d4703ea70b9bff2d4410e606a22062046992cda8c8aedaa387",
            "1e66649e162d99c4d675d8d8c3af90ece3799b33d24671bc83fe9ea5143daf2f",
            "97112ba1d4a2c86c1c15a3e13f606e8fcc0fb1b49154743cadd1f065c42fee5a",
            "37e9dcf799048b7d35ce53584e0984198e1bc3366c3bb5582fd97553d31beb4e"
        ],
        "mounts": []
    },
    "resolvConf": {
        "deviceHashes": [
            "606fd6baf5eb1a71fd286aea29672a06bfe55f0007ded92ee73142a37590ed19",
            "97adfda6943f3af972b9bf4fa684f533f10c023d913d195048fef03f9c3c60fd",
            "296e5baa5b9ded863ca0170e05cd9ecf4136f86c830a9da906184ab147415c7b",
            "a6a6918c07c85e29e48d4a87c1194781251d5185f682c26f20d6ee4e955a239f",
            "285cb680a55d09f548d4baa804a663764788619824565685b32b8097cbed3d26"
        ],
        "mounts": ["/mount/resolvconf"]
    }
}

custom_containers := [
    {
        "id": "pause",
        "command": ["/pause"],
        "overlayID": "pause",
        "depends": []
    },
    {
        "id": "attestationReport",
        "command": ["python3", "WebAttestationReport.py"],
        "overlayID": "python3",
        "depends": ["pause"]
    },
    {
        "id": "copy_resolv_conf",
        "command": ["bash", "/copy_resolv_conf.sh"],
        "overlayID": "resolvConf",
        "depends": ["pause"]
    }
]

mount_device := data.custom.mount_device
mount_overlay := data.custom.mount_overlay
create_container := data.custom.create_container
unmount_device := {"allowed": true}
unmount_overlay := {"allowed": true}
exec_in_container := {"allowed": true}
exec_external := {"allowed": true}
shutdown_container := {"allowed": true}
signal_container_process := {"allowed": true}
plan9_mount := {"allowed": true}
plan9_unmount := {"allowed": true}

default load_fragment := {"allowed": false}
load_fragment := {"allowed": true, "add_module": true} {
    input.issuer == "did:web:contoso.github.io"
    input.feed == "contoso.azurecr.io/custom"
}
