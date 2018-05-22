// +build windows

package hcsoci

//// TODO Remove this comment

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

//// createLCOWv1 creates a Linux (LCOW) container using the V1 schema.
//func createLCOWv1(coi *createOptionsInternal) (*hcs.System, error) {

//	configuration := &ContainerConfig{
//		HvPartition:   true,
//		Name:          coi.actualId,
//		SystemType:    "container",
//		ContainerType: "linux",
//		Owner:         coi.actualOwner,
//		TerminateOnLastHandleClosed: true,
//	}
//	configuration.HvRuntime = &HvRuntime{
//		ImagePath:           coi.actualKirdPath,
//		LinuxKernelFile:     coi.actualKernelFile,
//		LinuxInitrdFile:     coi.actualInitrdFile,
//		LinuxBootParameters: coi.KernelBootOptions,
//	}

//	if coi.Spec.Windows != nil {
//		// Strip off the top-most layer as that's passed in separately to HCS
//		if len(coi.Spec.Windows.LayerFolders) > 0 {
//			configuration.LayerFolderPath = coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]
//			layerFolders := coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1]

//			for _, layerPath := range layerFolders {
//				_, filename := filepath.Split(layerPath)
//				g, err := NameToGuid(filename)
//				if err != nil {
//					return nil, err
//				}
//				configuration.Layers = append(configuration.Layers, Layer{
//					ID:   g.ToString(),
//					Path: filepath.Join(layerPath, "layer.vhd"),
//				})
//			}
//		}

//		if coi.Spec.Windows.Network != nil {
//			configuration.EndpointList = coi.Spec.Windows.Network.EndpointList
//			configuration.AllowUnqualifiedDNSQuery = coi.Spec.Windows.Network.AllowUnqualifiedDNSQuery
//			if coi.Spec.Windows.Network.DNSSearchList != nil {
//				configuration.DNSSearchList = strings.Join(coi.Spec.Windows.Network.DNSSearchList, ",")
//			}
//			configuration.NetworkSharedContainerName = coi.Spec.Windows.Network.NetworkSharedContainerName
//		}
//	}

//	// Add the mounts (volumes, bind mounts etc) to the structure. We have to do
//	// some translation for both the mapped directories passed into HCS and in
//	// the spec.
//	//
//	// For HCS, we only pass in the mounts from the spec which are type "bind".
//	// Further, the "ContainerPath" field (which is a little mis-leadingly
//	// named when it applies to the utility VM rather than the container in the
//	// utility VM) is moved to under /tmp/gcs/<ID>/binds, where this is passed
//	// by the caller through a 'uvmpath' option.
//	//
//	// We do similar translation for the mounts in the spec by stripping out
//	// the uvmpath option, and translating the Source path to the location in the
//	// utility VM calculated above.
//	//
//	// From inside the utility VM, you would see a 9p mount such as in the following
//	// where a host folder has been mapped to /target. The line with /tmp/gcs/<ID>/binds
//	// specifically:
//	//
//	//	/ # mount
//	//	rootfs on / type rootfs (rw,size=463736k,nr_inodes=115934)
//	//	proc on /proc type proc (rw,relatime)
//	//	sysfs on /sys type sysfs (rw,relatime)
//	//	udev on /dev type devtmpfs (rw,relatime,size=498100k,nr_inodes=124525,mode=755)
//	//	tmpfs on /run type tmpfs (rw,relatime)
//	//	cgroup on /sys/fs/cgroup type cgroup (rw,relatime,cpuset,cpu,cpuacct,blkio,memory,devices,freezer,net_cls,perf_event,net_prio,hugetlb,pids,rdma)
//	//	mqueue on /dev/mqueue type mqueue (rw,relatime)
//	//	devpts on /dev/pts type devpts (rw,relatime,mode=600,ptmxmode=000)
//	//	/binds/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/target on /binds/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/target type 9p (rw,sync,dirsync,relatime,trans=fd,rfdno=6,wfdno=6)
//	//	/dev/pmem0 on /tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/layer0 type ext4 (ro,relatime,block_validity,delalloc,norecovery,barrier,dax,user_xattr,acl)
//	//	/dev/sda on /tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/scratch type ext4 (rw,relatime,block_validity,delalloc,barrier,user_xattr,acl)
//	//	overlay on /tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/rootfs type overlay (rw,relatime,lowerdir=/tmp/base/:/tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/layer0,upperdir=/tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/scratch/upper,workdir=/tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc/scratch/work)
//	//
//	//  /tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc # ls -l
//	//	total 16
//	//	drwx------    3 0        0               60 Sep  7 18:54 binds
//	//	-rw-r--r--    1 0        0             3345 Sep  7 18:54 config.json
//	//	drwxr-xr-x   10 0        0             4096 Sep  6 17:26 layer0
//	//	drwxr-xr-x    1 0        0             4096 Sep  7 18:54 rootfs
//	//	drwxr-xr-x    5 0        0             4096 Sep  7 18:54 scratch
//	//
//	//	/tmp/gcs/b3ea9126d67702173647ece2744f7c11181c0150e9890fc9a431849838033edc # ls -l binds
//	//	total 0
//	//	drwxrwxrwt    2 0        0             4096 Sep  7 16:51 target

