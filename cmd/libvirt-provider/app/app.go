// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	goflag "flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/ironcore-image/oci/remote"
	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	"github.com/ironcore-dev/ironcore/broker/common"
	commongrpc "github.com/ironcore-dev/ironcore/broker/common/grpc"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/console"
	"github.com/ironcore-dev/libvirt-provider/internal/controllers"
	"github.com/ironcore-dev/libvirt-provider/internal/event"
	"github.com/ironcore-dev/libvirt-provider/internal/healthcheck"
	"github.com/ironcore-dev/libvirt-provider/internal/host"
	"github.com/ironcore-dev/libvirt-provider/internal/libvirt/guest"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/internal/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/internal/mcr"
	"github.com/ironcore-dev/libvirt-provider/internal/networkinterfaceplugin"
	"github.com/ironcore-dev/libvirt-provider/internal/oci"
	volumeplugin "github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/ceph"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/emptydisk"
	"github.com/ironcore-dev/libvirt-provider/internal/qcow2"
	"github.com/ironcore-dev/libvirt-provider/internal/raw"
	"github.com/ironcore-dev/libvirt-provider/internal/server"
	"github.com/ironcore-dev/libvirt-provider/internal/strategy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	homeDir string
	scheme  = runtime.NewScheme()
)

func init() {
	homeDir, _ = os.UserHomeDir()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apinetv1alpha1.AddToScheme(scheme))
}

type Options struct {
	Address          string
	StreamingAddress string
	BaseURL          string

	Servers ServersOptions

	RootDir string

	PathSupportedMachineClasses string
	ResyncIntervalVolumeSize    time.Duration

	ApinetKubeconfig string

	EnableHugepages bool

	GuestAgent GuestAgentOption

	Libvirt   LibvirtOptions
	NicPlugin *networkinterfaceplugin.Options

	GCVMGracefulShutdownTimeout    time.Duration
	ResyncIntervalGarbageCollector time.Duration
}

type HTTPServerOptions struct {
	Addr            string
	GracefulTimeout time.Duration
}

type ServersOptions struct {
	Metrics     HTTPServerOptions
	HealthCheck HTTPServerOptions
}

type LibvirtOptions struct {
	Socket  string
	Address string
	URI     string

	PreferredDomainTypes  []string
	PreferredMachineTypes []string

	Qcow2Type string
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Address, "address", "/var/run/iri-machinebroker.sock", "Address to listen on.")
	fs.StringVar(&o.RootDir, "libvirt-provider-dir", filepath.Join(homeDir, ".libvirt-provider"), "Path to the directory libvirt-provider manages its content at.")

	fs.StringVar(&o.PathSupportedMachineClasses, "supported-machine-classes", o.PathSupportedMachineClasses, "File containing supported machine classes.")
	fs.DurationVar(&o.ResyncIntervalVolumeSize, "volume-size-resync-interval", 1*time.Minute, "Interval to determine volume size changes.")

	fs.StringVar(&o.ApinetKubeconfig, "apinet-kubeconfig", "", "Path to the kubeconfig file for the apinet-cluster.")

	fs.StringVar(&o.StreamingAddress, "streaming-address", "127.0.0.1:20251", "Address to run the streaming server on")
	fs.StringVar(&o.BaseURL, "base-url", "", "The base url to construct urls for streaming from. If empty it will be "+
		"constructed from the streaming-address")

	fs.StringVar(&o.Servers.Metrics.Addr, "servers-metrics-address", "", "Address to listen on exposing of metrics. If address isn't set, server is disabled.")
	fs.DurationVar(&o.Servers.Metrics.GracefulTimeout, "servers-metrics-gracefultimeout", 2*time.Second, "Graceful timeout for shutdown metrics server.")

	fs.StringVar(&o.Servers.HealthCheck.Addr, "servers-health-check-address", "127.0.0.1:8080", "Address to listen on health check liveness.")
	fs.DurationVar(&o.Servers.HealthCheck.GracefulTimeout, "servers-health-check-gracefultimeout", 2*time.Second, "Graceful timeout for shutdown health check server.")

	fs.BoolVar(&o.EnableHugepages, "enable-hugepages", false, "Enable using Hugepages.")
	fs.Var(&o.GuestAgent, "guest-agent-type", fmt.Sprintf("Guest agent implementation to use. Available: %v", guestAgentOptionAvailable()))

	// LibvirtOptions
	fs.StringVar(&o.Libvirt.Socket, "libvirt-socket", o.Libvirt.Socket, "Path to the libvirt socket to use.")
	fs.StringVar(&o.Libvirt.Address, "libvirt-address", o.Libvirt.Address, "Address of a RPC libvirt socket to connect to.")
	fs.StringVar(&o.Libvirt.URI, "libvirt-uri", o.Libvirt.URI, "URI to connect to inside the libvirt system.")

	// Guest Capabilities
	fs.StringSliceVar(&o.Libvirt.PreferredDomainTypes, "preferred-domain-types", []string{"kvm", "qemu"}, "Ordered list of preferred domain types to use.")
	fs.StringSliceVar(&o.Libvirt.PreferredMachineTypes, "preferred-machine-types", []string{"pc-q35"}, "Ordered list of preferred machine types to use.")

	fs.StringVar(&o.Libvirt.Qcow2Type, "qcow2-type", qcow2.Default(), fmt.Sprintf("qcow2 implementation to use. Available: %v", qcow2.Available()))

	fs.DurationVar(&o.GCVMGracefulShutdownTimeout, "gc-vm-graceful-shutdown-timeout", 5*time.Minute, "Duration to wait for the VM to gracefully shut down. If the VM does not shut down within this period, it will be forcibly destroyed by garbage collector.")
	fs.DurationVar(&o.ResyncIntervalGarbageCollector, "gc-resync-interval", 1*time.Minute, "Interval for resynchronizing the garbage collector.")

	o.NicPlugin = networkinterfaceplugin.NewDefaultOptions()
	o.NicPlugin.AddFlags(fs)
}

