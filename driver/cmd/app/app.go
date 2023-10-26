// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	goflag "flag"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/onmetal/libvirt-driver/driver/server"
	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/controllers"
	"github.com/onmetal/libvirt-driver/pkg/event"
	"github.com/onmetal/libvirt-driver/pkg/host"
	"github.com/onmetal/libvirt-driver/pkg/libvirt/guest"
	libvirtutils "github.com/onmetal/libvirt-driver/pkg/libvirt/utils"
	"github.com/onmetal/libvirt-driver/pkg/mcr"
	volumeplugin "github.com/onmetal/libvirt-driver/pkg/plugins/volume"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume/ceph"
	"github.com/onmetal/libvirt-driver/pkg/plugins/volume/emptydisk"
	"github.com/onmetal/libvirt-driver/pkg/qcow2"
	"github.com/onmetal/libvirt-driver/pkg/raw"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	"github.com/onmetal/onmetal-api/broker/common"
	commongrpc "github.com/onmetal/onmetal-api/broker/common/grpc"
	ori "github.com/onmetal/onmetal-api/ori/apis/machine/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	homeDir string
)

func init() {
	homeDir, _ = os.UserHomeDir()
}

type Options struct {
	Address string
	RootDir string
	Libvirt LibvirtOptions
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
	fs.StringVar(&o.Address, "address", "/var/run/ori-machinebroker.sock", "Address to listen on.")
	fs.StringVar(&o.RootDir, "virtlet-dir", filepath.Join(homeDir, ".virtlet"), "Path to the directory virtlet manages its content at.")

	// LibvirtOptions
	fs.StringVar(&o.Libvirt.Socket, "libvirt-socket", o.Libvirt.Socket, "Path to the libvirt socket to use.")
	fs.StringVar(&o.Libvirt.Address, "libvirt-address", o.Libvirt.Address, "Address of a RPC libvirt socket to connect to.")
	fs.StringVar(&o.Libvirt.URI, "libvirt-uri", o.Libvirt.URI, "URI to connect to inside the libvirt system.")

	// Guest Capabilities
	fs.StringSliceVar(&o.Libvirt.PreferredDomainTypes, "preferred-domain-types", []string{"kvm", "qemu"}, "Ordered list of preferred domain types to use.")
	fs.StringSliceVar(&o.Libvirt.PreferredMachineTypes, "preferred-machine-types", []string{"pc-q35"}, "Ordered list of preferred machine types to use.")

	fs.StringVar(&o.Libvirt.Qcow2Type, "qcow2-type", qcow2.Default(), fmt.Sprintf("qcow2 implementation to use. Available: %v", qcow2.Available()))
}

func Command() *cobra.Command {
	var (
		zapOpts = zap.Options{Development: true}
		opts    Options
	)

	cmd := &cobra.Command{
		Use: "machinebroker",
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

	return cmd
}

func Run(ctx context.Context, opts Options) error {
	log := ctrl.LoggerFrom(ctx)
	setupLog := log.WithName("setup")

	// Setup Libvirt Client
	libvirt, err := libvirtutils.GetLibvirt(opts.Libvirt.Socket, opts.Libvirt.Address, opts.Libvirt.URI)
	if err != nil {
		setupLog.Error(err, "error getting libvirt")
		os.Exit(1)
	}
	defer func() {
		if err := libvirt.ConnectClose(); err != nil {
			setupLog.Error(err, "Error closing libvirt connection")
		}
	}()

	// Detect Guest Capabilities
	caps, err := guest.DetectCapabilities(libvirt, guest.CapabilitiesOptions{
		PreferredDomainTypes:  opts.Libvirt.PreferredDomainTypes,
		PreferredMachineTypes: opts.Libvirt.PreferredMachineTypes,
	})
	if err != nil {
		setupLog.Error(err, "error detecting guest capabilities")
		os.Exit(1)
	}

	volumeStoreDir := filepath.Join(opts.RootDir, "volumes")
	setupLog.Info("Configuring volume store", "Directory", volumeStoreDir)
	volumeStore, err := host.NewStore(host.Options[*api.Volume]{
		NewFunc:        func() *api.Volume { return &api.Volume{} },
		CreateStrategy: utils.VolumeStrategy,
		Dir:            volumeStoreDir,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize volume store: %w", err)
	}

	volumeEvents, err := event.NewListWatchSource[*api.Volume](
		volumeStore.List,
		volumeStore.Watch,
		event.ListWatchSourceOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize volume events: %w", err)
	}

	_ = volumeEvents

	machineStoreDir := filepath.Join(opts.RootDir, "machines")
	setupLog.Info("Configuring machine store", "Directory", machineStoreDir)
	machineStore, err := host.NewStore(host.Options[*api.Machine]{
		NewFunc:        func() *api.Machine { return &api.Machine{} },
		CreateStrategy: utils.MachineStrategy,
		Dir:            machineStoreDir,
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
		volumeStore,
		controllers.MachineReconcilerOptions{
			GuestCapabilities: caps,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize machine controller: %w", err)
	}

	machineClasses, err := mcr.NewMachineClassRegistry([]ori.MachineClass{
		{
			Name: "x3-xlarge",
			Capabilities: &ori.MachineClassCapabilities{
				CpuMillis:   4000,
				MemoryBytes: 8589934592,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize machine class registry: %w", err)
	}

	qcow2Inst, err := qcow2.Instance(opts.Libvirt.Qcow2Type)
	if err != nil {
		setupLog.Error(err, "error creating qcow2 instance")
		os.Exit(1)
	}

	rawInst, err := raw.Instance(raw.Default())
	if err != nil {
		setupLog.Error(err, "error creating raw instance")
		os.Exit(1)
	}

	volumePlugins := volumeplugin.NewPluginManager()
	if err := volumePlugins.InitPlugins([]volumeplugin.Plugin{
		ceph.NewPlugin(),
		emptydisk.NewPlugin(qcow2Inst, rawInst),
	}); err != nil {
		return fmt.Errorf("failed to initialize machine class registry: %w", err)
	}

	srv, err := server.New(server.Options{
		MachineStore:   machineStore,
		VolumeStore:    volumeStore,
		MachineClasses: machineClasses,
		VolumePlugins:  volumePlugins,
	})
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

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
		}
		return nil
	})

	g.Go(func() error {
		setupLog.Info("Starting grpc server")
		return runGRPCServer(ctx, setupLog, log, srv, opts)
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
	ori.RegisterMachineRuntimeServer(grpcSrv, srv)

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
