package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden/client"
	"github.com/cloudfoundry-incubator/garden/client/connection"
	"github.com/eapache/go-resiliency/retrier"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var RootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var GraphRoot = os.Getenv("GARDEN_TEST_GRAPHPATH")
var TarPath = os.Getenv("GARDEN_TAR_PATH")

type RunningGarden struct {
	client.Client

	runner  *ginkgomon.Runner
	process ifrit.Process

	Pid int

	tmpdir string

	DepotDir  string
	GraphRoot string
	GraphPath string

	logger lager.Logger
}

func Start(bin, initBin, kawasakiBin, iodaemonBin, nstarBin string, argv ...string) *RunningGarden {
	network := "unix"
	addr := fmt.Sprintf("/tmp/garden_%d.sock", GinkgoParallelNode())
	tmpDir := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("test-garden-%d", ginkgo.GinkgoParallelNode()),
	)

	if GraphRoot == "" {
		GraphRoot = filepath.Join(tmpDir, "graph")
	}

	graphPath := filepath.Join(GraphRoot, fmt.Sprintf("node-%d", ginkgo.GinkgoParallelNode()))
	depotDir := filepath.Join(tmpDir, "containers")

	MustMountTmpfs(graphPath)

	r := &RunningGarden{
		DepotDir: depotDir,

		GraphRoot: GraphRoot,
		GraphPath: graphPath,
		tmpdir:    tmpDir,
		logger:    lagertest.NewTestLogger("garden-runner"),

		Client: client.New(connection.New(network, addr)),
	}

	c := cmd(tmpDir, depotDir, graphPath, network, addr, bin, initBin, kawasakiBin, iodaemonBin, nstarBin, TarPath, RootFSPath, argv...)
	r.runner = ginkgomon.New(ginkgomon.Config{
		Name:              "guardian",
		Command:           c,
		AnsiColorCode:     "31m",
		StartCheck:        "guardian.started",
		StartCheckTimeout: 30 * time.Second,
	})
	r.process = ifrit.Invoke(r.runner)

	r.Pid = c.Process.Pid

	return r
}

func (r *RunningGarden) Kill() error {
	r.process.Signal(syscall.SIGKILL)
	select {
	case err := <-r.process.Wait():
		return err
	case <-time.After(time.Second * 10):
		r.process.Signal(syscall.SIGKILL)
		return errors.New("timed out waiting for garden to shutdown after 10 seconds")
	}
}

func (r *RunningGarden) DestroyAndStop() error {
	if err := r.DestroyContainers(); err != nil {
		return err
	}

	if err := r.Stop(); err != nil {
		return err
	}

	return nil
}

func (r *RunningGarden) Stop() error {
	r.process.Signal(syscall.SIGTERM)

	var err error
	for i := 0; i < 5; i++ {
		select {
		case err := <-r.process.Wait():
			return err
		case <-time.After(time.Second * 5):
			r.process.Signal(syscall.SIGTERM)
			err = errors.New("timed out waiting for garden to shutdown after 5 seconds")
		}
	}

	r.process.Signal(syscall.SIGKILL)
	return err
}

