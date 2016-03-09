package main_test

import (
	"io/ioutil"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Reap", func() {
	var (
		reap string
	)

	BeforeEach(func() {
		var err error
		reap, err = gexec.Build("github.com/cloudfoundry-incubator/guardian/cmd/reap")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("executing the program from the arguments", func() {
		It("forwards the output stream", func() {
			sess, err := gexec.Start(exec.Command(reap, "echo", "hello", "world"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(sess.Out).Should(gbytes.Say("hello world"))
		})

		It("forwards the error stream", func() {
			sess, err := gexec.Start(exec.Command(reap, "sh", "-c", "echo hello world 1>&2"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(sess.Err).Should(gbytes.Say("hello world"))
		})

		It("reports the exit status on fd 3", func() {
			exitStatusR, exitStatusW, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command(reap, "sh", "-c", "exit 12")
			cmd.ExtraFiles = []*os.File{
				exitStatusW,
			}

			_, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			e := make([]byte, 1)
			_, err = exitStatusR.Read(e)
			Expect(err).NotTo(HaveOccurred())

			Expect(e[0]).To(BeEquivalentTo(12))
		})

		It("waits for the grandchild to exit and forwards its exit status", func() {
			pidFile, err := ioutil.TempFile("", "pidfile")
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command(reap, "--pid-file", pidFile.Name(), "--", reap, "--pid-file", pidFile.Name(), "test")

			_, exitStatusW, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())

			cmd.ExtraFiles = []*os.File{
				exitStatusW,
			}

			sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, "3s").Should(gexec.Exit(99))
		})
	})
})
