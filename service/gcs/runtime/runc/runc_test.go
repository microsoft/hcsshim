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

func cleanupContainers(rtime *runcRuntime) error {
	var errToReturn error
	if err := attemptKillAndDeleteAllContainers(rtime); err != nil {
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

func attemptKillAndDeleteAllContainers(rtime *runcRuntime) error {
	var errToReturn error
	states, err := rtime.ListContainerStates()
	if err != nil {
		return err
	}
	for _, state := range states {
		status := state.Status
		if status == "paused" {
			if err := rtime.ResumeContainer(state.ID); err != nil {
				io.WriteString(GinkgoWriter, err.Error())
				if errToReturn == nil {
					errToReturn = err
				}
			}
			status = "running"
		}
		if status == "running" {
			if err := rtime.KillContainer(state.ID, oslayer.SIGKILL); err != nil {
				io.WriteString(GinkgoWriter, err.Error())
				if errToReturn == nil {
					errToReturn = err
				}
			}
			if _, err := rtime.WaitOnContainer(state.ID); err != nil {
				io.WriteString(GinkgoWriter, err.Error())
				if errToReturn == nil {
					errToReturn = err
				}
			}
		} else if status == "created" {
			go func() {
				if _, err := rtime.WaitOnContainer(state.ID); err != nil {
					io.WriteString(GinkgoWriter, err.Error())
				}
			}()
		}
		if err := rtime.DeleteContainer(state.ID); err != nil {
			io.WriteString(GinkgoWriter, err.Error())
			if errToReturn == nil {
				errToReturn = err
			}
		}
	}

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
		rtime  *runcRuntime
		bundle string
		err    error

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
		err = cleanupContainers(rtime)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("creating a container", func() {
		var (
			id string
		)
		JustBeforeEach(func() {
			_, err = rtime.CreateContainer(id, bundle, createAllStdioOptions)
		})
		Context("using a valid ID", func() {
			for _, _id := range containerIds {
				Context(fmt.Sprintf("using ID %s", _id), func() {
					BeforeEach(func() { id = _id })
					It("should not have produced an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
					It("should put the container in the \"created\" state", func() {
						container, err := rtime.GetContainerState(id)
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
				JustBeforeEach(func() {
					_, err = rtime.CreateContainer(id, bundle, createAllStdioOptions)
					Expect(err).NotTo(HaveOccurred())
				})

				Describe("starting a container", func() {
					JustBeforeEach(func() {
						err = rtime.StartContainer(id)
					})
					It("should not produce an error", func() {
						Expect(err).NotTo(HaveOccurred())
					})
					It("should put the container in the \"running\" state", func() {
						container, err := rtime.GetContainerState(id)
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
						err = rtime.StartContainer(id)
						Expect(err).NotTo(HaveOccurred())
					})

					Describe("executing a process in a container", func() {
						JustBeforeEach(func() {
							_, err = rtime.ExecProcess(id, longSleepProcess, createAllStdioOptions)
						})
						It("should not have produced an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should have created another process in the container", func() {
							processes, err := rtime.GetRunningContainerProcesses(id)
							Expect(err).NotTo(HaveOccurred())
							Expect(processes).To(HaveLen(2))
						})
					})

					Describe("killing a container", func() {
						JustBeforeEach(func() {
							err = rtime.KillContainer(id, oslayer.SIGKILL)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"stopped\" state", func(done Done) {
							defer close(done)

							_, err = rtime.WaitOnContainer(id)
							Expect(err).NotTo(HaveOccurred())
							container, err := rtime.GetContainerState(id)
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("stopped"))
						}, 2) // Test fails if it takes longer than 2 seconds.
					})

					Describe("deleting a container", func() {
						JustBeforeEach(func(done Done) {
							defer close(done)

							err = rtime.KillContainer(id, oslayer.SIGKILL)
							Expect(err).NotTo(HaveOccurred())
							_, err = rtime.WaitOnContainer(id)
							Expect(err).NotTo(HaveOccurred())

							err = rtime.DeleteContainer(id)
						}, 2) // Test fails if it takes longer than 2 seconds.
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should delete the container", func() {
							states, err := rtime.ListContainerStates()
							Expect(err).NotTo(HaveOccurred())
							Expect(states).To(HaveLen(0))
							_, err = rtime.GetContainerState(id)
							Expect(err).To(HaveOccurred())
						})
					})

					Describe("deleting a process", func() {
						var (
							pid int
						)
						JustBeforeEach(func(done Done) {
							defer close(done)

							pid, err = rtime.ExecProcess(id, shortSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							_, err = rtime.WaitOnProcess(id, pid)
							Expect(err).NotTo(HaveOccurred())
							err = rtime.DeleteProcess(id, pid)
						}, 2) // Test fails if it takes longer than 2 seconds.
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should delete the process", func() {
							Expect(rtime.getProcessDir(id, pid)).NotTo(BeADirectory())
						})
					})

					Describe("pausing a container", func() {
						JustBeforeEach(func() {
							err = rtime.PauseContainer(id)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"paused\" state", func() {
							container, err := rtime.GetContainerState(id)
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("paused"))
						})
					})

					Describe("resuming a container", func() {
						JustBeforeEach(func() {
							err = rtime.PauseContainer(id)
							Expect(err).NotTo(HaveOccurred())
							err = rtime.ResumeContainer(id)
						})
						It("should not produce an error", func() {
							Expect(err).NotTo(HaveOccurred())
						})
						It("should put the container in the \"running\" state", func() {
							container, err := rtime.GetContainerState(id)
							Expect(err).NotTo(HaveOccurred())
							Expect(container.Status).To(Equal("running"))
						})
					})

					Describe("getting running container processes", func() {
						var (
							pid       int
							processes []runtime.ContainerProcessState
						)
						JustBeforeEach(func(done Done) {
							defer close(done)

							_, err = rtime.ExecProcess(id, longSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							pid, err = rtime.ExecProcess(id, shortSleepProcess, createAllStdioOptions)
							Expect(err).NotTo(HaveOccurred())
							_, err = rtime.WaitOnProcess(id, pid)
							Expect(err).NotTo(HaveOccurred())
							processes, err = rtime.GetRunningContainerProcesses(id)
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
