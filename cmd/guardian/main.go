package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	quotaed_aufs "github.com/cloudfoundry-incubator/garden-shed/docker_drivers/aufs"
	"github.com/cloudfoundry-incubator/garden-shed/layercake"
	"github.com/cloudfoundry-incubator/garden-shed/pkg/retrier"
	"github.com/cloudfoundry-incubator/garden-shed/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-shed/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/cloudfoundry-incubator/goci"
	"github.com/cloudfoundry-incubator/goci/specs"
	"github.com/cloudfoundry-incubator/guardian/gardener"
	"github.com/cloudfoundry-incubator/guardian/kawasaki"
	"github.com/cloudfoundry-incubator/guardian/kawasaki/factory"
	"github.com/cloudfoundry-incubator/guardian/kawasaki/iptables"
	"github.com/cloudfoundry-incubator/guardian/kawasaki/ports"
	"github.com/cloudfoundry-incubator/guardian/kawasaki/subnets"
	"github.com/cloudfoundry-incubator/guardian/logging"
	"github.com/cloudfoundry-incubator/guardian/netplugin"
	"github.com/cloudfoundry-incubator/guardian/properties"
	"github.com/cloudfoundry-incubator/guardian/rundmc"
	"github.com/cloudfoundry-incubator/guardian/rundmc/depot"
	"github.com/cloudfoundry-incubator/guardian/rundmc/process_tracker"
	"github.com/cloudfoundry-incubator/guardian/rundmc/runrunc"
	"github.com/cloudfoundry-incubator/guardian/sysinfo"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	"github.com/docker/docker/daemon/graphdriver"
	_ "github.com/docker/docker/daemon/graphdriver/aufs"
	"github.com/docker/docker/graph"
	_ "github.com/docker/docker/pkg/chrootarchive" // allow reexec of docker-applyLayer
	"github.com/docker/docker/pkg/reexec"
	"github.com/nu7hatch/gouuid"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/localip"
)

const OciStateDir = "/var/run/opencontainer/containers"

var PrivilegedContainerNamespaces = []specs.Namespace{
	goci.NetworkNamespace, goci.PIDNamespace, goci.UTSNamespace, goci.IPCNamespace, goci.MountNamespace,
}

var listenNetwork = flag.String(
	"listenNetwork",
	"unix",
	"how to listen on the address (unix, tcp, etc.)",
)

var listenAddr = flag.String(
	"listenAddr",
	"/tmp/garden.sock",
	"address to listen on",
)

var binPath = flag.String(
	"bin",
	"",
	"directory containing backend-specific scripts (i.e. ./create.sh)",
)

var iodaemonBin = flag.String(
	"iodaemonBin",
	"",
	"path to iodaemon binary",
)

var nstarBin = flag.String(
	"nstarBin",
	"",
	"path to nstar binary",
)

var tarBin = flag.String(
	"tarBin",
	"",
	"path to tar binary",
)

var kawasakiBin = flag.String(
	"kawasakiBin",
	"",
	"path to the kawasaki network hook binary",
)

var networkPlugin = flag.String(
	"networkPlugin",
	"",
	"path to the network plugin hook binary",
)

var networkPluginExtraArgs = flag.String(
	"networkPluginExtraArgs",
	"",
	"path to the extra arg to the network plugin hook binary",
)

var depotPath = flag.String(
	"depot",
	"",
	"directory in which to store containers",
)

var rootFSPath = flag.String(
	"rootfs",
	"",
	"directory of the rootfs for the containers",
)

var graceTime = flag.Duration(
	"containerGraceTime",
	0,
	"time after which to destroy idle containers",
)

var portPoolStart = flag.Uint(
	"portPoolStart",
	60000,
	"start of ephemeral port range used for mapped container ports",
)

var portPoolSize = flag.Uint(
	"portPoolSize",
	5000,
	"size of port pool used for mapped container ports",
)

var networkPool = flag.String("networkPool",
	"10.254.0.0/22",
	"Pool of dynamically allocated container subnets")

var denyNetworks = flag.String(
	"denyNetworks",
	"",
	"CIDR blocks representing IPs to blacklist",
)

var allowNetworks = flag.String(
	"allowNetworks",
	"",
	"CIDR blocks representing IPs to whitelist",
)

var graphRoot = flag.String(
	"graph",
	"/var/lib/garden-docker-graph",
	"docker image graph",
)

var dockerRegistry = flag.String(
	"registry",
	"",
	///registry.IndexServerAddress(),
	"docker registry API endpoint",
)

var insecureRegistries = flag.String(
	"insecureDockerRegistryList",
	"",
	"comma-separated list of docker registries to allow connection to even if they are not secure",
)

