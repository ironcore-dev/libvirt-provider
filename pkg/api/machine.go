// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

type Machine struct {
	Metadata `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec"`
	Status MachineStatus `json:"status"`
}

type MachineSpec struct {
	Power PowerState `json:"power"`

	CpuMillis   int64 `json:"cpuMillis"`
	MemoryBytes int64 `json:"memoryBytes"`

	Image    *string `json:"image"`
	Ignition []byte  `json:"ignition"`

	Volumes           []*VolumeSpec           `json:"volumes"`
	NetworkInterfaces []*NetworkInterfaceSpec `json:"networkInterfaces"`
}

type MachineStatus struct {
	VolumeStatus           []VolumeStatus           `json:"volumeStatus"`
	NetworkInterfaceStatus []NetworkInterfaceStatus `json:"networkInterfaceStatus"`
	State                  MachineState             `json:"state"`
	ImageRef               string                   `json:"imageRef"`
}

type MachineState string

const (
	MachineStatePending    MachineState = "Pending"
	MachineStateRunning    MachineState = "Running"
	MachineStateSuspended  MachineState = "Suspended"
	MachineStateTerminated MachineState = "Terminated"
)

type PowerState int32

const (
	PowerStatePowerOn  PowerState = 0
	PowerStatePowerOff PowerState = 1
)

type VolumeSpec struct {
	Name       string            `json:"name"`
	Device     string            `json:"device"`
	EmptyDisk  *EmptyDiskSpec    `json:"emptyDisk,omitempty"`
	Connection *VolumeConnection `json:"cephDisk,omitempty"`
}

type VolumeStatus struct {
	Name   string      `json:"name,omitempty"`
	Handle string      `json:"handle,omitempty"`
	State  VolumeState `json:"state,omitempty"`
}

type EmptyDiskSpec struct {
	Size int64 `json:"size"`
}

type VolumeConnection struct {
	Driver     string            ` json:"driver,omitempty"`
	Handle     string            ` json:"handle,omitempty"`
	Attributes map[string]string ` json:"attributes,omitempty"`
	SecretData map[string][]byte ` json:"secret_data,omitempty"`
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
	Name   string                ` json:"name"`
	Handle string                ` json:"handle"`
	State  NetworkInterfaceState `json:"state"`
}

type NetworkInterfaceState string

const (
	NetworkInterfaceStatePending  NetworkInterfaceState = "Pending"
	NetworkInterfaceStateAttached NetworkInterfaceState = "Attached"
)
