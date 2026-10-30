// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

// Package apinet implements the network interface plugin backed by
// ironcore-net. It is self-contained and only depends on the provider-utils
// plugin contract, ironcore-net, Kubernetes and controller-runtime.
package apinet

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/netip"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apinetv1alpha1 "github.com/ironcore-dev/ironcore-net/api/core/v1alpha1"
	apinet "github.com/ironcore-dev/ironcore-net/apimachinery/api/net"
	"github.com/ironcore-dev/ironcore-net/apinetlet/provider"
	apinetv1alpha1Apply "github.com/ironcore-dev/ironcore-net/client-go/applyconfigurations/core/v1alpha1"
	"github.com/ironcore-dev/provider-utils/plugin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const pluginName = "apinet"

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apinetv1alpha1.AddToScheme(scheme))
}

// Options configure the apinet plugin.
type Options struct {
	// NodeName is the apinet node the network interfaces are scheduled on.
	NodeName string
	// RestConfig is used to connect to the apinet API server.
	RestConfig *rest.Config
	// FieldOwner is the server-side apply field owner.
	FieldOwner client.FieldOwner
	// MachineIDLabel is the label on apinet network interfaces holding the
	// ID of the machine they belong to.
	MachineIDLabel string
	// MachineExists reports whether a machine is still known. If set, apinet
	// network interfaces of unknown machines are garbage collected in Start.
	// Optional.
	MachineExists func(machineID string) (bool, error)
}

// Plugin is the apinet network interface plugin.
type Plugin struct {
	nodeName       string
	restConfig     *rest.Config
	fieldOwner     client.FieldOwner
	machineIDLabel string
	machineExists  func(machineID string) (bool, error)
	onChange       func(machineID string)

	client client.Client
}

var _ plugin.NetworkInterface = (*Plugin)(nil)

// New creates the plugin. Apply and Delete are usable right away. Start must
// be run to get notified about network interface changes.
func New(opts Options) (*Plugin, error) {
	switch {
	case opts.NodeName == "":
		return nil, fmt.Errorf("must specify node name")
	case opts.RestConfig == nil:
		return nil, fmt.Errorf("must specify rest config")
	case opts.FieldOwner == "":
		return nil, fmt.Errorf("must specify field owner")
	case opts.MachineIDLabel == "":
		return nil, fmt.Errorf("must specify machine ID label")
	}

	c, err := client.New(opts.RestConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating apinet client: %w", err)
	}

	return &Plugin{
		nodeName:       opts.NodeName,
		restConfig:     opts.RestConfig,
		fieldOwner:     opts.FieldOwner,
		machineIDLabel: opts.MachineIDLabel,
		machineExists:  opts.MachineExists,
		client:         c,
	}, nil
}

func (p *Plugin) Name() string {
	return pluginName
}

// NicName returns the deterministic name of the apinet network interface for
// the given machine and compute network interface name.
func NicName(machineID, networkInterfaceName string) string {
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("%s/%s", machineID, networkInterfaceName)), 5).String()
}

func parseIPs(ips []string) ([]apinet.IP, error) {
	res := make([]apinet.IP, len(ips))
	for i, s := range ips {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("error parsing ip %q: %w", s, err)
		}
		res[i] = apinet.IP{Addr: addr}
	}
	return res, nil
}

func (p *Plugin) Apply(ctx context.Context, spec *plugin.NetworkInterfaceSpec, machineID string) (*plugin.NetworkInterfaceStatus, error) {
	log := ctrl.LoggerFrom(ctx)

	apinetNamespace, apinetNetworkName, _, _, err := provider.ParseNetworkID(spec.NetworkId)
	if err != nil {
		return nil, fmt.Errorf("error parsing ApiNet NetworkID %s: %w", spec.NetworkId, err)
	}

	ips, err := parseIPs(spec.Ips)
	if err != nil {
		return nil, err
	}

	apinetNicApply := apinetv1alpha1Apply.NetworkInterface(NicName(machineID, spec.Name), apinetNamespace).
		WithAPIVersion(apinetv1alpha1.SchemeGroupVersion.String()).
		WithKind("NetworkInterface").
		WithLabels(map[string]string{
			p.machineIDLabel: machineID,
		}).
		WithSpec(apinetv1alpha1Apply.NetworkInterfaceSpec().
			WithNetworkRef(corev1.LocalObjectReference{Name: apinetNetworkName}).
			WithNodeRef(corev1.LocalObjectReference{Name: p.nodeName}).
			WithIPs(ips...).
			WithHostname(spec.Hostname))

	log.V(1).Info("Applying APINet network interface")
	if err := p.client.Apply(ctx, apinetNicApply, client.ForceOwnership, p.fieldOwner); err != nil {
		return nil, fmt.Errorf("error applying apinet network interface: %w", err)
	}

	apinetNic := &apinetv1alpha1.NetworkInterface{}
	apinetNicKey := client.ObjectKey{Name: *apinetNicApply.GetName(), Namespace: *apinetNicApply.GetNamespace()}
	if err := p.client.Get(ctx, apinetNicKey, apinetNic); err != nil {
		return nil, fmt.Errorf("error getting apinet network interface: %w", err)
	}

	return statusFromNic(apinetNic)
}