//	mds := []MappedDir{}
//	specMounts := []specs.Mount{}
//	for _, mount := range coi.Spec.Mounts {
//		specMount := mount
//		if mount.Type == "bind" {
//			// Strip out the uvmpath from the options
//			updatedOptions := []string{}
//			uvmPath := ""
//			readonly := false
//			for _, opt := range mount.Options {
//				dropOption := false
//				elements := strings.SplitN(opt, "=", 2)
//				switch elements[0] {
//				case "uvmpath":
//					uvmPath = elements[1]
//					dropOption = true
//				case "rw":
//				case "ro":
//					readonly = true
//				case "rbind":
//				default:
//					return nil, fmt.Errorf("unsupported option %q", opt)
//				}
//				if !dropOption {
//					updatedOptions = append(updatedOptions, opt)
//				}
//			}
//			mount.Options = updatedOptions
//			if uvmPath == "" {
//				return nil, fmt.Errorf("no uvmpath for bind mount %+v", mount)
//			}
//			md := MappedDir{
//				HostPath:          mount.Source,
//				ContainerPath:     path.Join(uvmPath, mount.Destination),
//				CreateInUtilityVM: true,
//				ReadOnly:          readonly,
//			}
//			mds = append(mds, md)
//			specMount.Source = path.Join(uvmPath, mount.Destination)
//		}
//		specMounts = append(specMounts, specMount)
//	}
//	configuration.MappedDirectories = mds

//	container, err := CreateContainer(coi.actualId, configuration)
//	if err != nil {
//		return nil, err
//	}

//	// TODO - Not sure why after CreateContainer, but that's how I coded it in libcontainerd and it worked....
//	coi.Spec.Mounts = specMounts

//	logrus.Debugf("createLCOWv1() completed successfully")
//	return container, nil
//}

//func debugCommand(s string) string {
//	return fmt.Sprintf(`echo -e 'DEBUG COMMAND: %s\\n--------------\\n';%s;echo -e '\\n\\n';`, s, s)
//}

// DebugLCOWGCS extracts logs from the GCS in LCOW. It's a useful hack for debugging,
// but not necessarily optimal, but all that is available to us in RS3.
//func (container *container) DebugLCOWGCS() {
//	if logrus.GetLevel() < logrus.DebugLevel || len(os.Getenv("HCSSHIM_LCOW_DEBUG_ENABLE")) == 0 {
//		return
//	}

//	var out bytes.Buffer
//	cmd := os.Getenv("HCSSHIM_LCOW_DEBUG_COMMAND")
//	if cmd == "" {
//		cmd = `sh -c "`
//		cmd += debugCommand("kill -10 `pidof gcs`") // SIGUSR1 for stackdump
//		cmd += debugCommand("ls -l /tmp")
//		cmd += debugCommand("cat /tmp/gcs.log")
//		cmd += debugCommand("cat /tmp/gcs/gcs-stacks*")
//		cmd += debugCommand("cat /tmp/gcs/paniclog*")
//		cmd += debugCommand("ls -l /tmp/gcs")
//		cmd += debugCommand("ls -l /tmp/gcs/*")
//		cmd += debugCommand("cat /tmp/gcs/*/config.json")
//		cmd += debugCommand("ls -lR /var/run/gcsrunc")
//		cmd += debugCommand("cat /tmp/gcs/global-runc.log")
//		cmd += debugCommand("cat /tmp/gcs/*/runc.log")
//		cmd += debugCommand("ps -ef")
//		cmd += `"`
//	}

//	proc, _, err := container.CreateProcessEx(
//		&CreateProcessEx{
//			OCISpecification: &specs.Spec{
//				Process: &specs.Process{Args: []string{cmd}},
//				Linux:   &specs.Linux{},
//			},
//			CreateInUtilityVm: true,
//			Stdout:            &out,
//		})
//	defer func() {
//		if proc != nil {
//			proc.Kill()
//			proc.Close()
//		}
//	}()
//	if err != nil {
//		logrus.Debugln("benign failure getting gcs logs: ", err)
//	}
//	if proc != nil {
//		proc.WaitTimeout(time.Duration(int(time.Second) * 30))
//	}
//	logrus.Debugf("GCS Debugging:\n%s\n\nEnd GCS Debugging", strings.TrimSpace(out.String()))
//}

