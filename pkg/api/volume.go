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
	Provider  VolumeProvider `json:"provider"`
	EmptyDisk *EmptyDiskSpec `json:"emptyDisk,omitempty"`
	CephDisk  *CephDisk      `json:"cephDisk,omitempty"`
}

type EmptyDiskSpec struct {
	Size int64 `json:"size"`
}

type CephDisk struct {
	Image      string
	Monitors   []CephMonitor
	Auth       CephAuthentication
	Encryption *CephEncryption
}

type CephAuthentication struct {
	UserName string
	UserKey  string
}

type CephEncryption struct {
	EncryptionKey string
}

type CephMonitor struct {
	Name string
	Port string
}

type VolumeStatus struct {
	State  VolumeState `json:"state"`
	Handle string      `json:"handle"`
	Size   int64       `json:"size"`
}

type VolumeState string

const (
	VolumeStatePending  VolumeState = "Pending"
	VolumeStatePrepared VolumeState = "Prepared"
	VolumeStateAttached VolumeState = "Attached"
)

type VolumeProvider string

const (
	VolumeProviderCeph      VolumeProvider = "CephDisk"
	VolumeProviderEmptyDisk VolumeProvider = "EmptyDisk "
)
