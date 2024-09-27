// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package networkinterfaceplugin

import (
	"fmt"

	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface/apinet"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apinetv1alpha1.AddToScheme(scheme))
}

type apinetOptions struct {
	APInetNodeName   string
	ApinetKubeconfig string
}

func (o *apinetOptions) PluginName() string {
	return "apinet"
}

func (o *apinetOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.APInetNodeName, "apinet-node-name", "", "APInet node name")
	fs.StringVar(&o.ApinetKubeconfig, "apinet-kubeconfig", "", "Path to the kubeconfig file for the apinet-cluster.")
}

func (o *apinetOptions) NetworkInterfacePlugin() (providernetworkinterface.Plugin, func(), error) {
	if o.APInetNodeName == "" {
		return nil, nil, fmt.Errorf("must specify apinet-node-name")
	}

	// Check if apinetKubeconfig is provided
	var apinetCfg *rest.Config
	var err error
	if o.ApinetKubeconfig != "" {
		apinetCfg, err = clientcmd.BuildConfigFromFlags("", o.ApinetKubeconfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create config from apinet-kubeconfig: %w", err)
		}
	} else {
		// assuming in-cluster config
		apinetCfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create apinet in-cluster-config: %w", err)
		}
	}

	apinetClient, err := client.New(apinetCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize api-net client: %w", err)
	}

	return apinet.NewPlugin(o.APInetNodeName, apinetClient), nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&apinetOptions{}, 1))
}