//// TarToVhd streams a tarstream contained in an io.Reader to a fixed vhd file
//func TarToVhd(uvm Container, targetVHDFile string, reader io.Reader) (int64, error) {
//	logrus.Debugf("hcsshim: TarToVhd: %s", targetVHDFile)

//	if uvm == nil {
//		return 0, fmt.Errorf("cannot Tar2Vhd as no utility VM supplied")
//	}
//	defer uvm.DebugLCOWGCS()

//	outFile, err := os.Create(targetVHDFile)
//	if err != nil {
//		return 0, fmt.Errorf("tar2vhd failed to create %s: %s", targetVHDFile, err)
//	}
//	defer outFile.Close()
//	// BUGBUG Delete the file on failure

//	tar2vhd, byteCounts, err := uvm.CreateProcessEx(&CreateProcessEx{
//		OCISpecification: &specs.Spec{
//			Process: &specs.Process{Args: []string{"tar2vhd"}},
//			Linux:   &specs.Linux{},
//		},
//		CreateInUtilityVm: true,
//		Stdin:             reader,
//		Stdout:            outFile,
//	})
//	if err != nil {
//		return 0, fmt.Errorf("failed to start tar2vhd for %s: %s", targetVHDFile, err)
//	}
//	defer tar2vhd.Close()

//	logrus.Debugf("hcsshim: TarToVhd: %s created, %d bytes", targetVHDFile, byteCounts.Out)
//	return byteCounts.Out, err
//}

//// VhdToTar does what is says - it exports a VHD in a specified
//// folder (either a read-only layer.vhd, or a read-write sandbox.vhd) to a
//// ReadCloser containing a tar-stream of the layers contents.
//func VhdToTar(uvm Container, vhdFile string, uvmMountPath string, isSandbox bool, vhdSize int64) (io.ReadCloser, error) {
//	logrus.Debugf("hcsshim: VhdToTar: %s isSandbox: %t", vhdFile, isSandbox)

//	if config.Uvm == nil {
//		return nil, fmt.Errorf("cannot VhdToTar as no utility VM is in configuration")
//	}

//	defer uvm.DebugLCOWGCS()

//	vhdHandle, err := os.Open(vhdFile)
//	if err != nil {
//		return nil, fmt.Errorf("hcsshim: VhdToTar: failed to open %s: %s", vhdFile, err)
//	}
//	defer vhdHandle.Close()
//	logrus.Debugf("hcsshim: VhdToTar: exporting %s, size %d, isSandbox %t", vhdHandle.Name(), vhdSize, isSandbox)

//	// Different binary depending on whether a RO layer or a RW sandbox
//	command := "vhd2tar"
//	if isSandbox {
//		command = fmt.Sprintf("exportSandbox -path %s", uvmMountPath)
//	}

//	// Start the binary in the utility VM
//	proc, stdin, stdout, _, err := config.createLCOWUVMProcess(command)
//	if err != nil {
//		return nil, fmt.Errorf("hcsshim: VhdToTar: %s: failed to create utils process %s: %s", vhdHandle.Name(), command, err)
//	}

//	if !isSandbox {
//		// Send the VHD contents to the utility VM processes stdin handle if not a sandbox
//		logrus.Debugf("hcsshim: VhdToTar: copying the layer VHD into the utility VM")
//		if _, err = copyWithTimeout(stdin, vhdHandle, vhdSize, processOperationTimeoutSeconds, fmt.Sprintf("vhdtotarstream: sending %s to %s", vhdHandle.Name(), command)); err != nil {
//			proc.Close()
//			return nil, fmt.Errorf("hcsshim: VhdToTar: %s: failed to copyWithTimeout on the stdin pipe (to utility VM): %s", vhdHandle.Name(), err)
//		}
//	}

//	// Start a goroutine which copies the stdout (ie the tar stream)
//	reader, writer := io.Pipe()
//	go func() {
//		defer writer.Close()
//		defer proc.Close()
//		logrus.Debugf("hcsshim: VhdToTar: copying tar stream back from the utility VM")
//		bytes, err := copyWithTimeout(writer, stdout, vhdSize, processOperationTimeoutSeconds, fmt.Sprintf("vhdtotarstream: copy tarstream from %s", command))
//		if err != nil {
//			logrus.Errorf("hcsshim: VhdToTar: %s:  copyWithTimeout on the stdout pipe (from utility VM) failed: %s", vhdHandle.Name(), err)
//		}
//		logrus.Debugf("hcsshim: VhdToTar: copied %d bytes of the tarstream of %s from the utility VM", bytes, vhdHandle.Name())
//	}()

//	// Return the read-side of the pipe connected to the goroutine which is reading from the stdout of the process in the utility VM
//	return reader, nil
//}