// statusFromNic maps an apinet network interface to the plugin status. The
// handle is reported in every state so the network interface can be deleted
// even if it never became ready.
func statusFromNic(nic *apinetv1alpha1.NetworkInterface) (*plugin.NetworkInterfaceStatus, error) {
	handle := provider.GetNetworkInterfaceID(
		nic.Namespace,
		nic.Name,
		nic.Spec.NodeRef.Name,
		nic.UID,
	)

	switch nic.Status.State {
	case apinetv1alpha1.NetworkInterfaceStateReady:
	case apinetv1alpha1.NetworkInterfaceStateError:
		return &plugin.NetworkInterfaceStatus{Handle: handle, State: plugin.NetworkInterfaceStateError}, nil
	default:
		// Pending or not yet reported.
		return &plugin.NetworkInterfaceStatus{Handle: handle, State: plugin.NetworkInterfaceStatePending}, nil
	}

	status := &plugin.NetworkInterfaceStatus{
		Handle:    handle,
		State:     plugin.NetworkInterfaceStateReady,
		Prefixes:  make([]netip.Prefix, 0, len(nic.Status.Prefixes)),
		Ips:       make([]netip.Addr, 0, len(nic.Spec.IPs)),
		VirtualIp: netip.Addr{},
	}
	for _, prefix := range nic.Status.Prefixes {
		status.Prefixes = append(status.Prefixes, prefix.Prefix)
	}
	for _, ip := range nic.Spec.IPs {
		status.Ips = append(status.Ips, ip.Addr)
	}
	if len(nic.Status.PublicIPs) > 0 {
		status.VirtualIp = nic.Status.PublicIPs[0].Addr
	}

	switch pci, tap := nic.Status.PCIAddress, nic.Status.TAPDevice; {
	case pci == nil && tap == nil:
		return nil, fmt.Errorf("apinet network interface: PCIAddress and TAPDevice not set")
	case pci != nil && tap != nil:
		return nil, fmt.Errorf("apinet network interface: PCIAddress and TAPDevice should not be set at the same time")
	case pci != nil:
		status.PCI = &plugin.PCI{BDF: formatBDF(pci)}
	default:
		status.Netdev = &plugin.Netdev{Name: tap.Name}
	}
	return status, nil
}

func formatBDF(addr *apinetv1alpha1.PCIAddress) string {
	if addr.Domain == "" {
		return fmt.Sprintf("%s:%s.%s", addr.Bus, addr.Slot, addr.Function)
	}
	return fmt.Sprintf("%s:%s:%s.%s", addr.Domain, addr.Bus, addr.Slot, addr.Function)
}

func (p *Plugin) Delete(ctx context.Context, spec *plugin.NetworkInterfaceSpec, machineID string) error {
	log := ctrl.LoggerFrom(ctx)

	namespace, _, _, _, err := provider.ParseNetworkID(spec.NetworkId)
	if err != nil {
		return fmt.Errorf("error parsing ApiNet NetworkID %s: %w", spec.NetworkId, err)
	}
	name := NicName(machineID, spec.Name)

	nic := &apinetv1alpha1.NetworkInterface{}
	if err := p.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, nic); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("error getting apinet network interface: %w", err)
	}

	log.V(1).Info("Deleting APINet network interface", "APInetNetworkInterface", client.ObjectKeyFromObject(nic))
	if err := p.client.Delete(ctx, nic); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
			return nil
		}
		return fmt.Errorf("error deleting apinet network interface: %w", err)
	}
	return nil
}

func (p *Plugin) notify(machineID string) {
	if p.onChange != nil {
		p.onChange(machineID)
	}
}

// Start watches the plugin's apinet network interfaces and calls onChange with
// the machine ID whenever the network interface of a machine changed or was
// deleted. Once the cache is synced it garbage collects network interfaces of
// unknown machines (if Options.MachineExists is set) and calls onChange for
// all remaining ones. Start blocks until ctx is done and must be called at
// most once.
func (p *Plugin) Start(ctx context.Context, onChange func(machineID string)) error {
	log := ctrl.Log.WithName("apinet-watcher")

	p.onChange = onChange

	c, err := cache.New(p.restConfig, cache.Options{
		Scheme: scheme,
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

	cacheErr := make(chan error, 1)
	go func() {
		cacheErr <- c.Start(ctx)
	}()

	if !c.WaitForCacheSync(ctx) {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("failed to sync informer cache")
	}
	log.Info("Informer cache synced, enqueuing machines for NIC reconciliation")

	if err := p.enqueueNICMachines(ctx, log, c); err != nil {
		return err
	}

	if err := <-cacheErr; err != nil {
		return fmt.Errorf("informer cache stopped with error: %w", err)
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

	if reflect.DeepEqual(oldNic.Status, newNic.Status) {
		return
	}

	machineID := newNic.Labels[p.machineIDLabel]
	if machineID == "" {
		return
	}

	log.V(1).Info("NIC state changed, notifying", "machineID", machineID, "nic", newNic.Name, "oldState", oldNic.Status.State, "newState", newNic.Status.State)
	p.notify(machineID)
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

	machineID := nic.Labels[p.machineIDLabel]
	if machineID == "" {
		log.V(2).Info("Ignoring NIC delete without machine-id label", "name", nic.Name, "namespace", nic.Namespace)
		return
	}

	log.V(1).Info("NIC deleted, notifying", "machineID", machineID, "nic", nic.Name)
	p.notify(machineID)
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

		machineID := nic.Labels[p.machineIDLabel]
		if machineID == "" {
			continue
		}

		if p.machineExists != nil {
			exists, err := p.machineExists(machineID)
			if err != nil {
				log.Error(err, "Failed to check machine existence", "machineID", machineID)
				continue
			}
			if !exists {
				log.V(1).Info("Deleting stale APINet NIC", "machineID", machineID, "nic", nic.Name, "namespace", nic.Namespace)
				if err := p.client.Delete(ctx, nic); err != nil && !apierrors.IsNotFound(err) {
					log.Error(err, "Failed to delete stale NIC", "nic", nic.Name, "namespace", nic.Namespace)
				}
				continue
			}
		}

		log.V(1).Info("Notifying about existing NIC", "machineID", machineID, "nic", nic.Name)
		p.notify(machineID)
	}

	return nil
}
