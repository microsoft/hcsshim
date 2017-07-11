package runc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// globals for the whole test suite
var containerIds = []string{"a",
	"b",
	"aaaab",
	"abcdef"}
var invalidContainerIds = []string{"\"",
	"~`!@#$%^&*()[{]}',<.>/?=+\\|;:-_",
	"~`!@#$%^&*()[{]}'\",<.>/?=+\\|;:-_"}
var allContainerIds = append(containerIds, invalidContainerIds...)

var runcStateDir = "/var/run/runc"

func getBundlePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, "testbundle"), nil
}

func cleanupContainers(rtime *runcRuntime, containers []runtime.Container) error {
	var errToReturn error
	if err := attemptKillAndDeleteAllContainers(containers); err != nil {
		io.WriteString(GinkgoWriter, err.Error())
		if errToReturn == nil {
			errToReturn = err
		}
	}

	// now hard cleanup the files just in case
	if err := cleanupContainerFiles(); err != nil {
		io.WriteString(GinkgoWriter, err.Error())
		if errToReturn == nil {
			errToReturn = err
		}
	}
	if err := cleanupRuncState(); err != nil {
		io.WriteString(GinkgoWriter, err.Error())
		if errToReturn == nil {
			errToReturn = err
		}
	}

	return errToReturn
}

func attemptKillAndDeleteAllContainers(containers []runtime.Container) error {
	var errToReturn error
	for _, c := range containers {
		if state, err := c.GetState(); err == nil {
			status := state.Status
			if status == "paused" {
				if err := c.Resume(); err != nil {
					io.WriteString(GinkgoWriter, err.Error())
					if errToReturn == nil {
						errToReturn = err
					}
				}
				status = "running"
			}
			if status == "running" {
				if err := c.Kill(oslayer.SIGKILL); err != nil {
					io.WriteString(GinkgoWriter, err.Error())
					if errToReturn == nil {
						errToReturn = err
					}
				}
				if _, err := c.Wait(); err != nil {
					io.WriteString(GinkgoWriter, err.Error())
					if errToReturn == nil {
						errToReturn = err
					}
				}
			} else if status == "created" {
				go func() {
					if _, err := c.Wait(); err != nil {
						io.WriteString(GinkgoWriter, err.Error())
					}
				}()
			}
			if err := c.Delete(); err != nil {
				io.WriteString(GinkgoWriter, err.Error())
				if errToReturn == nil {
					errToReturn = err
				}
			}
		}
	}

	containers = nil
	return errToReturn
}

func cleanupContainerFiles() error {
	return removeSubdirs(containerFilesDir)
}

func cleanupRuncState() error {
	return removeSubdirs(runcStateDir)
}

