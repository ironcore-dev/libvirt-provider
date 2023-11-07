// Copyright 2023 OnMetal authors
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

package apinet

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/onmetal/libvirt-driver/pkg/api"
	virtletnetworkinterface "github.com/onmetal/libvirt-driver/pkg/plugins/networkinterface"
	virtlethost "github.com/onmetal/libvirt-driver/pkg/virtlethost"
	apinetv1alpha1 "github.com/onmetal/onmetal-api-net/api/core/v1alpha1"
	"github.com/onmetal/onmetal-api-net/apimachinery/api/net"
	"github.com/onmetal/onmetal-api-net/apinetlet/provider"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("networking.api.onmetal.de/libvirt-driver")

	defaultAPINetConfigFile = "api-net.json"

	perm         = 0777
	filePerm     = 0666
	pluginAPInet = "apinet"
)

type Plugin struct {
	nodeName string
	host     virtlethost.Host
}

func NewPlugin(nodeName string) virtletnetworkinterface.Plugin {
	return &Plugin{
		nodeName: nodeName,
	}
}

func GetAPInetPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(host virtlethost.Host) error {
	p.host = host
	return nil
}

func onmetalIPsToAPInetIPs(ips []string) []net.IP {
	res := make([]net.IP, len(ips))
	for i, ip := range ips {
		res[i] = net.MustParseIP(ip)
	}
	return res
}

type apiNetNetworkInterfaceConfig struct {
	Namespace string `json:"namespace"`
}

func (p *Plugin) apiNetNetworkInterfaceConfigFile(machineID string, networkInterfaceName string) string {
	return filepath.Join(p.host.MachineNetworkInterfaceDir(machineID, networkInterfaceName), defaultAPINetConfigFile)
}

func (p *Plugin) writeAPINetNetworkInterfaceConfig(machineID string, networkInterfaceName string, cfg *apiNetNetworkInterfaceConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(p.apiNetNetworkInterfaceConfigFile(machineID, networkInterfaceName), data, filePerm)
}

func (p *Plugin) readAPINetNetworkInterfaceConfig(machineID string, networkInterfaceName string) (*apiNetNetworkInterfaceConfig, error) {
	data, err := os.ReadFile(p.apiNetNetworkInterfaceConfigFile(machineID, networkInterfaceName))
	if err != nil {
		return nil, err
	}

	cfg := &apiNetNetworkInterfaceConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (p *Plugin) APInetNicName(machineID string, networkInterfaceName string) string {
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("%s/%s", machineID, networkInterfaceName)), 5).String()
}

func (p *Plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*virtletnetworkinterface.NetworkInterface, error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(1).Info("Writing network interface dir")
	if err := os.MkdirAll(p.host.MachineNetworkInterfaceDir(machine.ID, spec.Name), perm); err != nil {
		return nil, err
	}

	apinetNamespace, apinetNetworkName, _, _, err := provider.ParseNetworkID(spec.NetworkId)
	if err != nil {
		return nil, fmt.Errorf("error parsing ApiNet NetworkID %s: %w", spec.NetworkId, err)
	}

	log.V(1).Info("Writing APINet network interface config file")
	if err := p.writeAPINetNetworkInterfaceConfig(machine.ID, spec.Name, &apiNetNetworkInterfaceConfig{
		Namespace: apinetNamespace,
	}); err != nil {
		return nil, err
	}

	apinetNic := &apinetv1alpha1.NetworkInterface{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apinetv1alpha1.SchemeGroupVersion.String(),
			Kind:       "NetworkInterface",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: apinetNamespace,
			Name:      p.APInetNicName(machine.ID, spec.Name),
		},
		Spec: apinetv1alpha1.NetworkInterfaceSpec{
			NetworkRef: corev1.LocalObjectReference{
				Name: apinetNetworkName,
			},
			NodeRef: corev1.LocalObjectReference{
				Name: p.nodeName,
			},
			IPs: onmetalIPsToAPInetIPs(spec.Ips),
		},
	}

	log.V(1).Info("Applying apinet nic")
	if err := p.host.APINetClient().Patch(ctx, apinetNic, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return nil, fmt.Errorf("error applying apinet network interface: %w", err)
	}

	hostDev, err := getHostDevice(apinetNic)
	if err != nil {
		return nil, fmt.Errorf("error getting host device: %w", err)
	}
	if hostDev != nil {
		log.V(1).Info("Host device is ready", "HostDevice", hostDev)
		return &virtletnetworkinterface.NetworkInterface{
			Handle: provider.GetNetworkInterfaceID(
				apinetNic.Namespace,
				apinetNic.Name,
				apinetNic.Spec.NodeRef.Name,
				apinetNic.UID,
			),
			HostDevice: hostDev,
		}, nil
	}

	log.V(1).Info("Waiting for apinet network interface to become ready")
	apinetNicKey := client.ObjectKeyFromObject(apinetNic)
	if err := wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (done bool, err error) {
		if err := p.host.APINetClient().Get(ctx, apinetNicKey, apinetNic); err != nil {
			return false, fmt.Errorf("error getting apinet nic %s: %w", apinetNicKey, err)
		}

		hostDev, err = getHostDevice(apinetNic)
		if err != nil {
			return false, fmt.Errorf("error getting host device: %w", err)
		}
		return hostDev != nil, nil
	}); err != nil {
		return nil, fmt.Errorf("error waiting for nic to become ready: %w", err)
	}

	// Fetch the updated object to get the ID or any other updated fields
	if err := p.host.APINetClient().Get(ctx, apinetNicKey, apinetNic); err != nil {
		return nil, fmt.Errorf("error fetching updated apinet network interface: %w", err)
	}

	log.V(1).Info("Host device is ready", "HostDevice", hostDev)
	return &virtletnetworkinterface.NetworkInterface{
		Handle: provider.GetNetworkInterfaceID(
			apinetNic.Namespace,
			apinetNic.Name,
			apinetNic.Spec.NodeRef.Name,
			apinetNic.UID,
		),
		HostDevice: hostDev,
	}, nil
}

