// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

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

	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	apinet "github.com/ironcore-dev/ironcore-net/apimachinery/api/net"
	"github.com/ironcore-dev/ironcore-net/apinetlet/provider"
	"github.com/ironcore-dev/libvirt-provider/api"
	providerhost "github.com/ironcore-dev/libvirt-provider/internal/host"
	providernetworkinterface "github.com/ironcore-dev/libvirt-provider/internal/plugins/networkinterface"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("networking.ironcore.dev/libvirt-provider")

	defaultAPINetConfigFile = "api-net.json"

	perm         = 0o777
	filePerm     = 0o666
	pluginAPInet = "apinet"
)

type Plugin struct {
	nodeName        string
	host            providerhost.LibvirtHost
	apinetClient    client.Client
	pollingInterval time.Duration
	pollingDuration time.Duration
}

func NewPlugin(nodeName string, client client.Client, duration, interval time.Duration) providernetworkinterface.Plugin {
	return &Plugin{
		nodeName:        nodeName,
		apinetClient:    client,
		pollingDuration: duration,
		pollingInterval: interval,
	}
}

func GetAPInetPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(host providerhost.LibvirtHost) error {
	p.host = host
	return nil
}

func ironcoreIPsToAPInetIPs(ips []string) []apinet.IP {
	res := make([]apinet.IP, len(ips))
	for i, ip := range ips {
		res[i] = apinet.MustParseIP(ip)
	}
	return res
}

type apiNetNetworkInterfaceConfig struct {
	Namespace string `json:"namespace"`
}

func (p *Plugin) apiNetNetworkInterfaceConfigFile(machineID, networkInterfaceName string) string {
	return filepath.Join(p.host.MachineNetworkInterfaceDir(machineID, networkInterfaceName), defaultAPINetConfigFile)
}

func (p *Plugin) writeAPINetNetworkInterfaceConfig(machineID, networkInterfaceName string, cfg *apiNetNetworkInterfaceConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(p.apiNetNetworkInterfaceConfigFile(machineID, networkInterfaceName), data, filePerm)
}

func (p *Plugin) readAPINetNetworkInterfaceConfig(machineID, networkInterfaceName string) (*apiNetNetworkInterfaceConfig, error) {
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

func (p *Plugin) APInetNicName(machineID, networkInterfaceName string) string {
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("%s/%s", machineID, networkInterfaceName)), 5).String()
}

func (p *Plugin) Apply(ctx context.Context, spec *api.NetworkInterfaceSpec, machine *api.Machine) (*providernetworkinterface.NetworkInterface, error) {
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
			IPs:      ironcoreIPsToAPInetIPs(spec.Ips),
			Hostname: machine.Spec.Name,
		},
	}

	log.V(1).Info("Applying apinet nic")
	if err := p.apinetClient.Patch(ctx, apinetNic, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return nil, fmt.Errorf("error applying apinet network interface: %w", err)
	}

	hostDev, direct, err := getHostDevice(apinetNic)
	if err != nil {
		return nil, fmt.Errorf("error getting host device: %w", err)
	}
	if hostDev != nil {
		log.V(1).Info("Host device is ready", "HostDevice", hostDev)
		return &providernetworkinterface.NetworkInterface{
			Handle: provider.GetNetworkInterfaceID(
				apinetNic.Namespace,
				apinetNic.Name,
				apinetNic.Spec.NodeRef.Name,
				apinetNic.UID,
			),
			HostDevice: hostDev,
		}, nil
	}

	if direct != nil {
		log.V(1).Info("Direct device is ready", "Direct", direct)
		return &providernetworkinterface.NetworkInterface{
			Handle: provider.GetNetworkInterfaceID(
				apinetNic.Namespace,
				apinetNic.Name,
				apinetNic.Spec.NodeRef.Name,
				apinetNic.UID,
			),
			Direct: direct,
		}, nil
	}

	log.V(1).Info("Waiting for apinet network interface to become ready")
	apinetNicKey := client.ObjectKeyFromObject(apinetNic)
	if err := wait.PollUntilContextTimeout(ctx, p.pollingInterval, p.pollingDuration, true, func(ctx context.Context) (done bool, err error) {
		if err := p.apinetClient.Get(ctx, apinetNicKey, apinetNic); err != nil {
			return false, fmt.Errorf("error getting apinet nic %s: %w", apinetNicKey, err)
		}

		hostDev, direct, err = getHostDevice(apinetNic)
		if err != nil {
			return false, fmt.Errorf("error getting host device: %w", err)
		}
		return hostDev != nil || direct != nil, nil
	}); err != nil {
		return nil, fmt.Errorf("error waiting for nic to become ready: %w", err)
	}

	// Fetch the updated object to get the ID or any other updated fields
	if err := p.apinetClient.Get(ctx, apinetNicKey, apinetNic); err != nil {
		return nil, fmt.Errorf("error fetching updated apinet network interface: %w", err)
	}

	return &providernetworkinterface.NetworkInterface{
		Handle: provider.GetNetworkInterfaceID(
			apinetNic.Namespace,
			apinetNic.Name,
			apinetNic.Spec.NodeRef.Name,
			apinetNic.UID,
		),
		HostDevice: hostDev,
		Direct:     direct,
	}, nil
}

