// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/api"
	"libvirt.org/go/libvirtxml"
)

var (
	errNoNetworkInterfaceAlias = errors.New("no network interface alias")
)

// deleteAllNetworkInterfaces irrespective of their state through the network
// interface plugin. Does not consider which state the network interface might
// be in.
func (r *MachineReconciler) deleteAllNetworkInterfaces(ctx context.Context, log logr.Logger, machine *api.Machine) error {
	var errs []error
	for _, nic := range machine.Spec.NetworkInterfaces {
		log.V(1).Info("Deleting network interface", "NetworkInterfaceName", nic.Name)
		if err := r.networkInterfacePlugin.Delete(ctx, networkInterfaceSpecToPlugin(nic, machine), machine.ID); err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error deleting: %w", nic.Name, err))
		}
	}
	return errors.Join(errs...)
}

// deleteNetworkInterfaces which are safe to delete because they are marked for
// deletion and are not attached to the machine.
func (r *MachineReconciler) deleteNetworkInterfaces(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
	mountedNics map[string]mountedNetworkInterface,
) []error {
	nicStatus := make(map[string]api.NetworkInterfaceStatus, len(machine.Status.NetworkInterfaceStatus))
	for _, status := range machine.Status.NetworkInterfaceStatus {
		nicStatus[status.Name] = status
	}

	var errs []error
	var deleted []string
	for _, nic := range machine.Spec.NetworkInterfaces {
		if nic.DeletionTimestamp == nil || isNicMounted(mountedNics, nic.Name) {
			continue
		}

		log.V(1).Info("Deleting network interface", "NetworkInterfaceName", nic.Name)
		if err := r.networkInterfacePlugin.Delete(ctx, networkInterfaceSpecToPlugin(nic, machine), machine.ID); err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error deleting: %w", nic.Name, err))
			continue
		}
		log.V(1).Info("Successfully deleted network interface", "NetworkInterfaceName", nic.Name)
		deleted = append(deleted, nic.Name)
	}

	for _, e := range deleted {
		machine.Spec.NetworkInterfaces = slices.DeleteFunc(machine.Spec.NetworkInterfaces, func(nic *api.NetworkInterfaceSpec) bool {
			return e == nic.Name
		})
		machine.Status.NetworkInterfaceStatus = slices.DeleteFunc(machine.Status.NetworkInterfaceStatus, func(nic api.NetworkInterfaceStatus) bool {
			return e == nic.Name
		})
	}

	return errs
}

func isNicMounted(mountedNics map[string]mountedNetworkInterface, name string) bool {
	if mountedNics == nil {
		return false
	}

	_, ok := mountedNics[name]
	return ok
}

// setDomainNetworkInterfaces prepares the interfaces for a new machine.
//
// TODO: this shares a lot of code with [MachineReconciler.attachDetachNetworkInterfaces]
// and they could be merged by defining an interface for attach / detach that
// is implemented for both the create and the update case.
func (r *MachineReconciler) setDomainNetworkInterfaces(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
	domainDesc *libvirtxml.Domain,
) error {

	nicStatus := make(map[string]*api.NetworkInterfaceStatus, len(machine.Spec.NetworkInterfaces))
	for _, status := range machine.Status.NetworkInterfaceStatus {
		nicStatus[status.Name] = &status
	}

	var errs []error
	for _, nic := range machine.Spec.NetworkInterfaces {
		if nic.DeletionTimestamp != nil {
			log.V(1).Info("Skipping network interface due to deletion timestamp", "NetworkInterfaceName", nic.Name)
			continue
		}
		log.V(1).Info("Applying network interface", "NetworkInterfaceName", nic.Name)

		status, dev, err := r.applyNetworkInterface(ctx, machine, nic)
		if status != nil {
			nicStatus[nic.Name] = status
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error applying: %w", nic.Name, err))
			continue
		}

		libvirtNic, err := nicDeviceToLibvirt(nic.Name, dev)
		if err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error converting to libvirt nic: %w", nic.Name, err))
			nicStatus[nic.Name].State = api.NetworkInterfaceStateError
			continue
		}

		switch {
		case libvirtNic.hostDev != nil:
			addDomainHostdev(domainDesc, *libvirtNic.hostDev)
		case libvirtNic.iface != nil:
			addDomainInterface(domainDesc, *libvirtNic.iface)
		}

		log.V(1).Info("Successfully applied network interface", "NetworkInterfaceName", nic.Name)
		nicStatus[nic.Name].State = api.NetworkInterfaceStateAttached
	}

	// deleteNetworkInterfaces expect up-to-date state on the machine. Mirror
	// the new status objects back into the machine ordered like they are in the
	// spec.
	machine.Status.NetworkInterfaceStatus = make([]api.NetworkInterfaceStatus, 0, len(nicStatus))
	for _, nic := range machine.Spec.NetworkInterfaces {
		s, ok := nicStatus[nic.Name]
		if !ok {
			continue
		}
		machine.Status.NetworkInterfaceStatus = append(machine.Status.NetworkInterfaceStatus, *s)
	}

	deleteErrs := r.deleteNetworkInterfaces(ctx, log, machine, nil)
	errs = append(errs, deleteErrs...)

	if len(errs) > 0 {
		return fmt.Errorf("network interface attach / detach error(s): %w", errors.Join(errs...))
	}
	return nil
}