func (o *Options) MarkFlagsRequired(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired("supported-machine-classes")
}

func Command() *cobra.Command {
	var (
		zapOpts = zap.Options{Development: true}
		opts    Options
	)

	cmd := &cobra.Command{
		Use: "libvirt-provider",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger := zap.New(zap.UseFlagOptions(&zapOpts))
			ctrl.SetLogger(logger)
			cmd.SetContext(ctrl.LoggerInto(cmd.Context(), ctrl.Log))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			//flag parsing is done therefore we can silence the usage message
			cmd.SilenceUsage = true
			//error logging is done in the main
			cmd.SilenceErrors = true
			return Run(cmd.Context(), opts)
		},
	}

	goFlags := goflag.NewFlagSet("", 0)
	zapOpts.BindFlags(goFlags)
	cmd.PersistentFlags().AddGoFlagSet(goFlags)

	opts.AddFlags(cmd.Flags())
	opts.MarkFlagsRequired(cmd)

	return cmd
}

func Run(ctx context.Context, opts Options) error {
	log := ctrl.LoggerFrom(ctx)
	setupLog := log.WithName("setup")

	// Setup Libvirt Client
	libvirt, err := libvirtutils.GetLibvirt(opts.Libvirt.Socket, opts.Libvirt.Address, opts.Libvirt.URI)
	if err != nil {
		setupLog.Error(err, "failed to initialize libvirt")
		return err
	}
	defer func() {
		if err := libvirt.ConnectClose(); err != nil {
			setupLog.Error(err, "failed to close libvirt connection")
		}
	}()

	baseURL := opts.BaseURL
	if baseURL == "" {
		u := &url.URL{
			Scheme: "http",
			Host:   opts.StreamingAddress,
		}
		baseURL = u.String()
	}

	// Check if apinetKubeconfig is provided
	var apinetClient client.Client
	if opts.ApinetKubeconfig != "" {
		apinetCfg, err := clientcmd.BuildConfigFromFlags("", opts.ApinetKubeconfig)
		if err != nil {
			setupLog.Error(err, "failed to create config from apinet-kubeconfig")
			return err
		}

		apinetClient, err = client.New(apinetCfg, client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "failed to initialize api-net client")
			return err
		}
	}

	providerHost, err := host.NewLibvirtAt(apinetClient, opts.RootDir, libvirt)
	if err != nil {
		setupLog.Error(err, "failed to initialize provider host")
		return err
	}

	reg, err := remote.DockerRegistry(nil)
	if err != nil {
		setupLog.Error(err, "failed to initialize registry")
		return err
	}

	imgCache, err := oci.NewLocalCache(log, reg, providerHost.OCIStore())
	if err != nil {
		setupLog.Error(err, "failed to initialize oci manager")
		return err
	}

	qcow2Inst, err := qcow2.Instance(opts.Libvirt.Qcow2Type)
	if err != nil {
		setupLog.Error(err, "failed to initialize qcow2 instance")
		return err
	}

	rawInst, err := raw.Instance(raw.Default())
	if err != nil {
		setupLog.Error(err, "failed to initialize raw instance")
		return err
	}

	// Detect Guest Capabilities
	caps, err := guest.DetectCapabilities(libvirt, guest.CapabilitiesOptions{
		PreferredDomainTypes:  opts.Libvirt.PreferredDomainTypes,
		PreferredMachineTypes: opts.Libvirt.PreferredMachineTypes,
	})
	if err != nil {
		setupLog.Error(err, "failed to detect guest capabilities")
		return err
	}

	volumePlugins := volumeplugin.NewPluginManager()
	if err := volumePlugins.InitPlugins(providerHost, []volumeplugin.Plugin{
		ceph.NewPlugin(),
		emptydisk.NewPlugin(qcow2Inst, rawInst),
	}); err != nil {
		setupLog.Error(err, "failed to initialize volume plugin manager")
		return err
	}

	nicPlugin, nicPluginCleanup, err := opts.NicPlugin.NetworkInterfacePlugin()
	if err != nil {
		setupLog.Error(err, "failed to initialize network plugin")
		return err
	}
	if nicPluginCleanup != nil {
		defer nicPluginCleanup()
	}

	setupLog.Info("Initializing network interface plugin")

	if err := nicPlugin.Init(providerHost); err != nil {
		setupLog.Error(err, "failed to initialize network plugin")
		return err
	}

	setupLog.Info("Configuring machine store", "Directory", providerHost.MachineStoreDir())
	machineStore, err := host.NewStore(host.Options[*api.Machine]{
		NewFunc:        func() *api.Machine { return &api.Machine{} },
		CreateStrategy: strategy.MachineStrategy,
		Dir:            providerHost.MachineStoreDir(),
	})
	if err != nil {
		setupLog.Error(err, "failed to initialize machine store")
		return err
	}

	machineEvents, err := event.NewListWatchSource[*api.Machine](
		machineStore.List,
		machineStore.Watch,
		event.ListWatchSourceOptions{},
	)
	if err != nil {
		setupLog.Error(err, "failed to initialize machine events")
		return err
	}

	machineReconciler, err := controllers.NewMachineReconciler(
		log.WithName("machine-reconciler"),
		libvirt,
		machineStore,
		machineEvents,
		controllers.MachineReconcilerOptions{
			GuestCapabilities:              caps,
			ImageCache:                     imgCache,
			Raw:                            rawInst,
			Host:                           providerHost,
			VolumePluginManager:            volumePlugins,
			NetworkInterfacePlugin:         nicPlugin,
			ResyncIntervalVolumeSize:       opts.ResyncIntervalVolumeSize,
			ResyncIntervalGarbageCollector: opts.ResyncIntervalGarbageCollector,
			EnableHugepages:                opts.EnableHugepages,
			GCVMGracefulShutdownTimeout:    opts.GCVMGracefulShutdownTimeout,
		},
	)
	if err != nil {
		setupLog.Error(err, "failed to initialize machine controller")
		return err
	}

	setupLog.V(1).Info("Loading machine classes", "Path", opts.PathSupportedMachineClasses)
	classes, err := mcr.LoadMachineClassesFile(opts.PathSupportedMachineClasses)
	if err != nil {
		setupLog.Error(err, "failed to load machine classes")
		return err
	}

	machineClasses, err := mcr.NewMachineClassRegistry(classes)
	if err != nil {
		setupLog.Error(err, "failed to initialize machine class registry")
		return err
	}

	srv, err := server.New(server.Options{
		BaseURL:         baseURL,
		Libvirt:         libvirt,
		MachineStore:    machineStore,
		MachineClasses:  machineClasses,
		VolumePlugins:   volumePlugins,
		NetworkPlugins:  nicPlugin,
		EnableHugepages: opts.EnableHugepages,
		GuestAgent:      opts.GuestAgent.GetAPIGuestAgent(),
	})
	if err != nil {
		setupLog.Error(err, "failed to initialize server")
		return err
	}

	healthCheck := healthcheck.HealthCheck{
		Libvirt: libvirt,
		Log:     log.WithName("health-check"),
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return runMetricsServer(ctx, setupLog, opts.Servers.Metrics)
	})

	g.Go(func() error {
		setupLog.Info("Starting oci cache")
		if err := imgCache.Start(ctx); err != nil {
			setupLog.Error(err, "failed to start oci cache")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting machine reconciler")
		if err := machineReconciler.Start(ctx); err != nil {
			setupLog.Error(err, "failed to start machine reconciler")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting machine events")
		if err := machineEvents.Start(ctx); err != nil {
			setupLog.Error(err, "failed to start machine events")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting grpc server")
		if err := runGRPCServer(ctx, setupLog, log, srv, opts); err != nil {
			setupLog.Error(err, "failed to start grpc server")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting streaming server")
		if err := runStreamingServer(ctx, setupLog, log, srv, opts); err != nil {
			setupLog.Error(err, "failed to start streaming server")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting health check server")
		if err := runHealthCheckServer(ctx, setupLog, healthCheck, opts.Servers.HealthCheck); err != nil {
			setupLog.Error(err, "failed to start health check server")
			return err
		}
		return nil
	})

	return g.Wait()
}

func runGRPCServer(ctx context.Context, setupLog logr.Logger, log logr.Logger, srv *server.Server, opts Options) error {
	setupLog.V(1).Info("Cleaning up any previous socket")
	if err := common.CleanupSocketIfExists(opts.Address); err != nil {
		return fmt.Errorf("error cleaning up socket: %w", err)
	}

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			commongrpc.InjectLogger(log.WithName("iri-server")),
			commongrpc.LogRequest,
		),
	)
	iri.RegisterMachineRuntimeServer(grpcSrv, srv)

	setupLog.V(1).Info("Start listening on unix socket", "Address", opts.Address)
	l, err := net.Listen("unix", opts.Address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	setupLog.Info("Starting grpc server", "Address", l.Addr().String())
	go func() {
		<-ctx.Done()
		setupLog.Info("Shutting down grpc server")
		grpcSrv.GracefulStop()
		setupLog.Info("Shut down grpc server")
	}()
	if err := grpcSrv.Serve(l); err != nil {
		return fmt.Errorf("error serving grpc: %w", err)
	}
	return nil
}

func runStreamingServer(ctx context.Context, setupLog, log logr.Logger, srv *server.Server, opts Options) error {
	httpHandler := console.NewHandler(srv, console.HandlerOptions{
		Log: log.WithName("streaming-server"),
	})

	httpSrv := &http.Server{
		Addr:    opts.StreamingAddress,
		Handler: httpHandler,
	}

	go func() {
		<-ctx.Done()
		setupLog.Info("Shutting down streaming server")
		_ = httpSrv.Close()
		setupLog.Info("Shut down streaming server")
	}()

	setupLog.V(1).Info("Starting streaming server", "Address", opts.StreamingAddress)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error listening / serving streaming server: %w", err)
	}
	return nil
}

func runMetricsServer(ctx context.Context, setupLog logr.Logger, opts HTTPServerOptions) error {
	if opts.Addr == "" {
		setupLog.Info("Metrics server address isn't configured. Metrics server is disabled.")
		return nil
	}

	setupLog.Info("Starting metrics server on " + opts.Addr)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := http.Server{
		Addr:    opts.Addr,
		Handler: mux,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		setupLog.Info("Shutting down metrics server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.GracefulTimeout)
		defer cancel()
		locErr := srv.Shutdown(shutdownCtx)
		if locErr != nil {
			setupLog.Error(locErr, "metrics server wasn't shutdown properly")
		} else {
			setupLog.Info("Metrics server is shutdown")
		}
	}()

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error listening / serving metrics server: %w", err)
	}

	setupLog.Info("Metrics server stopped serve new connections")

	wg.Wait()

	return nil
}

func runHealthCheckServer(ctx context.Context, setupLog logr.Logger, healthCheck healthcheck.HealthCheck, opts HTTPServerOptions) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheck.HealthCheckHandler)

	srv := http.Server{
		Addr:    opts.Addr,
		Handler: mux,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		setupLog.Info("Shutting down health check server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.GracefulTimeout)
		defer cancel()
		locErr := srv.Shutdown(shutdownCtx)
		if locErr != nil {
			setupLog.Error(locErr, "health checkserver wasn't shutdown properly")
		} else {
			setupLog.Info("Health check server is shutdown")
		}
	}()

	setupLog.V(1).Info("Starting health check server", "Address", opts.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error listening / serving health check server: %w", err)
	}

	wg.Wait()

	return nil
}