func getHostDevice(apinetNic *apinetv1alpha1.NetworkInterface) (*providernetworkinterface.HostDevice, *providernetworkinterface.Direct, error) {
	switch apinetNic.Status.State {
	case apinetv1alpha1.NetworkInterfaceStateReady:

		switch {
		case apinetNic.Status.PCIAddress == nil && apinetNic.Status.TAPDevice == nil:
			return nil, nil, fmt.Errorf("apinet network interface: PCIAddress and TAPDevice not set")
		case apinetNic.Status.PCIAddress == nil && apinetNic.Status.TAPDevice != nil:
			tapDevice := apinetNic.Status.TAPDevice
			return nil, &providernetworkinterface.Direct{
				Dev: tapDevice.Name,
			}, nil
		case apinetNic.Status.PCIAddress != nil && apinetNic.Status.TAPDevice == nil:
			pciDevice := apinetNic.Status.PCIAddress
			domain, err := strconv.ParseUint(pciDevice.Domain, 16, strconv.IntSize)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing pci device domain %q: %w", pciDevice.Domain, err)
			}

			bus, err := strconv.ParseUint(pciDevice.Bus, 16, strconv.IntSize)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing pci device bus %q: %w", pciDevice.Bus, err)
			}

			slot, err := strconv.ParseUint(pciDevice.Slot, 16, strconv.IntSize)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing pci device slot %q: %w", pciDevice.Slot, err)
			}

			function, err := strconv.ParseUint(pciDevice.Function, 16, strconv.IntSize)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing pci device function %q: %w", pciDevice.Function, err)
			}

			return &providernetworkinterface.HostDevice{
				Domain:   uint(domain),
				Bus:      uint(bus),
				Slot:     uint(slot),
				Function: uint(function),
			}, nil, nil
		default:
			return nil, nil, fmt.Errorf("apinet network interface: PCIAddress and TAPDevice should not be set at the same time")
		}
	case apinetv1alpha1.NetworkInterfaceStatePending:
		return nil, nil, nil
	case apinetv1alpha1.NetworkInterfaceStateError:
		return nil, nil, fmt.Errorf("apinet network interface is in state error")
	default:
		return nil, nil, nil
	}
}

func (p *Plugin) Delete(ctx context.Context, computeNicName, machineID string) error {
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

	if err := p.apinetClient.Delete(ctx, &apinetv1alpha1.NetworkInterface{
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
		if err := p.apinetClient.Get(ctx, apinetNicKey, &apinetv1alpha1.NetworkInterface{}); err != nil {
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