func addDomainHostdev(domainDesc *libvirtxml.Domain, hostdev libvirtxml.DomainHostdev) {
	if domainDesc.Devices == nil {
		domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}

	domainDesc.Devices.Hostdevs = append(domainDesc.Devices.Hostdevs, hostdev)
}

func addDomainInterface(domainDesc *libvirtxml.Domain, iface libvirtxml.DomainInterface) {
	if domainDesc.Devices == nil {
		domainDesc.Devices = &libvirtxml.DomainDeviceList{}
	}

	domainDesc.Devices.Interfaces = append(domainDesc.Devices.Interfaces, iface)
}

func (r *MachineReconciler) attachDetachNetworkInterfaces(
	ctx context.Context,
	log logr.Logger,
	machine *api.Machine,
	domainDesc *libvirtxml.Domain,
) error {
	domain := machineDomain(machine.ID)

	mountedNics, err := r.computeMountedNetworkInterfaces(domainDesc)
	if err != nil {
		return err
	}

	desiredNics := make(map[string]*api.NetworkInterfaceSpec)
	for _, nic := range machine.Spec.NetworkInterfaces {
		if nic.DeletionTimestamp == nil {
			desiredNics[nic.Name] = nic
		}
	}

	nicStatus := make(map[string]*api.NetworkInterfaceStatus, len(machine.Spec.NetworkInterfaces))
	for _, status := range machine.Status.NetworkInterfaceStatus {
		nicStatus[status.Name] = &status
	}

	var errs []error

	// Unmount all NICs which should not be mounted.
	for nicName, actualNic := range mountedNics {
		if _, ok := desiredNics[nicName]; ok {
			continue
		}

		log.V(1).Info("Detaching network interface", "NetworkInterfaceName", nicName)
		if err := r.detachDomainDevice(domain, actualNic.libvirt.device()); err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error detaching: %w", nicName, err))
			continue
		}

		log.V(1).Info("Successfully detached network interface", "NetworkInterfaceName", nicName)
		delete(mountedNics, nicName)

		s, ok := nicStatus[nicName]
		if ok {
			s.State = api.NetworkInterfaceStateReady
		}
	}

	// Reconcile desired NICs.
	for _, nic := range machine.Spec.NetworkInterfaces {
		if nic.DeletionTimestamp != nil {
			log.V(1).Info("Skipping network interface due to deletion timestamp", "NetworkInterfaceName", nic.Name)
			continue
		}
		log.V(1).Info("Applying network interface", "NetworkInterfaceName", nic.Name)

		status, dev, err := r.applyNetworkInterface(ctx, machine, nic)
		if status != nil {
			// If apply did not return an error, status is guaranteed to be
			// non-nil.  But we also want to recored any state updated when the
			// apply went through but a later step failed. Since it may be nil
			// in case of an error we can't set the value unconditionally.
			nicStatus[nic.Name] = status
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("[network interface %s] error applying: %w", nic.Name, err))
			continue
		}

		mountedNic, ok := mountedNics[nic.Name]
		if ok {
			if reflect.DeepEqual(mountedNic.device, dev) {
				// The status update from the earlier apply may have overridden
				// the interface state to ready, make sure we keep it set to
				// attached.
				nicStatus[nic.Name].State = api.NetworkInterfaceStateAttached
				continue
			}

			if err := r.detachDomainDevice(domain, mountedNic.libvirt.device()); err != nil {
				nicStatus[nic.Name].State = api.NetworkInterfaceStateError
				errs = append(errs, err)
				continue
			}
		}

		libvirtNic, err := nicDeviceToLibvirt(nic.Name, dev)
		if err != nil {
			nicStatus[nic.Name].State = api.NetworkInterfaceStateError
			errs = append(errs, err)
			continue
		}

		if err := r.attachDomainDevice(domain, libvirtNic.device()); err != nil {
			nicStatus[nic.Name].State = api.NetworkInterfaceStateError
			errs = append(errs, fmt.Errorf("[network interface %s] failed to attach domain device: %w", nic.Name, err))
			continue
		}

		log.V(1).Info("Successfully reconciled desired network interface", "NetworkInterfaceName", nic.Name)
		nicStatus[nic.Name].State = api.NetworkInterfaceStateAttached
	}

	// deleteNetworkInterfaces expect up-to-date state on the machine. Mirror
	// the new status objects back into the machine ordered like they are in the
	// spec.
	machine.Status.NetworkInterfaceStatus = make([]api.NetworkInterfaceStatus, 0, len(nicStatus))
	for _, nic := range machine.Spec.NetworkInterfaces {
		s, ok := nicStatus[nic.Name]
		if !ok {
			continue
		}
		machine.Status.NetworkInterfaceStatus = append(machine.Status.NetworkInterfaceStatus, *s)
	}

	deleteErrs := r.deleteNetworkInterfaces(ctx, log, machine, mountedNics)
	errs = append(errs, deleteErrs...)

	if len(errs) > 0 {
		return fmt.Errorf("attach / detach error(s): %w", errors.Join(errs...))
	}
	return nil
}