var tag = flag.String(
	"tag",
	"",
	"server-wide identifier used for 'global' configuration, must be less than 3 character long",
)

var dropsondeOrigin = flag.String(
	"dropsondeOrigin",
	"garden-linux",
	"Origin identifier for dropsonde-emitted metrics.",
)

var dropsondeDestination = flag.String(
	"dropsondeDestination",
	"localhost:3457",
	"Destination for dropsonde-emitted metrics.",
)

var allowHostAccess = flag.Bool(
	"allowHostAccess",
	false,
	"allow network access to host",
)

var iptablesLogMethod = flag.String(
	"iptablesLogMethod",
	"kernel",
	"type of iptable logging to use, one of 'kernel' or 'nflog' (default: kernel)",
)

var mtu = flag.Int(
	"mtu",
	1500,
	"MTU size for container network interfaces")

var externalIP = flag.String(
	"externalIP",
	"",
	"IP address to use to reach container's mapped ports")

var maxContainers = flag.Uint(
	"maxContainers",
	0,
	"Maximum number of containers that can be created")

var idMappings rootfs_provider.MappingList

func init() {
	maxId := uint32(sysinfo.Min(sysinfo.MustGetMaxValidUID(), sysinfo.MustGetMaxValidGID()))
	idMappings = rootfs_provider.MappingList{
		{
			ContainerID: 0,
			HostID:      maxId,
			Size:        1,
		},
		{
			ContainerID: 1,
			HostID:      1,
			Size:        maxId - 1,
		},
	}
}

func main() {
	if reexec.Init() {
		return
	}

	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("guardian")

	if *depotPath == "" {
		missing("-depot")
	}

	if *iodaemonBin == "" {
		missing("-iodaemonBin")
	}

	if *nstarBin == "" {
		missing("-nstarBin")
	}

	if *tarBin == "" {
		missing("-tarBin")
	}

	resolvedRootFSPath, err := filepath.EvalSymlinks(*rootFSPath)
	if err != nil {
		panic(err)
	}

	_, networkPoolCIDR, err := net.ParseCIDR(*networkPool)
	if err != nil {
		panic(err)
	}

	var denyNetworksList []string
	if *denyNetworks != "" {
		denyNetworksList = strings.Split(*denyNetworks, ",")
	}

	externalIPAddr, err := parseExternalIP(*externalIP)
	if err != nil {
		panic(err)
	}

	interfacePrefix := fmt.Sprintf("g%s", *tag)
	chainPrefix := fmt.Sprintf("g-%s-", *tag)
	ipt := wireIptables(logger, chainPrefix)

	propManager := properties.NewManager()

	var networker gardener.Networker = netplugin.New(*networkPlugin, strings.Split(*networkPluginExtraArgs, ",")...)
	if *networkPlugin == "" {
		networker = wireNetworker(logger, *kawasakiBin, *tag, networkPoolCIDR, externalIPAddr, ipt, interfacePrefix, chainPrefix, propManager)
	}

	backend := &gardener.Gardener{
		SysInfoProvider: sysinfo.NewProvider(*depotPath),
		UidGenerator:    wireUidGenerator(),
		Starter:         wireStarter(logger, ipt, *allowHostAccess, interfacePrefix, denyNetworksList),
		Networker:       networker,
		VolumeCreator:   wireVolumeCreator(logger, *graphRoot),
		Containerizer:   wireContainerizer(logger, *depotPath, *iodaemonBin, *nstarBin, *tarBin, resolvedRootFSPath),
		Logger:          logger,
		PropertyManager: propManager,
	}

	gardenServer := server.New(*listenNetwork, *listenAddr, *graceTime, backend, logger.Session("api"))

	err = gardenServer.Start()
	if err != nil {
		logger.Fatal("failed-to-start-server", err)
	}

	signals := make(chan os.Signal, 1)

	go func() {
		<-signals
		gardenServer.Stop()
		os.Exit(0)
	}()

	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	logger.Info("started", lager.Data{
		"network": *listenNetwork,
		"addr":    *listenAddr,
	})

	select {}
}

func wireUidGenerator() gardener.UidGeneratorFunc {
	return gardener.UidGeneratorFunc(func() string { return mustStringify(uuid.NewV4()) })
}

func wireStarter(logger lager.Logger, ipt *iptables.IPTables, allowHostAccess bool, nicPrefix string, denyNetworks []string) gardener.Starter {
	runner := &logging.Runner{CommandRunner: linux_command_runner.New(), Logger: logger.Session("runner")}

	return &StartAll{starters: []gardener.Starter{
		rundmc.NewStarter(logger, mustOpen("/proc/cgroups"), path.Join(os.TempDir(), fmt.Sprintf("cgroups-%s", *tag)), runner),
		iptables.NewStarter(ipt, allowHostAccess, nicPrefix, denyNetworks),
	}}
}

