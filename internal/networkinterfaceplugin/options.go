// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"

	networkingv1alpha1 "github.com/ironcore-dev/ironcore/api/networking/v1alpha1"
	utilcertificate "github.com/ironcore-dev/ironcore/utils/certificate"
	"github.com/ironcore-dev/ironcore/utils/client/config"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/apinet"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/isolated"
	"github.com/ironcore-dev/provider-utils/plugin"
	"github.com/spf13/pflag"
	certificatesv1 "k8s.io/api/certificates/v1"
)

const (
	PluginNameAPInet   = "apinet"
	PluginNameIsolated = "isolated"

	apinetFieldOwner     = "networking.ironcore.dev/libvirt-provider"
	apinetMachineIDLabel = "libvirt-provider.ironcore.dev/machine-id"
)

// Options select and configure the network interface plugin.
type Options struct {
	PluginName string

	APInetNodeName   string
	GetConfigOptions config.GetConfigOptions
}

// Deps are the provider callbacks handed to the plugin.
type Deps struct {
	// MachineExists reports whether the provider knows the machine. Optional.
	MachineExists func(machineID string) (bool, error)
	// EnqueueMachine requests a reconcile of the machine with the given ID,
	// e.g. after one of its network interfaces changed. Plugins must only
	// call it from their background process (see the start interface), not
	// during New. Optional.
	EnqueueMachine func(machineID string)
}

func NewDefaultOptions() *Options {
	return &Options{PluginName: PluginNameAPInet}
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.PluginName, "network-interface-plugin-name", o.PluginName,
		fmt.Sprintf("Name of the network interface plugin to use. Available: [%s %s]", PluginNameAPInet, PluginNameIsolated))
	fs.StringVar(&o.APInetNodeName, "apinet-node-name", "", "APInet node name")
	o.GetConfigOptions.BindFlags(fs, config.WithNamePrefix("apinet-"))
}

// New creates the selected network interface plugin. The returned config
// controller is nil unless the plugin needs one; if set it must be started by
// the caller. Plugins that need a background process additionally implement
// the start interface the caller checks for.
func (o *Options) New(ctx context.Context, deps Deps) (plugin.NetworkInterface, config.Controller, error) {
	switch o.PluginName {
	case PluginNameAPInet:
		return o.newAPInet(ctx, deps)
	case PluginNameIsolated:
		return isolated.New(), nil, nil
	default:
		return nil, nil, fmt.Errorf("unknown network interface plugin %q", o.PluginName)
	}
}

func (o *Options) newAPInet(ctx context.Context, deps Deps) (plugin.NetworkInterface, config.Controller, error) {
	if o.APInetNodeName == "" {
		return nil, nil, fmt.Errorf("must specify apinet-node-name")
	}

	getter, err := config.NewGetter(config.GetterOptions{
		Name:       "libvirt-provider-apinet-plugin",
		SignerName: certificatesv1.KubeAPIServerClientSignerName,
		Template: &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   networkingv1alpha1.NetworkPluginUserNamePrefix + o.APInetNodeName,
				Organization: []string{networkingv1alpha1.NetworkPluginsGroup},
			},
		},
		GetUsages: utilcertificate.DefaultKubeAPIServerClientGetUsages,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error creating getter: %w", err)
	}

	cfg, configCtrl, err := getter.GetConfig(ctx, &o.GetConfigOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting apinet config: %w", err)
	}

	p, err := apinet.New(apinet.Options{
		NodeName:       o.APInetNodeName,
		RestConfig:     cfg,
		FieldOwner:     apinetFieldOwner,
		MachineIDLabel: apinetMachineIDLabel,
		MachineExists:  deps.MachineExists,
		EnqueueMachine: deps.EnqueueMachine,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error creating apinet plugin: %w", err)
	}
	return p, configCtrl, nil
}