// applyNetworkInterface applies the network interface via the plugin. The
// status is reported as a best-effort and may be nil when an err is not nil.
func (r *MachineReconciler) applyNetworkInterface(
	ctx context.Context,
	machine *api.Machine,
	nic *api.NetworkInterfaceSpec,
) (*api.NetworkInterfaceStatus, *nicDevice, error) {
	pluginStatus, err := r.networkInterfacePlugin.Apply(ctx, networkInterfaceSpecToPlugin(nic, machine), machine.ID)
	if err != nil {
		return nil, nil, err
	}

	status := networkInterfaceStatus(nic.Name, pluginStatus)

	switch status.State {
	case api.NetworkInterfaceStateError:
		return &status, nil, errNetworkInterfaceFailed
	case api.NetworkInterfaceStatePending:
		return &status, nil, errNetworkInterfaceNotReady
	case api.NetworkInterfaceStateReady:
		// do not return, continue below
	default:
		panic(fmt.Errorf("unknown network interface plugin state %q", status.State))
	}

	device, err := nicDeviceFromStatus(pluginStatus)
	if err != nil {
		status.State = api.NetworkInterfaceStateError
		return &status, nil, err
	}

	return &status, device, nil
}

type mountedNetworkInterface struct {
	device  *nicDevice
	libvirt *libvirtNetworkInterface
}

type libvirtNetworkInterface struct {
	hostDev *libvirtxml.DomainHostdev
	iface   *libvirtxml.DomainInterface
}

func (i *libvirtNetworkInterface) device() libvirtxml.Document {
	switch {
	case i.hostDev != nil:
		return i.hostDev
	case i.iface != nil:
		return i.iface
	default:
		return nil
	}
}

func (r *MachineReconciler) computeMountedNetworkInterfaces(domainDesc *libvirtxml.Domain) (map[string]mountedNetworkInterface, error) {
	res := make(map[string]mountedNetworkInterface)
	for _, hostDev := range domainDescHostDevices(domainDesc) {
		if hostDev.Alias == nil || !strings.HasPrefix(hostDev.Alias.Name, networkInterfaceAliasPrefix) {
			continue
		}

		name, err := parseNetworkInterfaceAlias(hostDev.Alias.Name)
		if err != nil {
			return nil, err
		}

		hostDev := hostDev
		device, err := libvirtHostdevToNicDevice(&hostDev)
		if err != nil {
			return nil, err
		}

		res[name] = mountedNetworkInterface{
			device: device,
			libvirt: &libvirtNetworkInterface{
				hostDev: &hostDev,
			},
		}
	}
	for _, iface := range domainDescInterfaces(domainDesc) {
		if iface.Alias == nil || !strings.HasPrefix(iface.Alias.Name, networkInterfaceAliasPrefix) {
			continue
		}

		name, err := parseNetworkInterfaceAlias(iface.Alias.Name)
		if err != nil {
			return nil, err
		}

		iface := iface
		device, err := libvirtInterfaceToNicDevice(&iface)
		if err != nil {
			return nil, err
		}

		res[name] = mountedNetworkInterface{
			device: device,
			libvirt: &libvirtNetworkInterface{
				iface: &iface,
			},
		}
	}
	return res, nil
}

func domainDescHostDevices(domainDesc *libvirtxml.Domain) []libvirtxml.DomainHostdev {
	if domainDesc.Devices == nil {
		return nil
	}
	return domainDesc.Devices.Hostdevs
}

func domainDescInterfaces(domainDesc *libvirtxml.Domain) []libvirtxml.DomainInterface {
	if domainDesc.Devices == nil {
		return nil
	}
	return domainDesc.Devices.Interfaces
}

func (r *MachineReconciler) attachDomainDevice(domain libvirt.Domain, dev libvirtxml.Document) error {
	data, err := dev.Marshal()
	if err != nil {
		return err
	}
	return r.host.Libvirt().DomainAttachDevice(domain, data)
}

func (r *MachineReconciler) detachDomainDevice(domain libvirt.Domain, dev libvirtxml.Document) error {
	data, err := dev.Marshal()
	if err != nil {
		return err
	}
	return r.host.Libvirt().DomainDetachDevice(domain, data)
}

func parseNetworkInterfaceAlias(alias string) (string, error) {
	if !strings.HasPrefix(alias, networkInterfaceAliasPrefix) {
		return "", errNoNetworkInterfaceAlias
	}
	return strings.TrimPrefix(alias, networkInterfaceAliasPrefix), nil
}

func networkInterfaceAlias(name string) string {
	return fmt.Sprintf("%s%s", networkInterfaceAliasPrefix, name)
}
