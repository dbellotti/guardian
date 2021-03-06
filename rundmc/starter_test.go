package rundmc_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/guardian/rundmc"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("CgroupStarter", func() {
	var (
		runner          *fake_command_runner.FakeCommandRunner
		starter         *rundmc.CgroupStarter
		procCgroups     *FakeReadCloser
		procSelfCgroups *FakeReadCloser
		logger          lager.Logger

		tmpDir string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "gdncgroup")
		Expect(err).NotTo(HaveOccurred())

		logger = lagertest.NewTestLogger("test")
		runner = fake_command_runner.New()
		procCgroups = &FakeReadCloser{Buffer: bytes.NewBufferString("")}
		procSelfCgroups = &FakeReadCloser{Buffer: bytes.NewBufferString("")}
		starter = &rundmc.CgroupStarter{
			CgroupPath:      path.Join(tmpDir, "cgroup"),
			CommandRunner:   runner,
			ProcCgroups:     procCgroups,
			ProcSelfCgroups: procSelfCgroups,
			Logger:          logger,
		}
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("mkdirs the cgroup path", func() {
		starter.Start()
		Expect(path.Join(tmpDir, "cgroup")).To(BeADirectory())
	})

	Context("when the cgroup path is not a mountpoint", func() {
		BeforeEach(func() {
			runner.WhenRunning(fake_command_runner.CommandSpec{
				Path: "mountpoint",
				Args: []string{"-q", path.Join(tmpDir, "cgroup")},
			}, func(cmd *exec.Cmd) error {
				return errors.New("not a mountpoint")
			})
		})

		It("mounts it", func() {
			starter.Start()
			Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "mount",
				Args: []string{"-t", "tmpfs", "-o", "uid=0,gid=0,mode=0755", "cgroup", path.Join(tmpDir, "cgroup")},
			}))
		})
	})

	Context("when the cgroup path exists", func() {
		It("does not mount it again", func() {
			starter.Start()
			Expect(runner).NotTo(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "mount",
				Args: []string{"-t", "tmpfs", "-o", "uid=0,gid=0,mode=0755", "cgroup", path.Join(tmpDir, "cgroup")},
			}))
		})
	})

	Context("when the ProcCgroup reader contains entries (after the header), and ProcSelfCgroup reader contains the mount scheme", func() {
		BeforeEach(func() {
			procCgroups.Write([]byte(
				`header header header
---- ---- ----
devices blah blah
memory lala la
cpu trala la
cpuacct blahdy blah
`))

			procSelfCgroups.Write([]byte(
				`5:devices:/
4:memory:/
3:cpu,cpuacct:/
`))
		})

		Context("and the hierarchy is not mounted", func() {
			BeforeEach(func() {
				for _, notMounted := range []string{"devices", "cpu", "cpuacct"} {
					runner.WhenRunning(fake_command_runner.CommandSpec{
						Path: "mountpoint",
						Args: []string{"-q", path.Join(tmpDir, "cgroup", notMounted)},
					}, func(cmd *exec.Cmd) error {
						return errors.New("not a mountpoint")
					})
				}
			})

			It("mounts the hierarchies which are not mounted", func() {
				starter.Start()
				Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "mount",
					Args: []string{"-n", "-t", "cgroup", "-o", "devices", "cgroup", path.Join(tmpDir, "cgroup", "devices")},
				}))

				Expect(runner).NotTo(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "mount",
					Args: []string{"-n", "-t", "cgroup", "-o", "memory", "cgroup", path.Join(tmpDir, "cgroup", "memory")},
				}))

				Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "mount",
					Args: []string{"-n", "-t", "cgroup", "-o", "cpu,cpuacct", "cgroup", path.Join(tmpDir, "cgroup", "cpu")},
				}))

				Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "mount",
					Args: []string{"-n", "-t", "cgroup", "-o", "cpu,cpuacct", "cgroup", path.Join(tmpDir, "cgroup", "cpuacct")},
				}))
			})

			It("creates needed directories", func() {
				starter.Start()
				Expect(path.Join(tmpDir, "cgroup", "devices")).To(BeADirectory())
			})
		})
	})

	It("closes the procCgroups reader", func() {
		starter.Start()
		Expect(procCgroups.closed).To(BeTrue())
	})

	It("closes the procSelfCgroups reader", func() {
		starter.Start()
		Expect(procSelfCgroups.closed).To(BeTrue())
	})
})

type FakeReadCloser struct {
	closed bool
	*bytes.Buffer
}

func (f *FakeReadCloser) Close() error {
	f.closed = true
	return nil
}