func wireIptables(logger lager.Logger, prefix string) *iptables.IPTables {
	runner := &logging.Runner{CommandRunner: linux_command_runner.New(), Logger: logger.Session("iptables-runner")}
	return iptables.New(runner, prefix)
}

func wireNetworker(
	log lager.Logger,
	kawasakiBin string,
	tag string,
	networkPoolCIDR *net.IPNet,
	externalIP net.IP,
	ipt *iptables.IPTables,
	interfacePrefix string,
	chainPrefix string,
	propManager *properties.Manager,
) gardener.Networker {
	idGenerator := kawasaki.NewSequentialIDGenerator(time.Now().UnixNano())
	portPool, err := ports.NewPool(uint32(*portPoolStart), uint32(*portPoolSize), ports.State{})
	if err != nil {
		log.Fatal("invalid pool range", err)
	}

	return kawasaki.New(
		kawasakiBin,
		kawasaki.SpecParserFunc(kawasaki.ParseSpec),
		subnets.NewPool(networkPoolCIDR),
		kawasaki.NewConfigCreator(idGenerator, interfacePrefix, chainPrefix, externalIP),
		factory.NewDefaultConfigurer(ipt),
		propManager,
		portPool,
		iptables.NewPortForwarder(ipt),
		iptables.NewFirewallOpener(ipt),
	)
}

func wireVolumeCreator(logger lager.Logger, graphRoot string) *rootfs_provider.CakeOrdinator {
	logger = logger.Session("volume-creator", lager.Data{"graphRoot": graphRoot})
	runner := &logging.Runner{CommandRunner: linux_command_runner.New(), Logger: logger}

	if err := os.MkdirAll(graphRoot, 0755); err != nil {
		logger.Fatal("failed-to-create-graph-directory", err)
	}

	dockerGraphDriver, err := graphdriver.New(graphRoot, nil)
	if err != nil {
		logger.Fatal("failed-to-construct-graph-driver", err)
	}

	backingStoresPath := filepath.Join(graphRoot, "backing_stores")
	if err := os.MkdirAll(backingStoresPath, 0660); err != nil {
		logger.Fatal("failed-to-mkdir-backing-stores", err)
	}

	graphRetrier := &retrier.Retrier{
		Timeout:         100 * time.Second,
		PollingInterval: 500 * time.Millisecond,
		Clock:           clock.NewClock(),
	}

	quotaedGraphDriver := &quotaed_aufs.QuotaedDriver{
		GraphDriver: dockerGraphDriver,
		Unmount:     quotaed_aufs.Unmount,
		BackingStoreMgr: &quotaed_aufs.BackingStore{
			RootPath: backingStoresPath,
			Logger:   logger.Session("backing-store-mgr"),
		},
		LoopMounter: &quotaed_aufs.Loop{
			Retrier: graphRetrier,
			Logger:  logger.Session("loop-mounter"),
		},
		Retrier:  graphRetrier,
		RootPath: graphRoot,
		Logger:   logger.Session("quotaed-driver"),
	}

	dockerGraph, err := graph.NewGraph(graphRoot, quotaedGraphDriver)
	if err != nil {
		logger.Fatal("failed-to-construct-graph", err)
	}

	var cake layercake.Cake = &layercake.Docker{
		Graph:  dockerGraph,
		Driver: quotaedGraphDriver,
	}

	if cake.DriverName() == "aufs" {
		cake = &layercake.AufsCake{
			Cake:      cake,
			Runner:    runner,
			GraphRoot: graphRoot,
		}
	}

	ovenCleanerCake := &layercake.OvenCleaner{
		Cake:               cake,
		Logger:             logger.Session("oven-cleaner"),
		EnableImageCleanup: true,
	}

	repoFetcher := &repository_fetcher.CompositeFetcher{
		LocalFetcher: &repository_fetcher.Local{
			Cake:              ovenCleanerCake,
			DefaultRootFSPath: *rootFSPath,
			IDProvider:        repository_fetcher.LayerIDProvider{},
		},
	}

	rootFSNamespacer := &rootfs_provider.UidNamespacer{
		Logger: logger,
		Translator: rootfs_provider.NewUidTranslator(
			idMappings, // uid
			idMappings, // gid
		),
	}

	layerCreator := rootfs_provider.NewLayerCreator(
		ovenCleanerCake, rootfs_provider.SimpleVolumeCreator{}, rootFSNamespacer)

	cakeOrdinator := rootfs_provider.NewCakeOrdinator(
		ovenCleanerCake, repoFetcher, layerCreator, nil,
	)

	return cakeOrdinator
}

