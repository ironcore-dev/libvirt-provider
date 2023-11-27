// Copyright 2022 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package networkinterfaceplugin

import (
	virtletnetworkinterface "github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface"
	"github.com/ironcore-dev/libvirt-provider/pkg/plugins/networkinterface/isolated"
	"github.com/spf13/pflag"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type isolatedOptions struct{}

func (o *isolatedOptions) AddFlags(fs *pflag.FlagSet) {}

func (o *isolatedOptions) PluginName() string {
	return "isolated"
}

func (o *isolatedOptions) NetworkInterfacePlugin() (virtletnetworkinterface.Plugin, func(), error) {
	return isolated.NewPlugin(), nil, nil
}

func init() {
	utilruntime.Must(DefaultPluginTypeRegistry.Register(&isolatedOptions{}, 5))
}
