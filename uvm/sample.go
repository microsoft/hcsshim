package uvm

//Linux UVM JSON Document
//{
//   "Owner":"TestCode",
//   "SchemaVersion":{
//      "Major":2,
//      "Minor":0
//   },
//   "VirtualMachine":{
//      "Chipset":{
//         "UEFI":{
//            "BootThis":{
//               "uefi_device":"VMBFS",
//               "device_path":"\\bootx64.efi",
//               "disk_number":0,
//               "optional_data":"initrd=\\initrd.img"
//            }
//         }
//      },
//      "ComputeTopology":{
//         "Memory":{
//            "Startup":2048,
//            "Backing":"Virtual"
//         },
//         "Processor":{
//            "Count":1
//         }
//      },
//      "Devices":{
//         "SCSI":{
//            "primary":{
//               "Attachments":{

//               },
//               "ChannelInstanceGuid":"00000000-0000-0000-0000-000000000000"
//            }
//         },
//         "VPMem":{
//            "Devices":{

//            },
//            "MaximumCount":16
//         },
//         "GuestInterface":{
//            "ConnectToBridge":true,
//            "BridgeFlags":3
//         },
//         "VirtualSMBShares":[
//            {
//               "Name":"os",
//               "Path":"C:\\hcsintegration\\linux_BaseImageLayer\\Kernel",
//               "Flags":23,
//               "AllowedFiles":[

//               ]
//            }
//         ]
//      }
//   }
//}

//Add VPMEM to Linux UVM JSON Document (Open GCS doesn’t support the HostedSettings portion yet)
//{
//   "ResourceType":"VPMemDevice",
//   "Settings":{
//      "Devices":{
//         "0":{
//            "HostPath":"C:\\hcsintegration\\WorkingDir\\C26D095C-5BAC-4FE1-92CD-0B804ABD33EC\\029D9311-F900-4DDA-A9B0-53DEC8B1C8E5.vhd",
//            "ReadOnly":true,
//            "ImageFormat":"VHD1"
//         }
//      }
//   },
//   "HostedSettings":{
//      "MappedDevices":{
//         "0":"/tmp/ContainerLayer"
//      }
//   }
//}

//Add Container’s Mapped Virtual Disk to Linux UVM JSON Document
//{
//   "ResourceUri":"VirtualMachine/Devices/SCSI/primary/1",
//   "ResourceType":"MappedVirtualDisk",
//   "Settings":{
//      "Type":"VirtualDisk",
//      "Path":"C:\\hcsintegration\\WorkingDir\\C26D095C-5BAC-4FE1-92CD-0B804ABD33EC\\sandbox.vhdx"
//   },
//   "HostedSettings":{
//      "ContainerPath":"/tmp/ContainerScratchPath",
//      "Lun":1,
//      "CreateInUtilityVM":true
//   }
//}

//Combined Layers Request for Linux UVM JSON Document (Open GCS doesn’t support this yet)
//{
//   "ResourceType":"CombinedLayers",
//   "HostedSettings":{
//      "Layers":[
//         {
//            "Id":"029d9311-f900-4dda-a9b0-53dec8b1c8e5",
//            "Path":"/tmp/ContainerLayer"
//         }
//      ],
//      "ScratchPath":"/tmp/ContainerScratchPath",
//      "ContainerRootPath":"/tmp/ContainerSandbox"
//   }
//}

//LCOW V2 JSON Document (Open GCS doesn’t support this yet)
//{
//   "Owner":"Test hosted linux container",
//   "SchemaVersion":{
//      "Major":2,
//      "Minor":0
//   },
//   "HostingSystemId":"6010D2AC-9BF4-48F2-AF9A-1F7BDD4D857F",
//   "HostedSystem":{
//      "SchemaVersion":{
//         "Major":2,
//         "Minor":0
//      },
//      "Container":{
//         "Storage":{
//            "Layers":[
//               {
//                  "Id":"029d9311-f900-4dda-a9b0-53dec8b1c8e5",
//                  "Path":"/tmp/ContainerLayer"
//               }
//            ],
//            "Path":"/tmp/ContainerSandbox"
//         }
//      }
//   }
//}

//Remove Combined Layers Request for Linux UVM JSON Document (Open GCS doesn’t support this yet)
//{
//   "ResourceType":"CombinedLayers",
//   "RequestType":"Remove",
//   "HostedSettings":{
//      "ContainerRootPath":"/tmp/ContainerSandbox"
//   }
//}

//Remove Container’s Mapped Virtual Disk from Linux UVM JSON Document
//{
//   "ResourceUri":"VirtualMachine/Devices/SCSI/primary/1",
//   "ResourceType":"MappedVirtualDisk",
//   "RequestType":"Remove",
//   "Settings":{
//      "Type":"VirtualDisk",
//      "Path":"C:\\hcsintegration\\WorkingDir\\C26D095C-5BAC-4FE1-92CD-0B804ABD33EC\\sandbox.vhdx"
//   },
//   "HostedSettings":{
//      "ContainerPath":"/tmp/ContainerSandbox"
//   }
//}

//Remove VPMEM from Linux UVM JSON Document (Open GCS doesn’t support the HostedSettings portion yet)
//{
//   "ResourceType":"VPMemDevice",
//   "RequestType":"Remove",
//   "Settings":{
//      "Devices":{
//         "0":{
//            "HostPath":""
//         }
//      }
//   },
//   "HostedSettings":{
//      "MappedDevices":{
//         "0":"/tmp/ContainerLayer"
//      }
//   }
//}