func wireContainerizer(log lager.Logger, depotPath, iodaemonPath, nstarPath, tarPath, defaultRootFSPath string) *rundmc.Containerizer {
	depot := depot.New(depotPath)

	startChecker := rundmc.StartChecker{Expect: "Pid 1 Running", Timeout: 15 * time.Second}
	stateChecker := rundmc.StateChecker{StateFileDir: OciStateDir}

	commandRunner := linux_command_runner.New()

	runcrunner := runrunc.New(
		process_tracker.New(path.Join(os.TempDir(), fmt.Sprintf("garden-%s", *tag), "processes"), iodaemonPath, commandRunner),
		commandRunner,
		wireUidGenerator(),
		goci.RuncBinary("runc"),
		&goci.BndlLoader{},
		runrunc.LookupFunc(runrunc.LookupUser),
	)

	baseBundle := goci.Bundle().
		WithNamespaces(PrivilegedContainerNamespaces...).
		WithResources(&specs.Resources{}).
		WithMounts(
		goci.Mount{Name: "proc", Type: "proc", Source: "proc", Destination: "/proc"},
		goci.Mount{Name: "shm", Type: "tmpfs", Source: "tmpfs", Destination: "/dev/shm"},
		goci.Mount{Name: "pts", Type: "devpts", Source: "devpts", Destination: "/dev/pts",
			Options: []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"}},
	).WithRootFS(defaultRootFSPath).
		WithProcess(goci.Process("/bin/sh", "-c", `echo "Pid 1 Running"; read x`)).
		WithDevices(
		specs.Device{Path: "/dev/null", Type: 'c', Major: 1, Minor: 3, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
		specs.Device{Path: "/dev/tty", Type: 'c', Major: 5, Minor: 0, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
		specs.Device{Path: "/dev/random", Type: 'c', Major: 1, Minor: 8, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
		specs.Device{Path: "/dev/urandom", Type: 'c', Major: 1, Minor: 9, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
		specs.Device{Path: "/dev/zero", Type: 'c', Major: 1, Minor: 5, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
		specs.Device{Path: "/dev/full", Type: 'c', Major: 1, Minor: 7, UID: 0, GID: 0, Permissions: "rwm", FileMode: 0666},
	)

	template := &rundmc.BundleTemplate{
		Rules: []rundmc.BundlerRule{
			rundmc.BaseTemplateRule{
				PrivilegedBase:   baseBundle,
				UnprivilegedBase: baseBundle.WithNamespace(goci.UserNamespace).WithUIDMappings(idMappings...).WithGIDMappings(idMappings...),
			},
			rundmc.RootFSRule{
				ContainerRootUID: idMappings.Map(0),
				ContainerRootGID: idMappings.Map(0),
				MkdirChowner:     rundmc.MkdirChownFunc(rundmc.MkdirChown),
				DirRemover:       rundmc.OsDirRemover(os.Remove),
			},
			rundmc.LimitsRule{},
			rundmc.NetworkHookRule{LogFilePattern: filepath.Join(depotPath, "%s", "network.log")},
			rundmc.BindMountsRule{},
		},
	}

	nstar := rundmc.NewNstarRunner(nstarPath, tarPath, linux_command_runner.New())
	return rundmc.New(depot, template, runcrunner, startChecker, stateChecker, nstar)
}

func missing(flagName string) {
	println("missing " + flagName)
	println()
	flag.Usage()

	os.Exit(1)
}

func parseExternalIP(ip string) (net.IP, error) {
	if *externalIP == "" {
		localIP, err := localip.LocalIP()
		if err != nil {
			return nil, fmt.Errorf("Couldn't determine local IP to use for -externalIP parameter. You can use the -externalIP flag to pass an external IP")
		}
		externalIP = &localIP
	}

	externalIPAddr := net.ParseIP(*externalIP)
	if externalIPAddr == nil {
		return nil, fmt.Errorf("Value of -externalIP %s could not be converted to an IP", *externalIP)
	}
	return externalIPAddr, nil
}

func mustStringify(s interface{}, e error) string {
	if e != nil {
		panic(e)
	}

	return fmt.Sprintf("%s", s)
}

func mustOpen(path string) io.ReadCloser {
	if r, err := os.Open(path); err != nil {
		panic(err)
	} else {
		return r
	}
}

type StartAll struct {
	starters []gardener.Starter
}

func (s *StartAll) Start() error {
	for _, starter := range s.starters {
		if err := starter.Start(); err != nil {
			return err
		}
	}

	return nil
}
