package runc

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utils", func() {
	var (
		rtime *runcRuntime
		err   error
	)

	BeforeEach(func() {
		rt, err := NewRuntime("/tmp/gcs")
		rtime = rt.(*runcRuntime)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		err = cleanupContainers(nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("reading a pid file", func() {
		var (
			pidFile     string
			expectedPid int
			actualPid   int
		)
		BeforeEach(func() {
			// Manually make a pid file.
			pidFile = "/tmp/pid"
			expectedPid = 11111
			err = ioutil.WriteFile(pidFile, []byte(strconv.Itoa(expectedPid)), 0777)
			Expect(err).NotTo(HaveOccurred())
		})
		JustBeforeEach(func() {
			actualPid, err = rtime.readPidFile(pidFile)
		})
		It("should not produce an error", func() {
			Expect(err).NotTo(HaveOccurred())
		})
		It("should get the correct pid", func() {
			Expect(actualPid).To(Equal(expectedPid))
		})
	})

	Describe("cleaning up containers", func() {
		for _, id := range allContainerIds {
			Context(fmt.Sprintf("using ID %s", id), func() {
				var (
					containerDir string
				)
				BeforeEach(func() {
					containerDir = rtime.getContainerDir(id)
					Expect(containerDir).NotTo(BeADirectory())
					err = os.MkdirAll(containerDir, 0777)
					Expect(err).NotTo(HaveOccurred())
					Expect(containerDir).To(BeADirectory())
				})
				JustBeforeEach(func() {
					err = rtime.cleanupContainer(id)
				})
				It("should not produce an error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
				It("should have cleaned up the container directory", func() {
					Expect(containerDir).NotTo(BeADirectory())
				})
			})
		}
	})

	Describe("cleaning up processes", func() {
		for _, id := range allContainerIds {
			Context(fmt.Sprintf("using ID %s", id), func() {
				var (
					processDir string
				)
				BeforeEach(func() {
					processDir = rtime.getProcessDir(id, 123)
					Expect(processDir).NotTo(BeADirectory())
					err = os.MkdirAll(processDir, 0777)
					Expect(err).NotTo(HaveOccurred())
					Expect(processDir).To(BeADirectory())
				})
				JustBeforeEach(func() {
					err = rtime.cleanupProcess(id, 123)
				})
				It("should not produce an error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
				It("should have cleaned up the process directory", func() {
					Expect(processDir).NotTo(BeADirectory())
				})
			})
		}
	})

	Describe("getting the process directory", func() {
		for _, id := range allContainerIds {
			Context(fmt.Sprintf("using ID %s", id), func() {
				var (
					expectedDir string
					actualDir   string
				)
				BeforeEach(func() {
					expectedDir = "/var/run/gcsrunc/" + id + "/123"
				})
				JustBeforeEach(func() {
					actualDir = rtime.getProcessDir(id, 123)
				})
				It("should return the correct directory", func() {
					Expect(actualDir).To(Equal(expectedDir))
				})
			})
		}
	})

	Describe("getting the container directory", func() {
		for _, id := range allContainerIds {
			Context(fmt.Sprintf("using ID %s", id), func() {
				var (
					expectedDir string
					actualDir   string
				)
				BeforeEach(func() {
					expectedDir = "/var/run/gcsrunc/" + id
				})
				JustBeforeEach(func() {
					actualDir = rtime.getContainerDir(id)
				})
				It("should return the correct directory", func() {
					Expect(actualDir).To(Equal(expectedDir))
				})
			})
		}
	})

	Describe("getting the log path", func() {
		var (
			expectedPath string
			actualPath   string
		)
		id := "atestid"
		BeforeEach(func() {
			expectedPath = "/tmp/gcs/" + id + "/runc.log"
		})
		JustBeforeEach(func() {
			actualPath = rtime.getLogPath(id)
		})
		It("should return the correct log path", func() {
			Expect(actualPath).To(Equal(expectedPath))
		})
	})
})
