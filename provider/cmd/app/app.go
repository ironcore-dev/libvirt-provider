// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"flag"
	goflag "flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/ironcore-image/oci/remote"
	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	"github.com/ironcore-dev/ironcore/broker/common"
	commongrpc "github.com/ironcore-dev/ironcore/broker/common/grpc"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	"github.com/ironcore-dev/libvirt-provider/pkg/controllers"
	"github.com/ironcore-dev/libvirt-provider/pkg/event"
	"github.com/ironcore-dev/libvirt-provider/pkg/host"
	providerimage "github.com/ironcore-dev/libvirt-provider/pkg/image"
	"github.com/ironcore-dev/libvirt-provider/pkg/libvirt/guest"
	libvirtutils "github.com/ironcore-dev/libvirt-provider/pkg/libvirt/utils"
	"github.com/ironcore-dev/libvirt-provider/pkg/mcr"
	volumeplugin "github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume/ceph"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/volume/emptydisk"
	providerhost "github.com/ironcore-dev/libvirt-provider/pkg/providerhost"
	"github.com/ironcore-dev/libvirt-provider/pkg/qcow2"
	"github.com/ironcore-dev/libvirt-provider/pkg/raw"
	"github.com/ironcore-dev/libvirt-provider/pkg/utils"

	providerhttp "github.com/ironcore-dev/libvirt-provider/provider/http"
	"github.com/ironcore-dev/libvirt-provider/provider/networkinterfaceplugin"
	"github.com/ironcore-dev/libvirt-provider/provider/server"
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
	homeDir         string
	scheme          = runtime.NewScheme()
	virshExecutable string
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

	RootDir string

	PathSupportedMachineClasses string
	ResyncDurationVolumeSize    time.Duration

	ApinetKubeconfig string

	Libvirt   LibvirtOptions
	NicPlugin *networkinterfaceplugin.Options
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
	fs.DurationVar(&o.ResyncDurationVolumeSize, "volume-size-resync-duration", o.ResyncDurationVolumeSize, "Interval to determine volume size changes.")

	fs.StringVar(&o.ApinetKubeconfig, "apinet-kubeconfig", "", "Path to the kubeconfig file for the apinet-cluster.")

	fs.StringVar(&o.StreamingAddress, "streaming-address", "127.0.0.1:20251", "Address to run the streaming server on")
	fs.StringVar(&o.BaseURL, "base-url", "", "The base url to construct urls for streaming from. If empty it will be "+
		"constructed from the streaming-address")

	flag.StringVar(&virshExecutable, "virsh-executable", "virsh", "Path / name of the virsh executable.")

	// LibvirtOptions
	fs.StringVar(&o.Libvirt.Socket, "libvirt-socket", o.Libvirt.Socket, "Path to the libvirt socket to use.")
	fs.StringVar(&o.Libvirt.Address, "libvirt-address", o.Libvirt.Address, "Address of a RPC libvirt socket to connect to.")
	fs.StringVar(&o.Libvirt.URI, "libvirt-uri", o.Libvirt.URI, "URI to connect to inside the libvirt system.")

	// Guest Capabilities
	fs.StringSliceVar(&o.Libvirt.PreferredDomainTypes, "preferred-domain-types", []string{"kvm", "qemu"}, "Ordered list of preferred domain types to use.")
	fs.StringSliceVar(&o.Libvirt.PreferredMachineTypes, "preferred-machine-types", []string{"pc-q35"}, "Ordered list of preferred machine types to use.")

	fs.StringVar(&o.Libvirt.Qcow2Type, "qcow2-type", qcow2.Default(), fmt.Sprintf("qcow2 implementation to use. Available: %v", qcow2.Available()))

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
		setupLog.Error(err, "error getting libvirt")
		return err
	}
	defer func() {
		if err := libvirt.ConnectClose(); err != nil {
			setupLog.Error(err, "Error closing libvirt connection")
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
			setupLog.Error(err, "Failed to build config from apinet-kubeconfig")
			return err
		}

		apinetClient, err = client.New(apinetCfg, client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "Error creating api-net client:")
			return err
		}
	}

	providerHost, err := providerhost.NewLibvirtAt(apinetClient, opts.RootDir, libvirt)
	if err != nil {
		setupLog.Error(err, "error creating provider host")
		return err
	}

	reg, err := remote.DockerRegistry(nil)
	if err != nil {
		setupLog.Error(err, "error creating registry")
		return err
	}

	imgCache, err := providerimage.NewLocalCache(log, reg, providerHost.OCIStore())
	if err != nil {
		setupLog.Error(err, "error setting up image manager")
		return err
	}

	qcow2Inst, err := qcow2.Instance(opts.Libvirt.Qcow2Type)
	if err != nil {
		setupLog.Error(err, "error creating qcow2 instance")
		return err
	}

	rawInst, err := raw.Instance(raw.Default())
	if err != nil {
		setupLog.Error(err, "error creating raw instance")
		return err
	}

	// Detect Guest Capabilities
	caps, err := guest.DetectCapabilities(libvirt, guest.CapabilitiesOptions{
		PreferredDomainTypes:  opts.Libvirt.PreferredDomainTypes,
		PreferredMachineTypes: opts.Libvirt.PreferredMachineTypes,
	})
	if err != nil {
		setupLog.Error(err, "error detecting guest capabilities")
		return err
	}

	volumePlugins := volumeplugin.NewPluginManager()
	if err := volumePlugins.InitPlugins(providerHost, []volumeplugin.Plugin{
		ceph.NewPlugin(),
		emptydisk.NewPlugin(qcow2Inst, rawInst),
	}); err != nil {
		return fmt.Errorf("failed to initialize machine class registry: %w", err)
	}

	nicPlugin, nicPluginCleanup, err := opts.NicPlugin.NetworkInterfacePlugin()
	if err != nil {
		setupLog.Error(err, "Error creating network plugin")
		return err
	}
	if nicPluginCleanup != nil {
		defer nicPluginCleanup()
	}

	setupLog.Info("Initializing network interface plugin")

	if err := nicPlugin.Init(providerHost); err != nil {
		setupLog.Error(err, "Error initializing network plugin")
		return err
	}

	setupLog.Info("Configuring machine store", "Directory", providerHost.MachineStoreDir())
	machineStore, err := host.NewStore(host.Options[*api.Machine]{
		NewFunc:        func() *api.Machine { return &api.Machine{} },
		CreateStrategy: utils.MachineStrategy,
		Dir:            providerHost.MachineStoreDir(),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize machine store: %w", err)
	}

	machineEvents, err := event.NewListWatchSource[*api.Machine](
		machineStore.List,
		machineStore.Watch,
		event.ListWatchSourceOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize machine events: %w", err)
	}

	machineReconciler, err := controllers.NewMachineReconciler(
		log.WithName("machine-reconciler"),
		libvirt,
		machineStore,
		machineEvents,
		controllers.MachineReconcilerOptions{
			GuestCapabilities:        caps,
			ImageCache:               imgCache,
			Raw:                      rawInst,
			Host:                     providerHost,
			VolumePluginManager:      volumePlugins,
			NetworkInterfacePlugin:   nicPlugin,
			ResyncDurationVolumeSize: opts.ResyncDurationVolumeSize,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize machine controller: %w", err)
	}

	setupLog.V(1).Info("Loading machine classes", "Path", opts.PathSupportedMachineClasses)
	classes, err := mcr.LoadMachineClassesFile(opts.PathSupportedMachineClasses)
	if err != nil {
		return fmt.Errorf("failed to initialize machine class registry: %w", err)
	}

	machineClasses, err := mcr.NewMachineClassRegistry(classes)
	if err != nil {
		return fmt.Errorf("failed to initialize machine class registry: %w", err)
	}

	srv, err := server.New(server.Options{
		BaseURL:         baseURL,
		Libvirt:         libvirt,
		MachineStore:    machineStore,
		MachineClasses:  machineClasses,
		VolumePlugins:   volumePlugins,
		VirshExecutable: virshExecutable,
	})
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		setupLog.Info("Starting image cache")
		if err := imgCache.Start(ctx); err != nil {
			log.Error(err, "failed to start image cache")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting machine reconciler")
		if err := machineReconciler.Start(ctx); err != nil {
			log.Error(err, "failed to start machine reconciler")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting machine events")
		if err := machineEvents.Start(ctx); err != nil {
			log.Error(err, "failed to start machine events")
			return err
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting grpc server")
		return runGRPCServer(ctx, setupLog, log, srv, opts)
	})

	g.Go(func() error {
		setupLog.Info("Starting streaming server")
		return runStreamingServer(ctx, setupLog, log, srv, opts)
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
			commongrpc.InjectLogger(log),
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
	httpHandler := providerhttp.NewHandler(srv, providerhttp.HandlerOptions{
		Log: log.WithName("server"),
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

	log.V(1).Info("Starting streaming server", "Address", opts.StreamingAddress)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error listening / serving streaming server: %w", err)
	}
	return nil
}