func cmd(tmpdir, depotDir, graphPath, network, addr, bin, initBin, kawasakiBin, iodaemonBin, nstarBin, tarBin, rootFSPath string, argv ...string) *exec.Cmd {
	Expect(os.MkdirAll(tmpdir, 0755)).To(Succeed())

	snapshotsPath := filepath.Join(tmpdir, "snapshots")

	Expect(os.MkdirAll(depotDir, 0755)).To(Succeed())

	Expect(os.MkdirAll(snapshotsPath, 0755)).To(Succeed())

	appendDefaultFlag := func(ar []string, key, value string) []string {
		for _, a := range argv {
			if a == key {
				return ar
			}
		}

		if value != "" {
			return append(ar, key, value)
		} else {
			return append(ar, key)
		}
	}

	gardenArgs := make([]string, len(argv))
	copy(gardenArgs, argv)

	gardenArgs = appendDefaultFlag(gardenArgs, "--listenNetwork", network)
	gardenArgs = appendDefaultFlag(gardenArgs, "--listenAddr", addr)
	gardenArgs = appendDefaultFlag(gardenArgs, "--depot", depotDir)
	gardenArgs = appendDefaultFlag(gardenArgs, "--graph", graphPath)
	gardenArgs = appendDefaultFlag(gardenArgs, "--tag", fmt.Sprintf("%d", GinkgoParallelNode()))
	gardenArgs = appendDefaultFlag(gardenArgs, "--initBin", initBin)
	gardenArgs = appendDefaultFlag(gardenArgs, "--iodaemonBin", iodaemonBin)
	gardenArgs = appendDefaultFlag(gardenArgs, "--kawasakiBin", kawasakiBin)
	gardenArgs = appendDefaultFlag(gardenArgs, "--nstarBin", nstarBin)
	gardenArgs = appendDefaultFlag(gardenArgs, "--tarBin", tarBin)
	gardenArgs = appendDefaultFlag(gardenArgs, "--logLevel", "debug")
	gardenArgs = appendDefaultFlag(gardenArgs, "--debugAddr", fmt.Sprintf(":808%d", ginkgo.GinkgoParallelNode()))
	gardenArgs = appendDefaultFlag(gardenArgs, "--rootfs", rootFSPath)
	return exec.Command(bin, gardenArgs...)
}

func (r *RunningGarden) Cleanup() {
	// unmount aufs since the docker graph driver leaves this around,
	// otherwise the following commands might fail
	retry := retrier.New(retrier.ConstantBackoff(200, 500*time.Millisecond), nil)

	err := retry.Run(func() error {
		if err := os.RemoveAll(path.Join(r.GraphPath, "aufs")); err == nil {
			return nil // if we can remove it, it's already unmounted
		}

		err := syscall.Unmount(path.Join(r.GraphPath, "aufs"), 0)
		r.logger.Error("failed-unmount-attempt", err)
		return err
	})

	if err != nil {
		r.logger.Error("failed-to-unmount", err)
	}

	MustUnmountTmpfs(r.GraphPath)

	if err := os.RemoveAll(r.GraphPath); err != nil {
		r.logger.Error("remove graph", err)
	}

	if os.Getenv("BTRFS_SUPPORTED") != "" {
		r.cleanupSubvolumes()
	}

	r.logger.Info("cleanup-tempdirs")
	if err := os.RemoveAll(r.tmpdir); err != nil {
		r.logger.Error("cleanup-tempdirs-failed", err, lager.Data{"tmpdir": r.tmpdir})
	} else {
		r.logger.Info("tempdirs-removed")
	}
}

func (r *RunningGarden) cleanupSubvolumes() {
	r.logger.Info("cleanup-subvolumes")

	// need to remove subvolumes before cleaning graphpath
	subvolumesOutput, err := exec.Command("btrfs", "subvolume", "list", "-o", r.GraphRoot).CombinedOutput()
	r.logger.Debug(fmt.Sprintf("listing-subvolumes: %s", string(subvolumesOutput)))
	if err != nil {
		r.logger.Fatal("listing-subvolumes-error", err)
	}

	for _, line := range strings.Split(string(subvolumesOutput), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}

		subvolumePath := fields[len(fields)-1] // this path is relative to the outer Garden-Linux BTRFS mount
		idx := strings.Index(subvolumePath, r.GraphRoot)
		if idx == -1 {
			continue
		}
		subvolumeAbsolutePath := subvolumePath[idx:]

		if strings.Contains(subvolumeAbsolutePath, r.GraphPath) {
			if b, err := exec.Command("btrfs", "subvolume", "delete", subvolumeAbsolutePath).CombinedOutput(); err != nil {
				r.logger.Fatal(fmt.Sprintf("deleting-subvolume: %s", string(b)), err)
			}
		}
	}

	if err := os.RemoveAll(r.GraphPath); err != nil {
		r.logger.Error("remove-graph-again", err)
	}
}

func (r *RunningGarden) DestroyContainers() error {
	containers, err := r.Containers(nil)
	if err != nil {
		return err
	}

	for _, container := range containers {
		r.Destroy(container.Handle())
	}

	return nil
}

func (r *RunningGarden) Buffer() *gbytes.Buffer {
	return r.runner.Buffer()
}
