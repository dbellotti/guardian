package runrunc_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/goci"
	"github.com/cloudfoundry-incubator/guardian/rundmc/runrunc"
	"github.com/cloudfoundry-incubator/guardian/rundmc/runrunc/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runc/libcontainer/user"
	"github.com/opencontainers/specs/specs-go"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("RuncRunner", func() {
	var (
		tracker           *fakes.FakeProcessTracker
		commandRunner     *fake_command_runner.FakeCommandRunner
		pidGenerator      *fakes.FakeUidGenerator
		runcBinary        *fakes.FakeRuncBinary
		loggingRuncBinary *fakes.FakeLoggingRuncBinary
		bundleLoader      *fakes.FakeBundleLoader
		users             *fakes.FakeUserLookupper
		mkdirer           *fakes.FakeMkdirer
		bundlePath        string
		logger            *lagertest.TestLogger

		runner *runrunc.RunRunc
	)

	var rootfsPath = func(bundlePath string) string {
		return "/rootfs/of/bundle" + bundlePath
	}

	BeforeEach(func() {
		tracker = new(fakes.FakeProcessTracker)
		pidGenerator = new(fakes.FakeUidGenerator)
		runcBinary = new(fakes.FakeRuncBinary)
		loggingRuncBinary = new(fakes.FakeLoggingRuncBinary)
		commandRunner = fake_command_runner.New()
		bundleLoader = new(fakes.FakeBundleLoader)
		users = new(fakes.FakeUserLookupper)
		mkdirer = new(fakes.FakeMkdirer)
		logger = lagertest.NewTestLogger("test")

		var err error
		bundlePath, err = ioutil.TempDir("", "bundle")
		Expect(err).NotTo(HaveOccurred())

		runner = runrunc.New(
			tracker,
			commandRunner,
			pidGenerator,
			loggingRuncBinary,
			runrunc.NewExecPreparer(
				bundleLoader,
				users,
				mkdirer,
			),
		)

		bundleLoader.LoadStub = func(path string) (*goci.Bndl, error) {
			bndl := &goci.Bndl{}
			bndl.Spec.Root.Path = rootfsPath(path)
			return bndl, nil
		}

		users.LookupReturns(&user.ExecUser{}, nil)

		runcBinary.StartCommandStub = func(path, id string, detach bool) *exec.Cmd {
			return exec.Command("funC", "start", path, id, fmt.Sprintf("%t", detach))
		}

		runcBinary.ExecCommandStub = func(id, processJSONPath, pidFilePath string) *exec.Cmd {
			return exec.Command("funC", "exec", id, processJSONPath, "--pid-file", pidFilePath)
		}

		runcBinary.KillCommandStub = func(id, signal string) *exec.Cmd {
			return exec.Command("funC", "kill", id, signal)
		}

		runcBinary.StateCommandStub = func(id string) *exec.Cmd {
			return exec.Command("funC", "state", id)
		}

		runcBinary.StatsCommandStub = func(id string) *exec.Cmd {
			return exec.Command("funC-stats", "--handle", id)
		}

		runcBinary.DeleteCommandStub = func(id string) *exec.Cmd {
			return exec.Command("funC", "delete", id)
		}

		loggingRuncBinary.WithLogFileReturns(runcBinary)
	})

	Describe("Start", func() {
		It("starts the container with runC passing the detach flag", func() {
			tracker.RunStub = func(_ string, cmd *exec.Cmd, io garden.ProcessIO, _ *garden.TTYSpec, _ string) (garden.Process, error) {
				logFile := loggingRuncBinary.WithLogFileArgsForCall(0)
				Expect(ioutil.WriteFile(logFile, []byte(""), 0700)).To(Succeed())
				return new(fakes.FakeProcess), nil
			}

			Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(Succeed())

			Expect(tracker.RunCallCount()).To(Equal(1))
			_, cmd, _, _, _ := tracker.RunArgsForCall(0)

			Expect(cmd.Path).To(Equal("funC"))
			Expect(cmd.Args[4]).To(Equal("true"))
		})

		It("returns an error if the log file from start can't be read", func() {
			tracker.RunReturns(new(fakes.FakeProcess), nil)
			Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(MatchError(ContainSubstring("start: read log file")))
		})

		Describe("forwarding logs from runC", func() {
			var (
				exitStatus     int
				errorFromStart error
				logs           string
			)

			BeforeEach(func() {
				exitStatus = 0
				errorFromStart = nil
				logs = `time="2016-03-02T13:56:38Z" level=warning msg="signal: potato"
				time="2016-03-02T13:56:38Z" level=error msg="fork/exec POTATO: no such file or directory"
				time="2016-03-02T13:56:38Z" level=fatal msg="Container start failed: [10] System error: fork/exec POTATO: no such file or directory"`
			})

			JustBeforeEach(func() {
				tracker.RunStub = func(_ string, cmd *exec.Cmd, io garden.ProcessIO, _ *garden.TTYSpec, _ string) (garden.Process, error) {
					logFile := loggingRuncBinary.WithLogFileArgsForCall(0)
					Expect(ioutil.WriteFile(logFile, []byte(logs), 0700)).To(Succeed())
					fakeProcess := new(fakes.FakeProcess)

					fakeProcess.WaitReturns(exitStatus, errorFromStart)
					return fakeProcess, nil
				}
			})

			It("sends all the logs to the logger", func() {
				Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(Succeed())

				runcLogs := make([]lager.LogFormat, 0)
				for _, log := range logger.Logs() {
					if log.Message == "test.start.runc" {
						runcLogs = append(runcLogs, log)
					}
				}

				Expect(runcLogs).To(HaveLen(3))
				Expect(runcLogs[0].Data).To(HaveKeyWithValue("message", "signal: potato"))
			})

			Context("when runC start fails", func() {
				BeforeEach(func() {
					errorFromStart = errors.New("exit status potato")
				})

				It("return an error including parsed logs when runC fails to starts the container", func() {
					Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(MatchError("runc start: exit status potato: Container start failed: [10] System error: fork/exec POTATO: no such file or directory"))
				})

				Context("when the log messages can't be parsed", func() {
					BeforeEach(func() {
						logs = `foo="'
					`
					})

					It("returns an error with only the exit status if the log can't be parsed", func() {
						Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(MatchError("runc start: exit status potato"))
					})
				})
			})

			Context("when runC start exits non-zero", func() {
				BeforeEach(func() {
					exitStatus = 12
				})

				It("return an error including parsed logs when runC fails to starts the container", func() {
					Expect(runner.Start(logger, bundlePath, "some-id", garden.ProcessIO{})).To(MatchError("runc start: exit status 12: Container start failed: [10] System error: fork/exec POTATO: no such file or directory"))
				})
			})
		})
	})

	Describe("Exec", func() {
		It("forwards any logs to lager", func() {
			logs := `time="2016-03-02T13:56:38Z" level=warning msg="signal: potato"
				time="2016-03-02T13:56:38Z" level=error msg="fork/exec POTATO: no such file or directory"
				time="2016-03-02T13:56:38Z" level=fatal msg="Container start failed: [10] System error: fork/exec POTATO: no such file or directory"`

			tracker.RunStub = func(_ string, cmd *exec.Cmd, io garden.ProcessIO, _ *garden.TTYSpec, _ string) (garden.Process, error) {
				logFile := loggingRuncBinary.WithLogFileArgsForCall(0)
				Expect(ioutil.WriteFile(logFile, []byte(logs), 0700)).To(Succeed())
				return new(fakes.FakeProcess), nil
			}

			_, err := runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{}, garden.ProcessIO{Stdout: GinkgoWriter})
			Expect(err).NotTo(HaveOccurred())

			runcLogs := make([]lager.LogFormat, 0)
			for _, log := range logger.Logs() {
				if log.Message == "test.exec.runc" {
					runcLogs = append(runcLogs, log)
				}
			}

			Expect(runcLogs).To(HaveLen(3))
			Expect(runcLogs[0].Data).To(HaveKeyWithValue("message", "signal: potato"))
		})

		It("runs exec against the injected runC binary using process tracker", func() {
			pidGenerator.GenerateReturns("another-process-guid")
			ttyspec := &garden.TTYSpec{WindowSize: &garden.WindowSize{Rows: 1}}
			runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{TTY: ttyspec}, garden.ProcessIO{Stdout: GinkgoWriter})
			Expect(tracker.RunCallCount()).To(Equal(1))

			pid, cmd, io, tty, _ := tracker.RunArgsForCall(0)
			Expect(pid).To(Equal("another-process-guid"))
			Expect(cmd.Args[:3]).To(Equal([]string{"funC", "exec", "some-id"}))
			Expect(io.Stdout).To(Equal(GinkgoWriter))
			Expect(tty).To(Equal(ttyspec))
		})

		It("creates the processes directory if it does not exist", func() {
			runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{}, garden.ProcessIO{Stdout: GinkgoWriter})
			Expect(path.Join(bundlePath, "processes")).To(BeADirectory())
		})

		Context("When creating the processes directory fails", func() {
			It("returns a helpful error", func() {
				Expect(ioutil.WriteFile(path.Join(bundlePath, "processes"), []byte(""), 0700)).To(Succeed())
				_, err := runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{}, garden.ProcessIO{Stdout: GinkgoWriter})
				Expect(err).To(MatchError(MatchRegexp("mkdir .*: .*")))
			})
		})

		It("asks for the pid file to be placed in processes/$guid.pid", func() {
			pidGenerator.GenerateReturns("another-process-guid")
			runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{}, garden.ProcessIO{Stdout: GinkgoWriter})
			Expect(tracker.RunCallCount()).To(Equal(1))

			_, cmd, _, _, _ := tracker.RunArgsForCall(0)
			Expect(cmd.Args[4:]).To(Equal([]string{"--pid-file", path.Join(bundlePath, "/processes/another-process-guid.pid")}))
		})

		It("tells process tracker that it can find the pid-file at processes/$guid.pid", func() {
			pidGenerator.GenerateReturns("another-process-guid")
			runner.Exec(logger, bundlePath, "some-id", garden.ProcessSpec{}, garden.ProcessIO{Stdout: GinkgoWriter})
			Expect(tracker.RunCallCount()).To(Equal(1))

			_, _, _, _, pidFile := tracker.RunArgsForCall(0)
			Expect(pidFile).To(Equal(path.Join(bundlePath, "/processes/another-process-guid.pid")))
		})

		Describe("the process.json passed to 'runc exec'", func() {
			var spec specs.Process

			BeforeEach(func() {
				tracker.RunStub = func(_ string, cmd *exec.Cmd, _ garden.ProcessIO, _ *garden.TTYSpec, _ string) (garden.Process, error) {
					f, err := os.Open(cmd.Args[3])
					Expect(err).NotTo(HaveOccurred())

					json.NewDecoder(f).Decode(&spec)
					return nil, nil
				}
			})

			It("passes a process.json with the correct path and args", func() {
				runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{Path: "to enlightenment", Args: []string{"infinity", "and beyond"}}, garden.ProcessIO{})
				Expect(tracker.RunCallCount()).To(Equal(1))
				Expect(spec.Args).To(Equal([]string{"to enlightenment", "infinity", "and beyond"}))
			})

			It("sets the rlimits correctly", func() {
				ptr := func(n uint64) *uint64 { return &n }
				Expect(runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
					Limits: garden.ResourceLimits{
						As:         ptr(12),
						Core:       ptr(24),
						Cpu:        ptr(36),
						Data:       ptr(99),
						Fsize:      ptr(101),
						Locks:      ptr(111),
						Memlock:    ptr(987),
						Msgqueue:   ptr(777),
						Nice:       ptr(111),
						Nofile:     ptr(222),
						Nproc:      ptr(1234),
						Rss:        ptr(888),
						Rtprio:     ptr(254),
						Sigpending: ptr(101),
						Stack:      ptr(44),
					},
				}, garden.ProcessIO{})).To(Succeed())
				Expect(tracker.RunCallCount()).To(Equal(1))

				Expect(spec.Rlimits).To(ConsistOf(
					specs.Rlimit{Type: "RLIMIT_AS", Hard: 12, Soft: 12},
					specs.Rlimit{Type: "RLIMIT_CORE", Hard: 24, Soft: 24},
					specs.Rlimit{Type: "RLIMIT_CPU", Hard: 36, Soft: 36},
					specs.Rlimit{Type: "RLIMIT_DATA", Hard: 99, Soft: 99},
					specs.Rlimit{Type: "RLIMIT_FSIZE", Hard: 101, Soft: 101},
					specs.Rlimit{Type: "RLIMIT_LOCKS", Hard: 111, Soft: 111},
					specs.Rlimit{Type: "RLIMIT_MEMLOCK", Hard: 987, Soft: 987},
					specs.Rlimit{Type: "RLIMIT_MSGQUEUE", Hard: 777, Soft: 777},
					specs.Rlimit{Type: "RLIMIT_NICE", Hard: 111, Soft: 111},
					specs.Rlimit{Type: "RLIMIT_NOFILE", Hard: 222, Soft: 222},
					specs.Rlimit{Type: "RLIMIT_NPROC", Hard: 1234, Soft: 1234},
					specs.Rlimit{Type: "RLIMIT_RSS", Hard: 888, Soft: 888},
					specs.Rlimit{Type: "RLIMIT_RTPRIO", Hard: 254, Soft: 254},
					specs.Rlimit{Type: "RLIMIT_SIGPENDING", Hard: 101, Soft: 101},
					specs.Rlimit{Type: "RLIMIT_STACK", Hard: 44, Soft: 44},
				))
			})

			Describe("passing the correct uid and gid", func() {
				Context("when the bundle can be loaded", func() {
					BeforeEach(func() {
						users.LookupReturns(&user.ExecUser{Uid: 9, Gid: 7}, nil)
						_, err := runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{User: "spiderman"}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
					})

					It("looks up the user and group IDs of the user in the right rootfs", func() {
						Expect(users.LookupCallCount()).To(Equal(1))
						actualRootfsPath, actualUserName := users.LookupArgsForCall(0)
						Expect(actualRootfsPath).To(Equal(rootfsPath("some/oci/container")))
						Expect(actualUserName).To(Equal("spiderman"))
					})

					It("passes a process.json with the correct user and group ids", func() {
						Expect(spec.User).To(Equal(specs.User{UID: 9, GID: 7}))
					})
				})

				Context("when the bundle can't be loaded", func() {
					BeforeEach(func() {
						bundleLoader.LoadReturns(nil, errors.New("whoa! Hold them horses!"))
					})

					It("fails", func() {
						_, err := runner.Exec(logger, "some/oci/container", "someid",
							garden.ProcessSpec{User: "spiderman"}, garden.ProcessIO{})
						Expect(err).To(MatchError(ContainSubstring("Hold them horses")))
					})
				})

				Context("when User Lookup returns an error", func() {
					It("passes a process.json with the correct user and group ids", func() {
						users.LookupReturns(&user.ExecUser{Uid: 0, Gid: 0}, errors.New("bang"))

						_, err := runner.Exec(logger, "some/oci/container", "some-id", garden.ProcessSpec{User: "spiderman"}, garden.ProcessIO{})
						Expect(err).To(MatchError(ContainSubstring("bang")))
					})
				})
			})

			Context("when the user is specified in the process spec", func() {
				Context("when the environment does not contain a USER", func() {
					It("appends a default user", func() {
						runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
							User: "spiderman",
							Env:  []string{"a=1", "b=3", "c=4", "PATH=a", "HOME=/spidermanhome"},
						}, garden.ProcessIO{})

						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Env).To(ConsistOf("a=1", "b=3", "c=4", "PATH=a", "USER=spiderman", "HOME=/spidermanhome"))
					})
				})

				Context("when the environment does contain a USER", func() {
					It("appends a default user", func() {
						runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
							User: "spiderman",
							Env:  []string{"a=1", "b=3", "c=4", "PATH=a", "USER=superman"},
						}, garden.ProcessIO{})

						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4", "PATH=a", "USER=superman"}))
					})
				})
			})

			Context("when the user is not specified in the process spec", func() {
				Context("when the environment does not contain a USER", func() {
					It("passes the environment variables", func() {
						runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
							Env: []string{"a=1", "b=3", "c=4", "PATH=a"},
						}, garden.ProcessIO{})

						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4", "PATH=a", "USER=root"}))
					})
				})

				Context("when the environment already contains a USER", func() {
					It("passes the environment variables", func() {
						runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
							Env: []string{"a=1", "b=3", "c=4", "PATH=a", "USER=yo"},
						}, garden.ProcessIO{})

						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4", "PATH=a", "USER=yo"}))
					})
				})
			})

			Context("when the environment already contains a PATH", func() {
				It("passes the environment variables", func() {
					runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
						Env: []string{"a=1", "b=3", "c=4", "PATH=a"},
					}, garden.ProcessIO{})

					Expect(tracker.RunCallCount()).To(Equal(1))
					Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4", "PATH=a", "USER=root"}))
				})
			})

			Context("when the environment does not already contain a PATH", func() {
				It("appends a default PATH for the root user", func() {
					users.LookupReturns(&user.ExecUser{Uid: 0, Gid: 0}, nil)
					runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
						Env:  []string{"a=1", "b=3", "c=4"},
						User: "root",
					}, garden.ProcessIO{})

					Expect(tracker.RunCallCount()).To(Equal(1))
					Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4",
						"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "USER=root"}))
				})

				It("appends a default PATH for non-root users", func() {
					users.LookupReturns(&user.ExecUser{Uid: 1000, Gid: 1000}, nil)
					runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
						Env:  []string{"a=1", "b=3", "c=4"},
						User: "alice",
					}, garden.ProcessIO{})

					Expect(tracker.RunCallCount()).To(Equal(1))
					Expect(spec.Env).To(Equal([]string{"a=1", "b=3", "c=4",
						"PATH=/usr/local/bin:/usr/bin:/bin", "USER=alice"}))
				})
			})

			Context("when the container has environment variables", func() {
				var (
					processEnv   []string
					containerEnv []string
					bndl         *goci.Bndl
				)

				BeforeEach(func() {
					containerEnv = []string{"ENV_CONTAINER_NAME=garden"}
					processEnv = []string{"ENV_PROCESS_ID=1"}
				})

				JustBeforeEach(func() {
					bndl = &goci.Bndl{}
					bndl.Spec.Root.Path = "/some/rootfs/path"
					bndl.Spec.Process.Env = containerEnv
					bundleLoader.LoadReturns(bndl, nil)

					_, err := runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
						Env: processEnv,
					}, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("appends the process vars into container vars", func() {
					envWContainer := make([]string, len(spec.Env))
					copy(envWContainer, spec.Env)

					bndl.Spec.Process.Env = []string{}
					bundleLoader.LoadReturns(bndl, nil)

					_, err := runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{
						Env: processEnv,
					}, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())

					Expect(envWContainer).To(Equal(append(containerEnv, spec.Env...)))
				})

				Context("and the container environment contains PATH", func() {
					BeforeEach(func() {
						containerEnv = append(containerEnv, "PATH=/test")
					})

					It("should not apply the default PATH", func() {
						Expect(spec.Env).To(Equal([]string{
							"ENV_CONTAINER_NAME=garden",
							"PATH=/test",
							"ENV_PROCESS_ID=1",
							"USER=root",
						}))
					})
				})
			})

			Context("when the container has capabilities", func() {
				BeforeEach(func() {
					bndl := &goci.Bndl{}
					bndl.Spec.Process.Capabilities = []string{"foo", "bar", "baz"}
					bundleLoader.LoadReturns(bndl, nil)
				})

				It("passes them on to the process", func() {
					_, err := runner.Exec(logger, "some/oci/container", "someid", garden.ProcessSpec{}, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())
					Expect(spec.Capabilities).To(Equal([]string{"foo", "bar", "baz"}))
				})
			})

			Describe("working directory", func() {
				Context("when the working directory is specified", func() {
					It("passes the correct cwd to the spec", func() {
						runner.Exec(
							logger, bundlePath, "someid",
							garden.ProcessSpec{Dir: "/home/dir"}, garden.ProcessIO{},
						)
						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Cwd).To(Equal("/home/dir"))
					})

					Describe("Creating the working directory", func() {
						JustBeforeEach(func() {
							users.LookupReturns(&user.ExecUser{Uid: 1012, Gid: 1013}, nil)

							_, err := runner.Exec(logger, bundlePath, "someid", garden.ProcessSpec{
								Dir: "/path/to/banana/dir",
							}, garden.ProcessIO{})
							Expect(err).NotTo(HaveOccurred())
						})

						Context("when the container is privileged", func() {
							It("creates the working directory", func() {
								Expect(mkdirer.MkdirAsCallCount()).To(Equal(1))
								rootfs, uid, gid, mode, recreate, dirs := mkdirer.MkdirAsArgsForCall(0)
								Expect(rootfs).To(Equal(rootfsPath(bundlePath)))
								Expect(dirs).To(ConsistOf("/path/to/banana/dir"))
								Expect(mode).To(BeNumerically("==", 0755))
								Expect(recreate).To(BeFalse())
								Expect(uid).To(BeEquivalentTo(1012))
								Expect(gid).To(BeEquivalentTo(1013))
							})
						})

						Context("when the container is unprivileged", func() {
							BeforeEach(func() {
								bundleLoader.LoadStub = func(path string) (*goci.Bndl, error) {
									bndl := &goci.Bndl{}
									bndl.Spec.Root.Path = "/rootfs/of/bundle" + path
									bndl.Spec.Linux.UIDMappings = []specs.IDMapping{{
										HostID:      1712,
										ContainerID: 1012,
										Size:        1,
									}}
									bndl.Spec.Linux.GIDMappings = []specs.IDMapping{{
										HostID:      1713,
										ContainerID: 1013,
										Size:        1,
									}}
									return bndl, nil
								}
							})

							It("creates the working directory as the mapped user", func() {
								Expect(mkdirer.MkdirAsCallCount()).To(Equal(1))
								rootfs, uid, gid, mode, recreate, dirs := mkdirer.MkdirAsArgsForCall(0)
								Expect(rootfs).To(Equal(rootfsPath(bundlePath)))
								Expect(dirs).To(ConsistOf("/path/to/banana/dir"))
								Expect(mode).To(BeEquivalentTo(0755))
								Expect(recreate).To(BeFalse())
								Expect(uid).To(BeEquivalentTo(1712))
								Expect(gid).To(BeEquivalentTo(1713))
							})
						})
					})
				})

				Context("when the working directory is not specified", func() {
					It("defaults to the user's HOME directory", func() {
						users.LookupReturns(&user.ExecUser{Home: "/the/home/dir"}, nil)

						runner.Exec(
							logger, bundlePath, "someid",
							garden.ProcessSpec{Dir: ""}, garden.ProcessIO{},
						)

						Expect(tracker.RunCallCount()).To(Equal(1))
						Expect(spec.Cwd).To(Equal("/the/home/dir"))
					})

					It("creates the directory", func() {
						users.LookupReturns(&user.ExecUser{Uid: 1012, Gid: 1013, Home: "/some/dir"}, nil)

						_, err := runner.Exec(logger, bundlePath, "someid", garden.ProcessSpec{}, garden.ProcessIO{})
						Expect(err).NotTo(HaveOccurred())

						Expect(mkdirer.MkdirAsCallCount()).To(Equal(1))
						_, _, _, _, _, dirs := mkdirer.MkdirAsArgsForCall(0)
						Expect(dirs).To(ConsistOf("/some/dir"))
					})
				})

				Context("when the working directory creation fails", func() {
					It("returns an error", func() {
						mkdirer.MkdirAsReturns(errors.New("BOOOOOM"))
						_, err := runner.Exec(logger, bundlePath, "someid", garden.ProcessSpec{}, garden.ProcessIO{})
						Expect(err).To(MatchError(ContainSubstring("create working directory: BOOOOOM")))
					})
				})
			})
		})
	})

	Describe("Kill", func() {
		It("runs 'runc kill' in the container directory", func() {
			Expect(runner.Kill(logger, "some-container")).To(Succeed())
			Expect(commandRunner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "funC",
				Args: []string{"kill", "some-container", "KILL"},
			}))
		})

		It("returns any stderr output when 'runc kill' fails", func() {
			commandRunner.WhenRunning(fake_command_runner.CommandSpec{}, func(cmd *exec.Cmd) error {
				cmd.Stderr.Write([]byte("some error"))
				return errors.New("exit status banana")
			})

			Expect(runner.Kill(logger, "some-container")).To(MatchError("runc kill: exit status banana: some error"))
		})
	})

	Describe("Delete", func() {
		It("deletes the bundle with 'runc delete'", func() {
			Expect(runner.Delete(logger, "some-container")).To(Succeed())
			Expect(commandRunner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "funC",
				Args: []string{"delete", "some-container"},
			}))
		})
	})

	Describe("State", func() {
		var (
			stateCmdOutput string
			stateCmdExit   error
		)

		BeforeEach(func() {
			stateCmdExit = nil
			stateCmdOutput = `{
					"Pid": 4,
					"Status": "quite-a-status"
				}`
		})

		JustBeforeEach(func() {
			commandRunner.WhenRunning(fake_command_runner.CommandSpec{
				Path: "funC",
				Args: []string{"state", "some-container"},
			}, func(cmd *exec.Cmd) error {
				cmd.Stdout.Write([]byte(stateCmdOutput))
				return stateCmdExit
			})
		})

		It("gets the bundle state", func() {
			state, err := runner.State(logger, "some-container")
			Expect(err).NotTo(HaveOccurred())

			Expect(state.Pid).To(Equal(4))
			Expect(state.Status).To(BeEquivalentTo("quite-a-status"))
		})

		Context("when getting state fails", func() {
			BeforeEach(func() {
				stateCmdExit = errors.New("boom")
			})

			It("returns the error", func() {
				_, err := runner.State(logger, "some-container")
				Expect(err).To(
					MatchError(ContainSubstring("boom")),
				)
			})
		})

		Context("when the state output is not JSON", func() {
			BeforeEach(func() {
				stateCmdOutput = "potato"
			})

			It("returns a reasonable error", func() {
				_, err := runner.State(logger, "some-container")
				Expect(err).To(
					MatchError(ContainSubstring("runc state: invalid character 'p'")),
				)
			})
		})
	})

	Describe("Watching for Events", func() {
		BeforeEach(func() {
			runcBinary.EventsCommandStub = func(handle string) *exec.Cmd {
				return exec.Command("funC-events", "events", handle)
			}
		})

		It("blows up if `runc events` returns an error", func() {
			commandRunner.WhenRunning(fake_command_runner.CommandSpec{
				Path: "funC-events",
			}, func(cmd *exec.Cmd) error {
				return errors.New("boom")
			})

			Expect(runner.WatchEvents(logger, "some-container", nil)).To(MatchError("start: boom"))
		})

		Context("when runc events succeeds", func() {
			var (
				eventsCh chan string

				eventsNotifier *fakes.FakeEventsNotifier
			)

			BeforeEach(func() {
				eventsCh = make(chan string, 2)

				commandRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: "funC-events",
				}, func(cmd *exec.Cmd) error {
					go func(stdoutW io.WriteCloser) {
						defer stdoutW.Close()

						for eventJSON := range eventsCh {
							stdoutW.Write([]byte(eventJSON))
						}
					}(cmd.Stdout.(io.WriteCloser))

					return nil
				})

				eventsNotifier = new(fakes.FakeEventsNotifier)
			})

			It("reports an event if one happens", func() {
				defer close(eventsCh)

				go runner.WatchEvents(logger, "some-container", eventsNotifier)

				Consistently(eventsNotifier.OnEventCallCount).Should(Equal(0))

				eventsCh <- `{"type":"oom"}`
				Eventually(eventsNotifier.OnEventCallCount).Should(Equal(1))
				handle, event := eventsNotifier.OnEventArgsForCall(0)
				Expect(handle).To(Equal("some-container"))
				Expect(event).To(Equal("Out of memory"))

				eventsCh <- `{"type":"oom"}`
				Eventually(eventsNotifier.OnEventCallCount).Should(Equal(2))
				handle, event = eventsNotifier.OnEventArgsForCall(1)
				Expect(handle).To(Equal("some-container"))
				Expect(event).To(Equal("Out of memory"))
			})

			It("does not report non-OOM events", func() {
				defer close(eventsCh)

				go runner.WatchEvents(logger, "some-container", eventsNotifier)

				eventsCh <- `{"type":"stats"}`
				Consistently(eventsNotifier.OnEventCallCount).Should(Equal(0))
			})

			It("waits on the process to avoid zombies", func() {
				close(eventsCh)

				Expect(runner.WatchEvents(logger, "some-container", eventsNotifier)).To(Succeed())
				Eventually(commandRunner.WaitedCommands).Should(HaveLen(1))
				Expect(commandRunner.WaitedCommands()[0].Path).To(Equal("funC-events"))
			})
		})
	})

	Describe("Stats", func() {
		Context("when runC reports valid JSON", func() {
			BeforeEach(func() {
				commandRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: "funC-stats",
				}, func(cmd *exec.Cmd) error {
					cmd.Stdout.Write([]byte(`{
					"type": "stats",
					"data": {
						"CgroupStats": {
							"cpu_stats": {
								"cpu_usage": {
									"total_usage": 1,
									"usage_in_kernelmode": 2,
									"usage_in_usermode": 3
								}
							},
							"memory_stats": {
								"stats": {
									"active_anon": 1,
									"active_file": 2,
									"cache": 3,
									"hierarchical_memory_limit": 4,
									"inactive_anon": 5,
									"inactive_file": 6,
									"mapped_file": 7,
									"pgfault": 8,
									"pgmajfault": 9,
									"pgpgin": 10,
									"pgpgout": 11,
									"rss": 12,
									"rss_huge": 13,
									"total_active_anon": 14,
									"total_active_file": 15,
									"total_cache": 16,
									"total_inactive_anon": 17,
									"total_inactive_file": 18,
									"total_mapped_file": 19,
									"total_pgfault": 20,
									"total_pgmajfault": 21,
									"total_pgpgin": 22,
									"total_pgpgout": 23,
									"total_rss": 24,
									"total_rss_huge": 25,
									"total_unevictable": 26,
									"total_writeback": 27,
									"unevictable": 28,
									"writeback": 29,
									"swap": 30,
									"hierarchical_memsw_limit": 31,
									"total_swap": 32
								}
							}
						}
					}
				}`))

					return nil
				})
			})

			It("parses the CPU stats", func() {
				stats, err := runner.Stats(logger, "some-handle")
				Expect(err).NotTo(HaveOccurred())

				Expect(stats.CPU).To(Equal(garden.ContainerCPUStat{
					Usage:  1,
					System: 2,
					User:   3,
				}))
			})

			It("parses the memory stats", func() {
				stats, err := runner.Stats(logger, "some-handle")
				Expect(err).NotTo(HaveOccurred())

				Expect(stats.Memory).To(Equal(garden.ContainerMemoryStat{
					ActiveAnon: 1,
					ActiveFile: 2,
					Cache:      3,
					HierarchicalMemoryLimit: 4,
					InactiveAnon:            5,
					InactiveFile:            6,
					MappedFile:              7,
					Pgfault:                 8,
					Pgmajfault:              9,
					Pgpgin:                  10,
					Pgpgout:                 11,
					Rss:                     12,
					TotalActiveAnon:         14,
					TotalActiveFile:         15,
					TotalCache:              16,
					TotalInactiveAnon:       17,
					TotalInactiveFile:       18,
					TotalMappedFile:         19,
					TotalPgfault:            20,
					TotalPgmajfault:         21,
					TotalPgpgin:             22,
					TotalPgpgout:            23,
					TotalRss:                24,
					TotalUnevictable:        26,
					Unevictable:             28,
					Swap:                    30,
					HierarchicalMemswLimit: 31,
					TotalSwap:              32,
					TotalUsageTowardLimit:  22,
				}))
			})
		})

		Context("when runC reports invalid JSON", func() {
			BeforeEach(func() {
				commandRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: "funC-stats",
				}, func(cmd *exec.Cmd) error {
					cmd.Stdout.Write([]byte(`{ banana potato banana potato }`))

					return nil
				})
			})

			It("should return an error", func() {
				_, err := runner.Stats(logger, "some-container")
				Expect(err).To(MatchError(ContainSubstring("decode stats")))
			})
		})

		Context("when runC fails", func() {
			BeforeEach(func() {
				commandRunner.WhenRunning(fake_command_runner.CommandSpec{
					Path: "funC-stats",
				}, func(cmd *exec.Cmd) error {
					return errors.New("banana")
				})
			})

			It("returns an error", func() {
				_, err := runner.Stats(logger, "some-container")
				Expect(err).To(MatchError(ContainSubstring("runC stats: banana")))
			})
		})
	})
})