func getHostDevice(apinetNic *apinetv1alpha1.NetworkInterface) (*virtletnetworkinterface.HostDevice, error) {
	switch apinetNic.Status.State {
	case apinetv1alpha1.NetworkInterfaceStateReady:
		pciDevice := apinetNic.Status.PCIAddress
		if pciDevice == nil {
			return nil, nil
		}

		domain, err := strconv.ParseUint(pciDevice.Domain, 16, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("error parsing pci device domain %q: %w", pciDevice.Domain, err)
		}

		bus, err := strconv.ParseUint(pciDevice.Bus, 16, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("error parsing pci device bus %q: %w", pciDevice.Bus, err)
		}

		slot, err := strconv.ParseUint(pciDevice.Slot, 16, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("error parsing pci device slot %q: %w", pciDevice.Slot, err)
		}

		function, err := strconv.ParseUint(pciDevice.Function, 16, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("error parsing pci device function %q: %w", pciDevice.Function, err)
		}

		return &virtletnetworkinterface.HostDevice{
			Domain:   uint(domain),
			Bus:      uint(bus),
			Slot:     uint(slot),
			Function: uint(function),
		}, nil
	case apinetv1alpha1.NetworkInterfaceStatePending:
		return nil, nil
	case apinetv1alpha1.NetworkInterfaceStateError:
		return nil, fmt.Errorf("apinet network interface is in state error")
	default:
		return nil, nil
	}
}

func (p *Plugin) Delete(ctx context.Context, computeNicName string, machineID string) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(1).Info("Reading APINet network interface config file")
	cfg, err := p.readAPINetNetworkInterfaceConfig(machineID, computeNicName)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("error reading namespace file: %w", err)
		}

		log.V(1).Info("No namespace file found, deleting network interface dir")
		return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
	}

	apinetNicKey := client.ObjectKey{
		Namespace: cfg.Namespace,
		Name:      p.APInetNicName(machineID, computeNicName),
	}
	log = log.WithValues("APInetNetworkInterfaceKey", apinetNicKey)

	if err := p.host.APINetClient().Delete(ctx, &apinetv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: apinetNicKey.Namespace,
			Name:      apinetNicKey.Name,
		},
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("error deleting apinet network interface %s: %w", apinetNicKey, err)
		}

		log.V(1).Info("APInet network interface is already gone, removing network interface directory")
		return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
	}

	log.V(1).Info("Waiting until apinet network interface is gone")
	if err := wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (done bool, err error) {
		if err := p.host.APINetClient().Get(ctx, apinetNicKey, &apinetv1alpha1.NetworkInterface{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("error getting apinet network interface %s: %w", apinetNicKey, err)
			}
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for apinet network interface %s to be gone: %w", apinetNicKey, err)
	}

	log.V(1).Info("APInet network interface is gone, removing network interface dir")
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

func (p *Plugin) Name() string {
	return pluginAPInet
}
