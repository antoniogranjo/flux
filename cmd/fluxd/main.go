package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"k8s.io/client-go/1.5/rest"

	//	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	transport "github.com/weaveworks/flux/http"
	daemonhttp "github.com/weaveworks/flux/http/daemon"
	"github.com/weaveworks/flux/platform"
	"github.com/weaveworks/flux/platform/kubernetes"
	"github.com/weaveworks/flux/registry"
)

var version string

func main() {
	// Flag domain.
	fs := pflag.NewFlagSet("default", pflag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "DESCRIPTION\n")
		fmt.Fprintf(os.Stderr, "  fluxd is the agent of flux.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		fs.PrintDefaults()
	}
	// This mirrors how kubectl extracts information from the environment.
	var (
		listenAddr = fs.StringP("listen", "l", ":3031", "Listen address where /metrics and API will be served")
		//		fluxsvcAddress    = fs.String("fluxsvc-address", "wss://cloud.weave.works/api/flux", "Address of the fluxsvc to connect to.")
		//		token             = fs.String("token", "", "Token to use to authenticate with flux service")
		kubernetesKubectl = fs.String("kubernetes-kubectl", "", "Optional, explicit path to kubectl tool")
		versionFlag       = fs.Bool("version", false, "Get version number")
		// Git repo & key
		gitURL    = fs.String("git-url", "", "URL of git repo with Kubernetes manifests; e.g., git@github.com:weaveworks/flux-example")
		gitBranch = fs.String("git-branch", "master", "branch of git repo to use for Kubernetes manifests")
		gitPath   = fs.String("git-path", "", "path within git repo to locate Kubernetes manifests")
		gitKey    = fs.String("git-key", "", "path in local filesystem to (deploy) key")
		// registry
		dockerCredFile      = fs.String("docker-config", "~/.docker/config.json", "Path to config file with credentials for DockerHub, quay.io etc.")
		memcachedHostname   = fs.String("memcached-hostname", "", "Hostname for memcached service to use when caching chunks. If empty, no memcached will be used.")
		memcachedTimeout    = fs.Duration("memcached-timeout", 100*time.Millisecond, "Maximum time to wait before giving up on memcached requests.")
		memcachedService    = fs.String("memcached-service", "memcached", "SRV service used to discover memcache servers.")
		registryCacheExpiry = fs.Duration("registry-cache-expiry", 20*time.Minute, "Duration to keep cached registry tag info. Must be < 1 month.")

		upstreamURL = fs.String("connect", "", "Connect to an upstream service e.g., Weave Cloud, at this base address")
		token       = fs.String("token", "", "Authentication token for upstream service")
	)
	fs.Parse(os.Args)

	if version == "" {
		version = "unversioned"
	}
	if *versionFlag {
		fmt.Println(version)
		os.Exit(0)
	}

	// Logger component.
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.NewContext(logger).With("ts", log.DefaultTimestampUTC)
		logger = log.NewContext(logger).With("caller", log.DefaultCaller)
	}

	// Platform component.
	var k8s platform.Cluster
	{
		restClientConfig, err := rest.InClusterConfig()
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}

		restClientConfig.QPS = 50.0
		restClientConfig.Burst = 100

		// When adding a new platform, don't just bash it in. Create a Platform
		// or Cluster interface in package platform, and have kubernetes.Cluster
		// and your new platform implement that interface.
		logger := log.NewContext(logger).With("component", "platform")
		logger.Log("host", restClientConfig.Host)

		kubectl := *kubernetesKubectl
		if kubectl == "" {
			kubectl, err = exec.LookPath("kubectl")
		} else {
			_, err = os.Stat(kubectl)
		}
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}
		logger.Log("kubectl", kubectl)

		kubectlApplier := kubernetes.NewKubectl(kubectl, restClientConfig)
		cluster, err := kubernetes.NewCluster(restClientConfig, kubectlApplier, logger)
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}

		if err := cluster.Ping(); err != nil {
			logger.Log("ping", err)
		} else {
			logger.Log("ping", true)
		}

		k8s = cluster
	}

	var reg registry.Registry
	{
		var memcacheClient registry.MemcacheClient
		if *memcachedHostname != "" {
			memcacheClient = registry.NewMemcacheClient(registry.MemcacheConfig{
				Host:           *memcachedHostname,
				Service:        *memcachedService,
				Timeout:        *memcachedTimeout,
				UpdateInterval: 1 * time.Minute,
				Logger:         log.NewContext(logger).With("component", "memcached"),
			})
			memcacheClient = registry.InstrumentMemcacheClient(memcacheClient)
			defer memcacheClient.Stop()
		}

		creds, err := registry.CredentialsFromFile(*dockerCredFile)
		if err != nil {
			logger.Log("err", err)
		}
		registryLogger := log.NewContext(logger).With("component", "registry")
		reg := registry.NewRegistry(
			registry.NewRemoteClientFactory(creds, registryLogger, memcacheClient, *registryCacheExpiry),
			registryLogger,
		)
		reg = registry.NewInstrumentedRegistry(reg)
	}

	var repo git.Repo
	{
		repo = git.Repo{
			URL:    *gitURL,
			Path:   *gitPath,
			Branch: *gitBranch,
			Key:    *gitKey, // FIXME this should be treated as a path now
		}
	}

	daemon := &platform.Daemon{
		V:        version,
		Cluster:  k8s,
		Repo:     repo,
		Registry: reg,
	}

	// Connect to fluxsvc if given an upstream address
	if *upstreamURL != "" {
		daemonLogger := log.NewContext(logger).With("component", "upstream")
		daemonLogger.Log("connectURL", *upstreamURL)
		upstream, err := daemonhttp.NewUpstream(
			&http.Client{Timeout: 10 * time.Second},
			fmt.Sprintf("fluxd/%v", version),
			flux.Token(*token),
			transport.NewServiceRouter(), // TODO should be NewUpstreamRouter, since it only need the registration endpoint
			*upstreamURL,
			&platform.ErrorLoggingPlatform{daemon, daemonLogger},
			daemonLogger,
		)
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}
		defer upstream.Close()
	}

	// Mechanical components.
	errc := make(chan error)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	// HTTP transport component, for metrics
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		handler := daemonhttp.NewHandler(daemon, daemonhttp.NewRouter())
		mux.Handle("/api/flux/", http.StripPrefix("/api/flux", handler))
		logger.Log("addr", *listenAddr)
		errc <- http.ListenAndServe(*listenAddr, mux)
	}()

	// Go!
	logger.Log("exiting", <-errc)
}
