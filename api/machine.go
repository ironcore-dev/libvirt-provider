// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
	"k8s.io/utils/ptr"
)

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

	ShutdownAt time.Time `json:"shutdownAt,omitempty"`

	GuestAgent GuestAgent `json:"guestAgent"`
}

type GuestAgent string

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
	Name       string            `json:"name"`
	NetworkId  string            `json:"networkId"`
	Ips        []string          `json:"ips"`
	Attributes map[string]string `json:"attributes"`
}

type NetworkInterfaceStatus struct {
	Name   string                `json:"name"`
	Handle string                `json:"handle"`
	State  NetworkInterfaceState `json:"state"`
}

type NetworkInterfaceState string

const (
	NetworkInterfaceStatePending  NetworkInterfaceState = "Pending"
	NetworkInterfaceStateAttached NetworkInterfaceState = "Attached"
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