func removeSubdirs(parentDir string) error {
	dir, err := os.Open(parentDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	contents, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	var errToReturn error
	for _, item := range contents {
		itemPath := filepath.Join(parentDir, item)
		info, err := os.Stat(itemPath)
		if err != nil {
			io.WriteString(GinkgoWriter, err.Error())
			if errToReturn == nil {
				errToReturn = err
			}
		}
		if info.IsDir() {
			if err := os.RemoveAll(itemPath); err != nil {
				io.WriteString(GinkgoWriter, err.Error())
				if errToReturn == nil {
					errToReturn = err
				}
			}
		}
	}
	return errToReturn
}

var _ = Describe("runC", func() {
	var (
		rtime      *runcRuntime
		bundle     string
		err        error
		containers []runtime.Container

		createAllStdioOptions runtime.StdioOptions
	)

	BeforeEach(func() {
		rtime, err = NewRuntime()
		Expect(err).NotTo(HaveOccurred())
		bundle, err = getBundlePath()
		Expect(err).NotTo(HaveOccurred())

		createAllStdioOptions = runtime.StdioOptions{
			CreateIn:  true,
			CreateOut: true,
			CreateErr: true,
		}
	})
	AfterEach(func() {
		err = cleanupContainers(rtime, containers)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("creating a container", func() {
		var (
			id string
			c  runtime.Container
		)
		JustBeforeEach(func() {
			c, err = rtime.CreateContainer(id, bundle, createAllStdioOptions)
			if err == nil {
				containers = append(containers, c)
			}
		})
		Context("using a valid ID", func() {
			for _, _id := range containerIds {
				Context(fmt.Sprintf("using ID %s", _id), func() {
					BeforeEach(func() { id = _id })
					It("should not have produced an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
					It("should put the container in the \"created\" state", func() {
						container, err := c.GetState()
						Expect(err).NotTo(HaveOccurred())
						Expect(container.Status).To(Equal("created"))
					})
				})
			}
		})
		Context("using an invalid ID", func() {
			for _, _id := range invalidContainerIds {
				Context(fmt.Sprintf("using ID %s", _id), func() {
					BeforeEach(func() { id = _id })
					It("should have produced an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			}
		})
	})

	for _, id := range containerIds {
		Context(fmt.Sprintf("using ID %s", id), func() {
			Describe("performing post-Create operations", func() {
				var (
					c runtime.Container
				)
				JustBeforeEach(func() {
					c, err = rtime.CreateContainer(id, bundle, createAllStdioOptions)
					if err == nil {
						containers = append(containers, c)
					}
					Expect(err).NotTo(HaveOccurred())
				})

				Describe("starting a container", func() {
					JustBeforeEach(func() {
						err = c.Start()
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
					It("should put the container in the \"running\" state", func() {
						container, err := c.GetState()
						Expect(err).NotTo(HaveOccurred())
						Expect(container.Status).To(Equal("running"))
					})
				})

				Describe("performing post-Start operations", func() {
					var (
						longSleepProcess  oci.Process
						shortSleepProcess oci.Process
					)
					BeforeEach(func() {
						longSleepProcess = oci.Process{
							Terminal: false,
							Cwd:      "/",
							Args:     []string{"sleep", "100"},
							Env:      []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
						}
						shortSleepProcess = oci.Process{
							Terminal: false,
							Cwd:      "/",
							Args:     []string{"sleep", "0.1"},
							Env:      []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
						}
					})
					JustBeforeEach(func() {
						err = c.Start()
						Expect(err).NotTo(HaveOccurred())
					})

					Describe("executing a process in a container", func() {
						JustBeforeEach(func() {
							_, err = c.ExecProcess(longSleepProcess, createAllStdioOptions)
						})
						It("should not have produced an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should have created another process in the container", func() {
							processes, err := c.GetRunningProcesses()
							Expect(err).NotTo(HaveOccurred())
							Expect(processes).To(HaveLen(2))
						})
					})

					Describe("killing a container", func() {
						JustBeforeEach(func() {
							err = c.Kill(oslayer.SIGKILL)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"stopped\" state", func(done Done) {
							defer close(done)

							_, err = c.Wait()
							Expect(err).NotTo(HaveOccurred())
							container, err := c.GetState()
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("stopped"))
						}, 2) // Test fails if it takes longer than 2 seconds.
					})

					Describe("deleting a container", func() {
						JustBeforeEach(func(done Done) {
							defer close(done)

							err = c.Kill(oslayer.SIGKILL)
							Expect(err).NotTo(HaveOccurred())
							_, err = c.Wait()
							Expect(err).NotTo(HaveOccurred())

							err = c.Delete()
						}, 2) // Test fails if it takes longer than 2 seconds.
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should delete the container", func() {
							states, err := rtime.ListContainerStates()
							Expect(err).NotTo(HaveOccurred())
							Expect(states).To(HaveLen(0))
							_, err = c.GetState()
							Expect(err).To(HaveOccurred())
						})
					})

					Describe("deleting a process", func() {
						var (
							p runtime.Process
						)
						JustBeforeEach(func(done Done) {
							defer close(done)

							p, err = c.ExecProcess(shortSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							_, err = p.Wait()
							Expect(err).NotTo(HaveOccurred())
							err = p.Delete()
						}, 2) // Test fails if it takes longer than 2 seconds.
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should delete the process", func() {
							Expect(rtime.getProcessDir(id, p.Pid())).NotTo(BeADirectory())
						})
					})

					Describe("pausing a container", func() {
						JustBeforeEach(func() {
							err = c.Pause()
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"paused\" state", func() {
							container, err := c.GetState()
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("paused"))
						})
					})

					Describe("resuming a container", func() {
						JustBeforeEach(func() {
							err = c.Pause()
							Expect(err).NotTo(HaveOccurred())
							err = c.Resume()
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"running\" state", func() {
							container, err := c.GetState()
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("running"))
						})
					})

					Describe("getting running container processes", func() {
						var (
							p         runtime.Process
							processes []runtime.ContainerProcessState
						)
						JustBeforeEach(func(done Done) {
							defer close(done)

							_, err = c.ExecProcess(longSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							p, err = c.ExecProcess(shortSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							_, err = p.Wait()
							Expect(err).NotTo(HaveOccurred())
							processes, err = c.GetRunningProcesses()
						}, 2) // Test fails if it takes longer than 2 seconds.
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should only have 2 processes remaining running", func() {
							Expect(processes).To(HaveLen(2))
						})
					})
				})
			})
		})
	}
})
