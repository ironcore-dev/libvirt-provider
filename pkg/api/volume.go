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

type Volume struct {
	Metadata `json:"metadata,omitempty"`

	Spec   VolumeSpec   `json:"spec"`
	Status VolumeStatus `json:"status"`
}

type VolumeSpec struct {
	Name       string                `json:"name"`
	Device     string                `json:"device"`
	EmptyDisk  *EmptyDiskSpec        `json:"empty_disk,omitempty"`
	Connection *VolumeConnectionSpec `json:"connection,omitempty"`
}

type EmptyDiskSpec struct {
	Size int64 `json:"size"`
}

type VolumeConnectionSpec struct {
	Driver     string            `json:"driver"`
	Handle     string            `json:"handle"`
	Attributes map[string]string `json:"attributes,omitempty"`
	SecretData map[string][]byte `protobuf:"bytes,4,rep,name=secret_data,json=secretData,proto3" json:"secret_data,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

type VolumeStatus struct {
	State  VolumeState `json:"state"`
	Name   string      `json:"name"`
	Handle string      `json:"handle"`

	Size int64 `json:"size"`
}

type VolumeState string

const (
	VolumeStatePending  VolumeState = "Pending"
	VolumeStateAttached VolumeState = "Attached"
)
