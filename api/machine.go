// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	"github.com/ironcore-dev/provider-utils/claimutils/pci"
	"k8s.io/utils/ptr"
)

const (
	MachineMetadataDeletedField = "metadata.deleted"
	MachineSpecImageField       = "spec.image"
)

func SetupMachineMetadataDeletedFieldIndexer(m *Machine) string {
	if m.DeletedAt != nil {
		return "true"
	}
	return "false"
}

func SetupMachineSpecImageFieldIndexer(m *Machine) string {
	if img := HasBootImage(m); img != nil {
		return *img
	}
	return ""
}

type Machine struct {
	apiutils.Metadata `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec"`
	Status MachineStatus `json:"status"`
}

type MachineSpec struct {
	Power PowerState `json:"power"`

	Cpu         int64 `json:"cpu"`
	MemoryBytes int64 `json:"memoryBytes"`

	Ignition []byte `json:"ignition"`

	Volumes           []*VolumeSpec           `json:"volumes"`
	NetworkInterfaces []*NetworkInterfaceSpec `json:"networkInterfaces"`
	Gpu               []pci.Address           `json:"gpu"`

	ShutdownAt time.Time `json:"shutdownAt,omitempty"`

	GuestAgent GuestAgent `json:"guestAgent"`

	// GuestConfig contains an optional guest OS level configuration for the machine.
	GuestConfig *MachineGuestConfig `json:"guestConfig,omitempty"`
}

type GuestAgent string

type MachineGuestConfig struct {
	HostName string `json:"hostName,omitempty"`
}

const (
	GuestAgentNone GuestAgent = "None"
	GuestAgentQemu GuestAgent = "Qemu"
)

type MachineStatus struct {
	VolumeStatus           []VolumeStatus           `json:"volumeStatus"`
	NetworkInterfaceStatus []NetworkInterfaceStatus `json:"networkInterfaceStatus"`
	State                  MachineState             `json:"state"`
	ImageRef               string                   `json:"imageRef"`
	GuestAgentStatus       *GuestAgentStatus        `json:"guestAgentStatus,omitempty"`
}

type MachineState string

const (
	MachineStatePending     MachineState = "Pending"
	MachineStateRunning     MachineState = "Running"
	MachineStateSuspended   MachineState = "Suspended"
	MachineStateTerminating MachineState = "Terminating"
	MachineStateTerminated  MachineState = "Terminated"
)

type PowerState int32

const (
	PowerStatePowerOn  PowerState = 0
	PowerStatePowerOff PowerState = 1
)

type VolumeSpec struct {
	Name       string            `json:"name"`
	Device     string            `json:"device"`
	LocalDisk  *LocalDiskSpec    `json:"localDisk,omitempty"`
	Connection *VolumeConnection `json:"cephDisk,omitempty"`
}

type VolumeStatus struct {
	Name   string      `json:"name,omitempty"`
	Handle string      `json:"handle,omitempty"`
	State  VolumeState `json:"state,omitempty"`
	Size   int64       `json:"size,omitempty"`
}

type LocalDiskSpec struct {
	Size  int64   `json:"size"`
	Image *string `json:"image"`
}

type VolumeConnection struct {
	Driver                string            `json:"driver,omitempty"`
	Handle                string            `json:"handle,omitempty"`
	Attributes            map[string]string `json:"attributes,omitempty"`
	SecretData            map[string][]byte `json:"secret_data,omitempty"`
	EncryptionData        map[string][]byte `json:"encryption_data,omitempty"`
	EffectiveStorageBytes int64             `json:"effective_storage_bytes,omitempty"`
}

type VolumeState string

const (
	VolumeStatePending  VolumeState = "Pending"
	VolumeStateAttached VolumeState = "Attached"
)

type NetworkInterfaceSpec struct {
	// Metadata of the stand-alone network interface referenced by the machine.
	Metadata *NetworkInterfaceMetadata `json:"metadata,omitempty"`
	// Name of the network interface within the machine.
	Name       string            `json:"name"`
	NetworkId  string            `json:"networkId"`
	Ips        []string          `json:"ips"`
	Attributes map[string]string `json:"attributes"`
}

type NetworkInterfaceMetadata struct {
	ID          string            `json:"id,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type NetworkInterfaceStatus struct {
	Name      string                `json:"name"`
	Handle    string                `json:"handle"`
	State     NetworkInterfaceState `json:"state"`
	Ips       []string              `json:"ips,omitempty"`
	Prefixes  []string              `json:"prefixes,omitempty"`
	VirtualIP string                `json:"virtualIP,omitempty"`
}

// NetworkInterfaceState is a combined state of the IronCore NetworkInterface
// and the network interface sub-resource of the machine.
type NetworkInterfaceState string

const (
	// NetworkInterfaceStatePending means the networking for the interface is
	// not realized yet and the interface is not attached to the machine.
	NetworkInterfaceStatePending NetworkInterfaceState = "Pending"
	// NetworkInterfaceStateReady means the networking for the interface is
	// realized (addresses allocated, network plane programmed) but the
	// interface is not yet attached to the machine.
	NetworkInterfaceStateReady NetworkInterfaceState = "Ready"
	// NetworkInterfaceStateAttached means the interface is attached to the
	// machine.
	NetworkInterfaceStateAttached NetworkInterfaceState = "Attached"
	// NetworkInterfaceStateError means realizing or attaching the interface
	// failed.
	NetworkInterfaceStateError NetworkInterfaceState = "Error"
)

type GuestAgentStatus struct {
	Addr string `json:"addr,omitempty"`
}

func HasBootImage(machine *Machine) *string {
	for _, volume := range machine.Spec.Volumes {
		if volume.LocalDisk == nil {
			continue
		}

		if volume.LocalDisk.Image != nil {
			return volume.LocalDisk.Image
		}
	}
	return nil
}

func IsImageReferenced(machine *Machine, image string) bool {
	bootImage := HasBootImage(machine)
	if bootImage == nil {
		return false
	}

	return ptr.Deref(bootImage, "") == image
}
