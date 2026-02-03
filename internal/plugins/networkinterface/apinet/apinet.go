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
	"sync"

	"github.com/go-logr/logr"
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
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("networking.ironcore.dev/libvirt-provider")

	defaultAPINetConfigFile = "api-net.json"

	perm         = 0o777
	filePerm     = 0o666
	pluginAPInet = "apinet"

	labelMachineID = "libvirt-provider.ironcore.dev/machine-id"
)

var watcherScheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(watcherScheme))
	utilruntime.Must(apinetv1alpha1.AddToScheme(watcherScheme))
}

type Plugin struct {
	nodeName      string
	host          providerhost.LibvirtHost
	apinetClient  client.Client
	restConfig    *rest.Config
	mu            sync.RWMutex
	eventHandlers []providernetworkinterface.EventHandler
}

func NewPlugin(nodeName string, restCfg *rest.Config) providernetworkinterface.Plugin {
	return &Plugin{
		nodeName:   nodeName,
		restConfig: restCfg,
	}
}

func GetAPInetPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(ctx context.Context, host providerhost.LibvirtHost) error {
	p.host = host

	apinetClient, err := client.New(p.restConfig, client.Options{Scheme: watcherScheme})
	if err != nil {
		return fmt.Errorf("error creating apinet client: %w", err)
	}
	p.apinetClient = apinetClient

	if err := p.startWatcher(ctx); err != nil {
		return fmt.Errorf("error starting NIC watcher: %w", err)
	}
	return nil
}

func (p *Plugin) AddEventHandler(handler providernetworkinterface.EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventHandlers = append(p.eventHandlers, handler)
}

func (p *Plugin) notifyEventHandlers(machineID string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, h := range p.eventHandlers {
		h.HandleNICEvent(machineID)
	}
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
			Labels: map[string]string{
				labelMachineID: machine.ID,
			},
		},
		Spec: apinetv1alpha1.NetworkInterfaceSpec{
			NetworkRef: corev1.LocalObjectReference{
				Name: apinetNetworkName,
			},
			NodeRef: corev1.LocalObjectReference{
				Name: p.nodeName,
			},
			IPs:      ironcoreIPsToAPInetIPs(spec.Ips),
			Hostname: spec.HostName,
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

	if hostDev == nil && direct == nil {
		log.V(1).Info("APINet NIC not ready yet, will be requeued by watcher")
		return nil, providernetworkinterface.ErrNotReady
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

	apinetNic := &apinetv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.Namespace,
			Name:      p.APInetNicName(machineID, computeNicName),
		},
	}
	log = log.WithValues("APInetNetworkInterface", client.ObjectKeyFromObject(apinetNic))

	if err := p.apinetClient.Delete(ctx, apinetNic); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("error deleting apinet network interface: %w", err)
	}

	log.V(1).Info("APInet network interface delete requested, removing network interface dir")
	return os.RemoveAll(p.host.MachineNetworkInterfaceDir(machineID, computeNicName))
}

func (p *Plugin) Name() string {
	return pluginAPInet
}

func (p *Plugin) startWatcher(ctx context.Context) error {
	log := ctrl.Log.WithName("apinet-watcher")

	c, err := cache.New(p.restConfig, cache.Options{
		Scheme: watcherScheme,
		ByObject: map[client.Object]cache.ByObject{
			&apinetv1alpha1.NetworkInterface{}: {},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create informer cache: %w", err)
	}

	informer, err := c.GetInformer(ctx, &apinetv1alpha1.NetworkInterface{})
	if err != nil {
		return fmt.Errorf("failed to get informer: %w", err)
	}

	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			p.handleNICUpdate(log, oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			p.handleNICDelete(log, obj)
		},
	}); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	go func() {
		if err := c.Start(ctx); err != nil {
			log.Error(err, "Informer cache stopped with error")
		}
	}()

	if !c.WaitForCacheSync(ctx) {
		return fmt.Errorf("failed to sync informer cache")
	}
	log.Info("Informer cache synced, enqueuing machines for NIC reconciliation")

	if err := p.enqueueNICMachines(ctx, log, c); err != nil {
		return err
	}

	return nil
}

func (p *Plugin) handleNICUpdate(log logr.Logger, oldObj, newObj interface{}) {
	newNic, ok := newObj.(*apinetv1alpha1.NetworkInterface)
	if !ok {
		return
	}

	if newNic.Spec.NodeRef.Name != p.nodeName {
		return
	}

	oldNic, ok := oldObj.(*apinetv1alpha1.NetworkInterface)
	if !ok {
		return
	}

	if oldNic.Status.State == newNic.Status.State {
		return
	}

	machineID := newNic.Labels[labelMachineID]
	if machineID == "" {
		return
	}

	log.V(1).Info("NIC state changed, requeueing machine", "machineID", machineID, "nic", newNic.Name, "oldState", oldNic.Status.State, "newState", newNic.Status.State)
	p.notifyEventHandlers(machineID)
}

func (p *Plugin) handleNICDelete(log logr.Logger, obj interface{}) {
	if d, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = d.Obj
	}

	nic, ok := obj.(*apinetv1alpha1.NetworkInterface)
	if !ok {
		return
	}

	if nic.Spec.NodeRef.Name != p.nodeName {
		return
	}

	machineID := nic.Labels[labelMachineID]
	if machineID == "" {
		log.V(2).Info("Ignoring NIC delete without machine-id label", "name", nic.Name, "namespace", nic.Namespace)
		return
	}

	log.V(1).Info("NIC deleted, requeueing machine", "machineID", machineID, "nic", nic.Name)
	p.notifyEventHandlers(machineID)
}

func (p *Plugin) enqueueNICMachines(ctx context.Context, log logr.Logger, c cache.Cache) error {
	nicList := &apinetv1alpha1.NetworkInterfaceList{}
	if err := c.List(ctx, nicList); err != nil {
		return fmt.Errorf("failed to list NICs: %w", err)
	}

	for i := range nicList.Items {
		nic := &nicList.Items[i]
		if nic.Spec.NodeRef.Name != p.nodeName {
			continue
		}

		machineID := nic.Labels[labelMachineID]
		if machineID == "" {
			continue
		}

		if _, err := os.Stat(p.host.MachineDir(machineID)); err != nil {
			if !os.IsNotExist(err) {
				log.Error(err, "Failed to stat machine directory", "machineID", machineID)
				continue
			}
			log.V(1).Info("Deleting stale APINet NIC", "machineID", machineID, "nic", nic.Name, "namespace", nic.Namespace)
			if err := p.apinetClient.Delete(ctx, nic); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete stale NIC", "nic", nic.Name, "namespace", nic.Namespace)
			}
			continue
		}

		log.V(1).Info("Enqueuing machine", "machineID", machineID, "nic", nic.Name)
		p.notifyEventHandlers(machineID)
	}

	return nil
}
