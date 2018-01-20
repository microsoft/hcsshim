package gcs

import (
	"fmt"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/mockos"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime/mockruntime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("GCS", func() {
	var (
		err error
	)
	AssertNoError := func() {
		It("should not produce an error", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	}
	AssertError := func() {
		It("should produce an error", func() {
			Expect(err).To(HaveOccurred())
		})
	}
	Describe("unittests", func() {
		Describe("calling processParametersToOCI", func() {
			var (
				params  prot.ProcessParameters
				process oci.Process
			)
			JustBeforeEach(func() {
				process, err = processParametersToOCI(params)
			})
			Context("params are zeroed", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{}
				})
				AssertNoError()
				It("should output an oci.Process with non-defaulted fields zeroed", func() {
					Expect(process).To(Equal(oci.Process{
						Args: []string{},
						Env:  []string{},
						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
			Context("params are set to values", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{
						CommandArgs:      []string{"sh", "-c", "sleep", "20"},
						WorkingDirectory: "/home/user/work",
						Environment: map[string]string{
							"PATH": "/this/is/my/path",
						},
						EmulateConsole:   true,
						CreateStdInPipe:  true,
						CreateStdOutPipe: true,
						CreateStdErrPipe: true,
						IsExternal:       true,
					}
				})
				AssertNoError()
				It("should output an oci.Process which matches the input values", func() {
					Expect(process).To(Equal(oci.Process{
						Args:     []string{"sh", "-c", "sleep", "20"},
						Cwd:      "/home/user/work",
						Env:      []string{"PATH=/this/is/my/path"},
						Terminal: true,

						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
			Context("CommandLine is used rather than CommandArgs", func() {
				BeforeEach(func() {
					params = prot.ProcessParameters{
						CommandLine:      "sh -c sleep 20",
						WorkingDirectory: "/home/user/work",
						Environment: map[string]string{
							"PATH": "/this/is/my/path",
						},
						EmulateConsole:   true,
						CreateStdInPipe:  true,
						CreateStdOutPipe: true,
						CreateStdErrPipe: true,
						IsExternal:       true,
					}
				})
				AssertNoError()
				It("should output an oci.Process which matches the input values", func() {
					Expect(process).To(Equal(oci.Process{
						Args:     []string{"sh", "-c", "sleep", "20"},
						Cwd:      "/home/user/work",
						Env:      []string{"PATH=/this/is/my/path"},
						Terminal: true,

						User: oci.User{UID: 0, GID: 0},
						Capabilities: &oci.LinuxCapabilities{
							Bounding: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Effective: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Inheritable: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Permitted: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
							Ambient: []string{
								"CAP_AUDIT_WRITE",
								"CAP_KILL",
								"CAP_NET_BIND_SERVICE",
								"CAP_SYS_ADMIN",
								"CAP_NET_ADMIN",
								"CAP_SETGID",
								"CAP_SETUID",
								"CAP_CHOWN",
								"CAP_FOWNER",
								"CAP_DAC_OVERRIDE",
								"CAP_NET_RAW",
							},
						},
						Rlimits: []oci.POSIXRlimit{
							oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
						},
						NoNewPrivileges: true,
					}))
				})
			})
		})

		Describe("calling processParamCommandLineToOCIArgs", func() {
			var (
				commandLine string
				args        []string
			)
			JustBeforeEach(func() {
				args, err = processParamCommandLineToOCIArgs(commandLine)
			})
			Context("commandLine is empty", func() {
				BeforeEach(func() {
					commandLine = ""
				})
				AssertNoError()
				It("should produce an empty slice", func() {
					Expect(args).To(BeEmpty())
				})
			})
			Context("commandLine has one argument", func() {
				BeforeEach(func() {
					commandLine = "sh"
				})
				AssertNoError()
				It("should produce a slice with just that argument", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has two arguments", func() {
				BeforeEach(func() {
					commandLine = "sleep 100"
				})
				AssertNoError()
				It("should produce a slice with both arguments", func() {
					Expect(args).To(Equal([]string{"sleep", "100"}))
				})
			})
			Context("commandLine has many arguments", func() {
				BeforeEach(func() {
					commandLine = "sh -c cat /bin/ls と℅Eṁに"
				})
				AssertNoError()
				It("should produce a slice with all the arguments", func() {
					Expect(args).To(Equal([]string{"sh", "-c", "cat", "/bin/ls", "と℅Eṁに"}))
				})
			})
			for _, quoteType := range []string{"\"", "'"} {
				Context(fmt.Sprintf("using quote type %s", quoteType), func() {
					Context("commandLine has a single quoted string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%ssh%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a slice without the quotes", func() {
							Expect(args).To(Equal([]string{"sh"}))
						})
					})
					Context("commandLine has a single quoted string as second argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %sa%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a slice without the quotes", func() {
							Expect(args).To(Equal([]string{"sh", "a"}))
						})
					})
					Context("commandLine has a single quoted string with spaces in it", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%sa b c%s", quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a one-element slice without the quotes", func() {
							Expect(args).To(Equal([]string{"a b c"}))
						})
					})
					Context("commandLine has multiple quoted strings with spaces in them", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%sa b c%s %sd e%s", quoteType, quoteType, quoteType, quoteType)
						})
						AssertNoError()
						It("should produce a multi-element slice without the quotes", func() {
							Expect(args).To(Equal([]string{"a b c", "d e"}))
						})
					})
					Context("commandLine has only spaces in quoted initial argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s  %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should remove the argument", func() {
							Expect(args).To(BeEmpty())
						})
					})
					Context("commandLine has quoted arguments with only spaces as the non-initial argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s   %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should remove the argument", func() {
							Expect(args).To(Equal([]string{"sh"}))
						})
					})
					Context("commandLine has spaces and non-spaces in a quoted argument", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s sh  %s", quoteType, quoteType)
						})
						AssertNoError()
						It("should preserve the spaces and the argument", func() {
							Expect(args).To(Equal([]string{" sh  "}))
						})
					})
					Context("commandLine has unclosed quotes at the beginning of string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("%s sh", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes at the end of string", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes with characters after them", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s  o ", quoteType)
						})
						AssertError()
					})
					Context("commandLine has unclosed quotes along-side closed quotes", func() {
						BeforeEach(func() {
							commandLine = fmt.Sprintf("sh %s -c %s %s o ", quoteType, quoteType, quoteType)
						})
						AssertError()
					})
				})
			}
			Context("commandLine has multiple quoted strings with escaped quotes in them", func() {
				BeforeEach(func() {
					commandLine = "\"a b \\\" c\" \"d e \\\"\""
				})
				AssertNoError()
				It("should produce a multi-element slice with only the escaped quotes", func() {
					Expect(args).To(Equal([]string{"a b \" c", "d e \""}))
				})
			})
			Context("commandLine has spaces at the edges", func() {
				BeforeEach(func() {
					commandLine = " sh "
				})
				AssertNoError()
				It("should produce a one-element slice without the spaces", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has multiple spaces at the edges", func() {
				BeforeEach(func() {
					commandLine = "  sh     "
				})
				AssertNoError()
				It("should produce a one-element slice without the spaces", func() {
					Expect(args).To(Equal([]string{"sh"}))
				})
			})
			Context("commandLine has multiple spaces in between arguments", func() {
				BeforeEach(func() {
					commandLine = "   sh   -c  ls  "
				})
				AssertNoError()
				It("should produce a multi-element slice without extra spaces", func() {
					Expect(args).To(Equal([]string{"sh", "-c", "ls"}))
				})
			})
			Context("commandLine has a combination of previous test contexts", func() {
				BeforeEach(func() {
					commandLine = " \"sh  \" -c \"  ls    \\\"/bin\\\" \"   "
				})
				AssertNoError()
				It("should parse the string correctly", func() {
					Expect(args).To(Equal([]string{"sh  ", "-c", "  ls    \"/bin\" "}))
				})
			})
		})

		Describe("calling processParamEnvToOCIEnv", func() {
			var (
				environment     map[string]string
				environmentList []string
			)
			JustBeforeEach(func() {
				environmentList = processParamEnvToOCIEnv(environment)
			})
			Context("environment is empty", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
				})
				It("should produce an empty list", func() {
					Expect(environmentList).To(BeEmpty())
				})
			})
			Context("environment has one element", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
				})
				It("should produce a list containing that element", func() {
					Expect(environmentList).To(Equal([]string{"TEST=this is a test variable!"}))
				})
			})
			Context("environment has two elements", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
					environment["PATH"] = "/this/is/a/test/path"
				})
				It("should produce a list containing both elements", func() {
					Expect(environmentList).To(ConsistOf([]string{
						"TEST=this is a test variable!",
						"PATH=/this/is/a/test/path",
					}))
				})
			})
			Context("environment has many elements", func() {
				BeforeEach(func() {
					environment = make(map[string]string)
					environment["TEST"] = "this is a test variable!"
					environment["PATH"] = "/this/is/a/test/path"
					environment["HELLO"] = "world"
					environment["VaR"] = "variable"
					environment["¥¢£"] = "ピめと"
				})
				It("should produce a list containing all the elements", func() {
					Expect(environmentList).To(ConsistOf([]string{
						"TEST=this is a test variable!",
						"PATH=/this/is/a/test/path",
						"HELLO=world",
						"VaR=variable",
						"¥¢£=ピめと",
					}))
				})
			})
		})

		Describe("calling into the primary GCS functions", func() {
			var (
				coreint                              *gcsCore
				containerID                          string
				processID                            int
				createSettings                       prot.VMHostedContainerSettings
				createSettingsCreateInUtilityVMFalse prot.VMHostedContainerSettings
				initialExecParams                    prot.ProcessParameters
				nonInitialExecParams                 prot.ProcessParameters
				externalParams                       prot.ProcessParameters
				fullStdioSet                         *stdio.ConnectionSet
				mappedVirtualDisk                    prot.MappedVirtualDisk
				mappedDirectory                      prot.MappedDirectory
				diskModificationRequest              prot.ResourceModificationRequestResponse
				diskModificationRequestSameLun       prot.ResourceModificationRequestResponse
				diskModificationRequestRemove        prot.ResourceModificationRequestResponse
				dirModificationRequest               prot.ResourceModificationRequestResponse
				dirModificationRequestSamePort       prot.ResourceModificationRequestResponse
				dirModificationRequestRemove         prot.ResourceModificationRequestResponse
				err                                  error
			)
			BeforeEach(func() {
				rtime := mockruntime.NewRuntime("/tmp/gcs")
				os := mockos.NewOS()
				cint := NewGCSCore("/tmp/gcs", "/tmp", rtime, os, &transport.MockTransport{})
				coreint = cint.(*gcsCore)
				containerID = "01234567-89ab-cdef-0123-456789abcdef"
				processID = 101
				createSettings = prot.VMHostedContainerSettings{
					Layers:          []prot.Layer{prot.Layer{Path: "0"}, prot.Layer{Path: "1"}, prot.Layer{Path: "2"}},
					SandboxDataPath: "3",
					MappedVirtualDisks: []prot.MappedVirtualDisk{
						prot.MappedVirtualDisk{
							ContainerPath:     "/path/inside/container",
							Lun:               4,
							CreateInUtilityVM: true,
							ReadOnly:          false,
						},
					},
					NetworkAdapters: []prot.NetworkAdapter{
						prot.NetworkAdapter{
							AdapterInstanceID:  "00000000-0000-0000-0000-000000000000",
							FirewallEnabled:    false,
							NatEnabled:         true,
							AllocatedIPAddress: "192.168.0.0",
							HostIPAddress:      "192.168.0.1",
							HostIPPrefixLength: 16,
							HostDNSServerList:  "0.0.0.0 1.1.1.1 8.8.8.8",
							HostDNSSuffix:      "microsoft.com",
							EnableLowMetric:    true,
						},
					},
				}
				createSettingsCreateInUtilityVMFalse = prot.VMHostedContainerSettings{
					Layers:          []prot.Layer{prot.Layer{Path: "0"}, prot.Layer{Path: "1"}, prot.Layer{Path: "2"}},
					SandboxDataPath: "3",
					MappedVirtualDisks: []prot.MappedVirtualDisk{
						prot.MappedVirtualDisk{
							ContainerPath:     "/path/inside/container",
							Lun:               4,
							CreateInUtilityVM: false,
							ReadOnly:          false,
						},
					},
					NetworkAdapters: []prot.NetworkAdapter{
						prot.NetworkAdapter{
							AdapterInstanceID:  "00000000-0000-0000-0000-000000000000",
							FirewallEnabled:    false,
							NatEnabled:         true,
							AllocatedIPAddress: "192.168.0.0",
							HostIPAddress:      "192.168.0.1",
							HostIPPrefixLength: 16,
							HostDNSServerList:  "0.0.0.0 1.1.1.1 8.8.8.8",
							HostDNSSuffix:      "microsoft.com",
							EnableLowMetric:    true,
						},
					},
				}
				initialExecParams = prot.ProcessParameters{
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       false,
					OCISpecification: oci.Spec{},
				}
				nonInitialExecParams = prot.ProcessParameters{
					CommandLine:      "cat file",
					WorkingDirectory: "/",
					Environment:      map[string]string{"PATH": "/usr/bin:/usr/sbin"},
					EmulateConsole:   true,
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       false,
				}
				externalParams = prot.ProcessParameters{
					CommandLine:      "cat file",
					WorkingDirectory: "/",
					Environment:      map[string]string{"PATH": "/usr/bin:/usr/sbin"},
					EmulateConsole:   true,
					CreateStdInPipe:  true,
					CreateStdOutPipe: true,
					CreateStdErrPipe: true,
					IsExternal:       true,
					OCISpecification: oci.Spec{},
				}
				fullStdioSet = &stdio.ConnectionSet{
					In:  mockos.NewMockReadWriteCloser(),
					Out: mockos.NewMockReadWriteCloser(),
					Err: mockos.NewMockReadWriteCloser(),
				}

				mappedVirtualDisk = prot.MappedVirtualDisk{
					ContainerPath:     "/path/inside/container",
					Lun:               5,
					CreateInUtilityVM: true,
					ReadOnly:          false,
				}
				mappedDirectory = prot.MappedDirectory{
					ContainerPath:     "abcdefghijklmnopqrstuvwxyz",
					CreateInUtilityVM: true,
					ReadOnly:          false,
					Port:              5,
				}

				diskModificationRequest = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedVirtualDisk,
					RequestType:  prot.RtAdd,
					Settings:     &mappedVirtualDisk,
				}
				diskSameLun := prot.MappedVirtualDisk{
					ContainerPath:     "/path/inside/container",
					Lun:               4,
					CreateInUtilityVM: true,
					ReadOnly:          false,
				}
				diskModificationRequestSameLun = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedVirtualDisk,
					RequestType:  prot.RtAdd,
					Settings:     &diskSameLun,
				}
				diskModificationRequestRemove = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedVirtualDisk,
					RequestType:  prot.RtRemove,
					Settings:     &mappedVirtualDisk,
				}
				dirModificationRequest = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedDirectory,
					RequestType:  prot.RtAdd,
					Settings:     &mappedDirectory,
				}
				dirSamePort := prot.MappedDirectory{
					ContainerPath:     "abcdefghijklmnopqrstuvwxyz",
					CreateInUtilityVM: true,
					ReadOnly:          false,
					Port:              4,
				}
				dirModificationRequestSamePort = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedDirectory,
					RequestType:  prot.RtAdd,
					Settings:     &dirSamePort,
				}
				dirModificationRequestRemove = prot.ResourceModificationRequestResponse{
					ResourceType: prot.PtMappedDirectory,
					RequestType:  prot.RtRemove,
					Settings:     &mappedDirectory,
				}
			})
			Describe("calling CreateContainer", func() {
				Context("mapped virtual disk is created in the utility VM", func() {
					JustBeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettings)
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
				Context("mapped virtual disk is created in the container namespace", func() {
					JustBeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettingsCreateInUtilityVMFalse)
					})
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling ExecProcess", func() {
				var (
					params prot.ProcessParameters
					pid    int
				)
				JustBeforeEach(func() {
					pid, err = coreint.ExecProcess(containerID, params, fullStdioSet)
				})
				Context("it is the initial process", func() {
					BeforeEach(func() {
						params = initialExecParams
					})
					Context("the container has already been created", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
				Context("it is not the initial process", func() {
					BeforeEach(func() {
						params = nonInitialExecParams
					})
					Context("the container has already been created", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
						})
						Context("the container already has an initial process in it", func() {
							BeforeEach(func() {
								pid, err = coreint.ExecProcess(containerID, initialExecParams, fullStdioSet)
								Expect(err).NotTo(HaveOccurred())
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
						})
						Context("the container does not already have an initial process in it", func() {
							It("should produce an error", func() {
								// TODO: Find a way to produce an error in this
								// context, possibly.
								//Expect(err).To(HaveOccurred())
							})
						})
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})
			Describe("calling SignalContainer", func() {
				Context("using signal SIGKILL", func() {
					JustBeforeEach(func() {
						err = coreint.SignalContainer(containerID, oslayer.SIGKILL)
					})
					Context("the container has already been created", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
				Context("using signal SIGTERM", func() {
					JustBeforeEach(func() {
						err = coreint.SignalContainer(containerID, oslayer.SIGTERM)
					})
					Context("the container has already been created", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
					})
					Context("the container has not already been created", func() {
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})
			Describe("calling SignalProcess", func() {
				var (
					sigkillOptions prot.SignalProcessOptions
				)
				BeforeEach(func() {
					sigkillOptions = prot.SignalProcessOptions{Signal: int32(syscall.SIGKILL)}
				})
				JustBeforeEach(func() {
					err = coreint.SignalProcess(processID, sigkillOptions)
				})
				Context("the process has already been created", func() {
					BeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettings)
						Expect(err).NotTo(HaveOccurred())
						_, err = coreint.ExecProcess(containerID, initialExecParams, fullStdioSet)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
				Context("the external process has already been created", func() {
					BeforeEach(func() {
						_, err = coreint.RunExternalProcess(externalParams, fullStdioSet)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
				Context("the process has not already been created", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling GetProperties", func() {
				var (
					properties *prot.Properties
					query      string
				)
				JustBeforeEach(func() {
					properties, err = coreint.GetProperties(containerID, query)
				})
				Context("the container has already been created", func() {
					BeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettings)
						Expect(err).NotTo(HaveOccurred())
					})
					Context("a process has been executed", func() {
						BeforeEach(func() {
							_, err = coreint.ExecProcess(containerID, initialExecParams, fullStdioSet)
							Expect(err).NotTo(HaveOccurred())
						})
						Context("using an empty query", func() {
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
							It("should not return any processes", func() {
								Expect(properties).NotTo(BeNil())
								Expect(properties.ProcessList).To(BeEmpty())
							})
						})
						Context("using a process list query", func() {
							BeforeEach(func() {
								query = "{\"PropertyTypes\":[\"ProcessList\"]}"
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
							It("should return a process with pid 123", func() {
								Expect(properties).NotTo(BeNil())
								Expect(properties.ProcessList).To(HaveLen(1))
								Expect(properties.ProcessList[0].ProcessID).To(Equal(uint32(123)))
							})
						})
						Context("using an invalid JSON query", func() {
							BeforeEach(func() {
								query = "{"
							})
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
					Context("no process has been executed", func() {
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should return a nil properties", func() {
							Expect(properties).To(BeNil())
						})
					})
				})
				Context("the container has not already been created", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})
			Describe("calling RunExternalProcess", func() {
				var (
					pid int
				)
				JustBeforeEach(func() {
					pid, err = coreint.RunExternalProcess(externalParams, fullStdioSet)
				})
				It("should not produce an error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
			Describe("calling ModifySettings", func() {
				Context("adding a mapped virtual disk", func() {
					Context("the lun is already in use", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
							err = coreint.ModifySettings(containerID, diskModificationRequestSameLun)
						})
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
					Context("the lun is not already in use", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, diskModificationRequest)
						})
						Context("the container has already been created", func() {
							BeforeEach(func() {
								err = coreint.CreateContainer(containerID, createSettings)
								Expect(err).NotTo(HaveOccurred())
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("removing a mapped virtual disk", func() {
					Context("the disk has not been added", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
							err = coreint.ModifySettings(containerID, diskModificationRequestRemove)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
					})
					Context("the disk has been added", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, diskModificationRequestRemove)
						})
						Context("the container has already been created", func() {
							BeforeEach(func() {
								err = coreint.CreateContainer(containerID, createSettings)
								Expect(err).NotTo(HaveOccurred())
								coreint.containerCache[containerID].AddMappedVirtualDisk(mappedVirtualDisk)
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("adding a mapped directory", func() {
					Context("the port is already in use", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
							err = coreint.ModifySettings(containerID, dirModificationRequestSamePort)
							Expect(err).NotTo(HaveOccurred())
							err = coreint.ModifySettings(containerID, dirModificationRequestSamePort)
						})
						It("should produce an error", func() {
							Expect(err).To(HaveOccurred())
						})
					})
					Context("the port is not already in use", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, dirModificationRequest)
						})
						Context("the container has already been created", func() {
							BeforeEach(func() {
								err = coreint.CreateContainer(containerID, createSettings)
								Expect(err).NotTo(HaveOccurred())
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
				Context("removing a mapped directory", func() {
					Context("the directory has not been added", func() {
						BeforeEach(func() {
							err = coreint.CreateContainer(containerID, createSettings)
							Expect(err).NotTo(HaveOccurred())
							err = coreint.ModifySettings(containerID, dirModificationRequestRemove)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
					})
					Context("the directory has been added", func() {
						JustBeforeEach(func() {
							err = coreint.ModifySettings(containerID, dirModificationRequestRemove)
						})
						Context("the container has already been created", func() {
							BeforeEach(func() {
								err = coreint.CreateContainer(containerID, createSettings)
								Expect(err).NotTo(HaveOccurred())
								coreint.containerCache[containerID].AddMappedDirectory(mappedDirectory)
							})
							It("should not produce an error", func() {
								Expect(err).NotTo(HaveOccurred())
							})
						})
						Context("the container has not already been created", func() {
							It("should produce an error", func() {
								Expect(err).To(HaveOccurred())
							})
						})
					})
				})
			})
			Describe("calling wait container", func() {
				var (
					exitCode int = -1
				)
				JustBeforeEach(func() {
					var exitCodeFn func() int
					exitCodeFn, err = coreint.WaitContainer(containerID)
					if err == nil {
						exitCode = exitCodeFn()
					}
				})
				Context("container does not exist", func() {
					It("should produce errors", func() {
						Expect(err).To(HaveOccurred())
					})
				})
				Context("container does exist", func() {
					JustBeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettings)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
			Describe("calling wait process", func() {
				var (
					pid      int
					exitCode int
				)
				JustBeforeEach(func() {
					var exitCodeChan chan int
					var doneChan chan bool
					exitCodeChan, doneChan, err = coreint.WaitProcess(pid)
					if err == nil {
						exitCode = <-exitCodeChan
						doneChan <- true
					}
				})
				Context("process does not exist", func() {
					It("should produce an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
				Context("process does exist", func() {
					JustBeforeEach(func() {
						pid, err = coreint.RunExternalProcess(externalParams, fullStdioSet)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
				Context("is a container process", func() {
					JustBeforeEach(func() {
						err = coreint.CreateContainer(containerID, createSettings)
						Expect(err).NotTo(HaveOccurred())
						pid, err = coreint.ExecProcess(containerID, initialExecParams, fullStdioSet)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})
	})
})
