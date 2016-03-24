package runrunc

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

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/goci"
	"github.com/cloudfoundry-incubator/guardian/gardener"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/opencontainers/runc/libcontainer/user"
	"github.com/pivotal-golang/lager"
)

const DefaultRootPath = "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
const DefaultPath = "PATH=/usr/local/bin:/usr/bin:/bin"

//go:generate counterfeiter . ProcessTracker
//go:generate counterfeiter . Process

type runcStats struct {
	Data struct {
		CgroupStats struct {
			CPUStats struct {
				CPUUsage struct {
					Usage  uint64 `json:"total_usage"`
					System uint64 `json:"usage_in_kernelmode"`
					User   uint64 `json:"usage_in_usermode"`
				} `json:"cpu_usage"`
			} `json:"cpu_stats"`
			MemoryStats struct {
				Stats garden.ContainerMemoryStat `json:"stats"`
			} `json:"memory_stats"`
		} `json:"CgroupStats"`
	}
}

type Process interface {
	garden.Process
}

type ProcessTracker interface {
	Run(id string, cmd *exec.Cmd, io garden.ProcessIO, tty *garden.TTYSpec, pidFile string) (garden.Process, error)
}

//go:generate counterfeiter . UidGenerator
type UidGenerator interface {
	Generate() string
}

//go:generate counterfeiter . UserLookupper
type UserLookupper interface {
	Lookup(rootFsPath string, user string) (*user.ExecUser, error)
}

//go:generate counterfeiter . Mkdirer
type Mkdirer interface {
	MkdirAs(rootfsPath string, uid, gid int, mode os.FileMode, recreate bool, path ...string) error
}

//go:generate counterfeiter . EventsNotifier
type EventsNotifier interface {
	OnEvent(handle string, event string)
}

//go:generate counterfeiter . StatsNotifier
type StatsNotifier interface {
	OnStat(handle string, cpuStat garden.ContainerCPUStat, memoryStat garden.ContainerMemoryStat)
}

type LookupFunc func(rootfsPath, user string) (*user.ExecUser, error)

func (fn LookupFunc) Lookup(rootfsPath, user string) (*user.ExecUser, error) {
	return fn(rootfsPath, user)
}

//go:generate counterfeiter . BundleLoader
type BundleLoader interface {
	Load(path string) (*goci.Bndl, error)
}

// da doo
type RunRunc struct {
	tracker       ProcessTracker
	commandRunner command_runner.CommandRunner
	pidGenerator  UidGenerator
	runc          LoggingRuncBinary

	execPreparer *ExecPreparer
}

//go:generate counterfeiter . RuncBinary
type RuncBinary interface {
	goci.Runc
}

//go:generate counterfeiter . LoggingRuncBinary
type LoggingRuncBinary interface {
	WithLogFile(logFile string) goci.Runc
}

func New(tracker ProcessTracker, runner command_runner.CommandRunner, pidgen UidGenerator, runc LoggingRuncBinary, execPreparer *ExecPreparer) *RunRunc {
	return &RunRunc{
		tracker:       tracker,
		commandRunner: runner,
		pidGenerator:  pidgen,
		runc:          runc,
		execPreparer:  execPreparer,
	}
}

// Starts a bundle by running 'runc' in the bundle directory
func (r *RunRunc) Start(log lager.Logger, bundlePath, id string, _ garden.ProcessIO) (err error) {
	log = log.Session("start", lager.Data{"bundle": bundlePath})

	log.Info("started")
	defer log.Info("finished")

	logFile := filepath.Join(bundlePath, "start.log")

	cmd := r.runc.WithLogFile(logFile).StartCommand(bundlePath, id, true)
	process, err := r.tracker.Run(r.pidGenerator.Generate(), cmd, garden.ProcessIO{Stdout: os.Stdout, Stderr: os.Stderr}, nil, "")
	if err != nil {
		log.Error("run-runc-track-failed", err)
		return err
	}

	defer func() {
		err = processRuncLogs(log, logFile, err)
		if err != nil {
			err = fmt.Errorf("runc start: %s", err)
		}
	}()

	status, err := process.Wait()
	if err != nil {
		log.Error("run-runc-start-failed", err, lager.Data{"exit-status": status})
		return err
	}

	if status > 0 {
		log.Info("run-runc-start-exit-status-not-zero", lager.Data{"exit-status": status})
		err = fmt.Errorf("exit status %d", status)
	}

	return err
}

// Exec a process in a bundle using 'runc exec'
func (r *RunRunc) Exec(log lager.Logger, bundlePath, id string, spec garden.ProcessSpec, io garden.ProcessIO) (garden.Process, error) {
	log = log.Session("exec", lager.Data{"id": id, "path": spec.Path})

	pid := r.pidGenerator.Generate()

	log.Info("started", lager.Data{pid: "pid"})
	defer log.Info("finished")

	logFile := filepath.Join(bundlePath, "exec-"+pid+".log")

	pidFilePath := path.Join(bundlePath, "processes", fmt.Sprintf("%s.pid", pid))
	cmd, err := r.execPreparer.Prepare(log, id, bundlePath, pidFilePath, spec, r.runc.WithLogFile(logFile))
	if err != nil {
		log.Error("prepare-failed", err)
		return nil, err
	}

	process, err := r.tracker.Run(pid, cmd, io, spec.TTY, pidFilePath)
	if err != nil {
		log.Error("run-failed", err)
		return nil, err
	}

	return process, nil
}

type runcEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (r *RunRunc) WatchEvents(log lager.Logger, bundlePath, handle string, eventsNotifier EventsNotifier) error {
	stdoutR, w := io.Pipe()
	cmd := r.runc.WithLogFile("").EventsCommand(handle)
	cmd.Stdout = w

	log = log.Session("watch", lager.Data{
		"handle": handle,
	})
	log.Info("watching")
	defer log.Info("done")

	if err := r.commandRunner.Start(cmd); err != nil {
		log.Error("run-events", err)
		return fmt.Errorf("start: %s", err)
	}
	go r.commandRunner.Wait(cmd) // avoid zombie

	decoder := json.NewDecoder(stdoutR)
	for {
		log.Debug("wait-next-event")

		var event runcEvent
		err := decoder.Decode(&event)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("decode event: %s", err)
		}

		log.Debug("got-event", lager.Data{
			"type": event.Type,
		})
		if event.Type == "oom" {
			eventsNotifier.OnEvent(handle, "Out of memory")
		}
	}
}

func (r *RunRunc) Stats(log lager.Logger, path, id string) (gardener.ActualContainerMetrics, error) {
	buf, err := r.run(log, r.runc.WithLogFile("").StatsCommand(id))
	if err != nil {
		return gardener.ActualContainerMetrics{}, fmt.Errorf("runC stats: %s", err)
	}

	var data runcStats
	if err := json.NewDecoder(buf).Decode(&data); err != nil {
		return gardener.ActualContainerMetrics{}, fmt.Errorf("decode stats: %s", err)
	}

	stats := gardener.ActualContainerMetrics{
		Memory: data.Data.CgroupStats.MemoryStats.Stats,
		CPU: garden.ContainerCPUStat{
			Usage:  data.Data.CgroupStats.CPUStats.CPUUsage.Usage,
			System: data.Data.CgroupStats.CPUStats.CPUUsage.System,
			User:   data.Data.CgroupStats.CPUStats.CPUUsage.User,
		},
	}

	stats.Memory.TotalUsageTowardLimit = stats.Memory.TotalRss + (stats.Memory.TotalCache - stats.Memory.TotalInactiveFile)

	return stats, nil
}

// State gets the state of the bundle
func (r *RunRunc) State(log lager.Logger, bundlePath, handle string) (state State, err error) {
	log = log.Session("State", lager.Data{"handle": handle})

	log.Info("started")
	defer log.Info("finished")

	buff, err := r.run(log, r.runc.WithLogFile("").StateCommand(handle))
	if err != nil {
		log.Error("state-cmd-failed", err)
		return State{}, fmt.Errorf("runc state: %s", err)
	}

	if err := json.NewDecoder(buff).Decode(&state); err != nil {
		log.Error("decode-state-failed", err)
		return State{}, fmt.Errorf("runc state: %s", err)
	}

	return state, nil
}

// Kill a bundle using 'runc kill'
func (r *RunRunc) Kill(log lager.Logger, bundlePath, handle string) (err error) {
	log = log.Session("kill", lager.Data{"handle": handle})

	log.Info("started")
	defer log.Info("finished")

	logFileName, err := logFile(bundlePath, "kill")
	if err != nil {
		return err
	}

	buf, err := r.run(log, r.runc.WithLogFile(logFileName).KillCommand(handle, "KILL"))
	err = processRuncLogs(log, logFileName, err)
	if err != nil {
		log.Error("run-failed", err, lager.Data{"stderr": buf.String()})
		return fmt.Errorf("runc kill: %s", err)
	}

	return nil
}

func logFile(bundlePathPrefix, prefix string) (string, error) {
	file, err := ioutil.TempFile(bundlePathPrefix, prefix)
	if err != nil {
		return "", err
	}

	if err := file.Close(); err != nil {
		return "", err
	}

	return file.Name(), nil
}

// Delete a bundle which was detached (requires the bundle was already killed)
func (r *RunRunc) Delete(log lager.Logger, bundlePath, handle string) error {
	cmd := r.runc.WithLogFile("").DeleteCommand(handle)
	return r.commandRunner.Run(cmd)
}

func (r *RunRunc) run(log lager.Logger, cmd *exec.Cmd) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	cmd.Stderr = buf

	return buf, r.commandRunner.Run(cmd)
}

func processRuncLogs(log lager.Logger, logFile string, err error) error {
	logFileR, openErr := os.Open(logFile)
	if openErr != nil {
		return fmt.Errorf("read log file: %s", openErr)
	}

	buff, readErr := ioutil.ReadAll(logFileR)
	if readErr != nil {
		return fmt.Errorf("read log file: %s", readErr)
	}

	forwardRuncLogsToLager(log, buff)

	if err != nil {
		return wrapWithErrorFromRuncLog(log, err, buff)
	}

	return nil
}
