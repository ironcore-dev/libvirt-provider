// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"time"

	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	networkingv1alpha1 "github.com/ironcore-dev/ironcore/api/networking/v1alpha1"
	utilcertificate "github.com/ironcore-dev/ironcore/utils/certificate"
	"github.com/ironcore-dev/ironcore/utils/client/config"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/apinet"
	"github.com/spf13/pflag"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apinetv1alpha1.AddToScheme(scheme))
}

type apinetOptions struct {
	APInetNodeName  string
	PollingDuration time.Duration
	PollingInterval time.Duration

	GetConfigOptions config.GetConfigOptions
}

func (o *apinetOptions) PluginName() string {
	return "apinet"
}

func (o *apinetOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.APInetNodeName, "apinet-node-name", "", "APInet node name")
	fs.DurationVar(&o.PollingDuration, "apinet-polling-duration", 30*time.Second, "Duration to poll for apinet network interface readiness")
	fs.DurationVar(&o.PollingInterval, "apinet-polling-interval", 1*time.Second, "Interval between apinet network interface readiness polls")
	o.GetConfigOptions.BindFlags(fs, config.WithNamePrefix("apinet-"))
}

func (o *apinetOptions) NetworkInterfacePlugin(ctx context.Context) (
	providernetworkinterface.Plugin,
	config.Controller,
	func(),
	error,
) {
	if o.APInetNodeName == "" {
		return nil, nil, nil, fmt.Errorf("must specify apinet-node-name")
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
		return nil, nil, nil, fmt.Errorf("error creating getter: %w", err)
	}

	cfg, configCtrl, err := getter.GetConfig(ctx, &o.GetConfigOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error getting apinet config: %w", err)
	}

	apinetClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating apinet client: %w", err)
	}

	return apinet.NewPlugin(o.APInetNodeName, apinetClient, o.PollingDuration, o.PollingInterval), configCtrl, nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&apinetOptions{}, 1))
}
