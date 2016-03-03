package gqt_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/cloudfoundry-incubator/guardian/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Creating a Container", func() {
	var (
		initialSockets int
		initialPipes   int
	)

	BeforeEach(func() {
		client = startGarden()
		initialSockets = numOpenSockets(client.Pid)
		initialPipes = numPipes(client.Pid)
	})

	AfterEach(func() {
		Expect(client.DestroyAndStop()).To(Succeed())
	})

	Context("after creating a container without a specified handle", func() {
		var (
			privileged bool

			initProcPid int
		)

		JustBeforeEach(func() {
			var err error
			container, err = client.Create(garden.ContainerSpec{
				Privileged: privileged,
			})
			Expect(err).NotTo(HaveOccurred())

			initProcPid = initProcessPID(container.Handle())
		})

		It("should create a depot subdirectory based on the container handle", func() {
			Expect(container.Handle()).NotTo(BeEmpty())
			Expect(filepath.Join(client.DepotDir, container.Handle())).To(BeADirectory())
			Expect(filepath.Join(client.DepotDir, container.Handle(), "config.json")).To(BeARegularFile())
		})

		It("should lookup the right container", func() {
			lookupContainer, lookupError := client.Lookup(container.Handle())

			Expect(lookupError).NotTo(HaveOccurred())
			Expect(lookupContainer).To(Equal(container))
		})

		It("should not leak pipes", func() {
			process, err := container.Run(garden.ProcessSpec{Path: "echo", Args: []string{"hello"}}, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))

			Expect(client.Destroy(container.Handle())).To(Succeed())
			container = nil // avoid double-destroying

			Eventually(func() int { return numPipes(client.Pid) }).Should(Equal(initialPipes))
		})

		It("should not leak sockets", func() {
			Expect(client.Destroy(container.Handle())).To(Succeed())
			container = nil // avoid double-destroying

			Eventually(func() int { return numOpenSockets(client.Pid) }).Should(Equal(initialSockets))
		})

		DescribeTable("placing the container in to all namespaces", func(ns string) {
			hostNS, err := gexec.Start(exec.Command("ls", "-l", fmt.Sprintf("/proc/1/ns/%s", ns)), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			containerNS, err := gexec.Start(exec.Command("ls", "-l", fmt.Sprintf("/proc/%d/ns/%s", initProcPid, ns)), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(containerNS).Should(gexec.Exit(0))
			Eventually(hostNS).Should(gexec.Exit(0))

			hostFD := strings.Split(string(hostNS.Out.Contents()), ">")[1]
			containerFD := strings.Split(string(containerNS.Out.Contents()), ">")[1]

			Expect(hostFD).NotTo(Equal(containerFD))
		},
			Entry("should place the container in to the NET namespace", "net"),
			Entry("should place the container in to the IPC namespace", "ipc"),
			Entry("should place the container in to the UTS namespace", "uts"),
			Entry("should place the container in to the PID namespace", "pid"),
			Entry("should place the container in to the MNT namespace", "mnt"),
			Entry("should place the container in to the USER namespace", "user"),
		)

		It("should have the proper uid and gid mappings", func() {
			buffer := gbytes.NewBuffer()
			proc, err := container.Run(garden.ProcessSpec{
				Path: "cat",
				Args: []string{"/proc/self/uid_map"},
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(buffer, GinkgoWriter),
				Stderr: GinkgoWriter,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(proc.Wait()).To(Equal(0))

			Eventually(buffer).Should(gbytes.Say(`0\s+4294967294\s+1\n\s+1\s+1\s+4294967293`))
		})

		Context("which is privileged", func() {
			BeforeEach(func() {
				privileged = true
			})

			It("should not place the container in its own user namespace", func() {
				hostNS, err := gexec.Start(exec.Command("ls", "-l", "/proc/1/ns/user"), GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				containerNS, err := gexec.Start(exec.Command("ls", "-l", fmt.Sprintf("/proc/%d/ns/user", initProcPid)), GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(containerNS).Should(gexec.Exit(0))
				Eventually(hostNS).Should(gexec.Exit(0))

				hostFD := strings.Split(string(hostNS.Out.Contents()), ">")[1]
				containerFD := strings.Split(string(containerNS.Out.Contents()), ">")[1]

				Expect(hostFD).To(Equal(containerFD))
			})
		})
	})

	Context("after creating a container with a specified root filesystem", func() {
		var rootFSPath string

		BeforeEach(func() {
			var err error

			rootFSPath, err = ioutil.TempDir("", "test-rootfs")
			Expect(err).NotTo(HaveOccurred())
			command := fmt.Sprintf("cp -rf %s/* %s", os.Getenv("GARDEN_TEST_ROOTFS"), rootFSPath)
			Expect(exec.Command("sh", "-c", command).Run()).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(rootFSPath, "my-file"), []byte("some-content"), 0644)).To(Succeed())
			Expect(os.Mkdir(path.Join(rootFSPath, "somedir"), 0777)).To(Succeed())

			container, err = client.Create(garden.ContainerSpec{
				RootFSPath: rootFSPath,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("provides the containers with the right rootfs", func() {
			Expect(container).To(HaveFile("/my-file"))
		})

		It("isolates the filesystem properly for multiple containers", func() {
			runCommand(container, "touch", []string{"/somedir/created-file"})
			Expect(container).To(HaveFile("/somedir/created-file"))

			container2, err := client.Create(garden.ContainerSpec{
				RootFSPath: rootFSPath,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(container2).To(HaveFile("/my-file"))
			Expect(container2).NotTo(HaveFile("/somedir/created-file"))
		})
	})

	Context("after creating a container with a specified handle", func() {
		It("should lookup the right container for the handle", func() {
			container, err := client.Create(garden.ContainerSpec{
				Handle: "container-banana",
			})
			Expect(err).NotTo(HaveOccurred())

			lookupContainer, lookupError := client.Lookup("container-banana")
			Expect(lookupError).NotTo(HaveOccurred())
			Expect(lookupContainer).To(Equal(container))
		})

		It("allow the container to be created with the same name after destroying", func() {
			container, err := client.Create(garden.ContainerSpec{
				Handle: "another-banana",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(client.Destroy(container.Handle())).To(Succeed())

			container, err = client.Create(garden.ContainerSpec{
				Handle: "another-banana",
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when creating a container fails", func() {
		It("should not leak networking configuration", func() {
			_, err := client.Create(garden.ContainerSpec{
				Network:    fmt.Sprintf("172.250.%d.20/24", GinkgoParallelNode()),
				RootFSPath: "/banana/does/not/exist",
			})
			Expect(err).To(HaveOccurred())

			session, err := gexec.Start(
				exec.Command("ifconfig"),
				GinkgoWriter, GinkgoWriter,
			)
			Expect(err).NotTo(HaveOccurred())
			Consistently(session).ShouldNot(gbytes.Say(fmt.Sprintf("172-250-%d-0", GinkgoParallelNode())))
		})
	})
})

func initProcessPID(handle string) int {
	Eventually(fmt.Sprintf("/run/opencontainer/containers/%s/state.json", handle)).Should(BeAnExistingFile())

	state := struct {
		Pid int `json:"init_process_pid"`
	}{}

	Eventually(func() error {
		stateFile, err := os.Open(fmt.Sprintf("/run/opencontainer/containers/%s/state.json", handle))
		Expect(err).NotTo(HaveOccurred())
		defer stateFile.Close()

		// state.json is sometimes empty immediately after creation, so keep
		// trying until it's valid json
		return json.NewDecoder(stateFile).Decode(&state)
	}).Should(Succeed())

	return state.Pid
}

func runCommand(container garden.Container, path string, args []string) {
	proc, err := container.Run(
		garden.ProcessSpec{
			Path: path,
			Args: args,
		},
		ginkgoIO)
	Expect(err).NotTo(HaveOccurred())

	exitCode, err := proc.Wait()
	Expect(err).NotTo(HaveOccurred())
	Expect(exitCode).To(Equal(0))
}

func numOpenSockets(pid int) (num int) {
	sess, err := gexec.Start(exec.Command("sh", "-c", fmt.Sprintf("lsof -p %d | grep sock", pid)), GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	return bytes.Count(sess.Out.Contents(), []byte{'\n'})
}

func numPipes(pid int) (num int) {
	sess, err := gexec.Start(exec.Command("sh", "-c", fmt.Sprintf("lsof -p %d | grep pipe", pid)), GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	return bytes.Count(sess.Out.Contents(), []byte{'\n'})
}
