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
	Power   PowerState `json:"power"`
	Image   *ImageSpec `json:"image"`
	Volumes []string   `json:"volumes"`
}

type MachineStatus struct {
	State    MachineState `json:"state"`
	ImageRef string       `json:"imageRef"`
}

type MachineState string

const (
	MachineStatePending MachineState = "Pending"
)

type PowerState int32

const (
	PowerStatePowerOn  PowerState = 0
	PowerStatePowerOff PowerState = 1
)

type ImageSpec struct {
	Image string `json:"image"`
}
