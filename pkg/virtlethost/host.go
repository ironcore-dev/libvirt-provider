// Copyright 2022 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/digitalocean/go-libvirt"
	ocistore "github.com/onmetal/onmetal-image/oci/store"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultImagesDir  = "images"
	DefaultPluginsDir = "plugins"

	DefaultMachinesDir                 = "machines"
	DefaultMachineVolumesDir           = "volumes"
	DefaultMachineIgnitionsDir         = "ignitions"
	DefaultMachineIgnitionFile         = "data.ign"
	DefaultMachineRootFSDir            = "rootfs"
	DefaultMachineRootFSFile           = "rootfs"
	DefaultMachinePluginsDir           = "plugins"
	DefaultMachineNetworkInterfacesDir = "networkinterfaces"
)

type Paths interface {
	RootDir() string
	MachinesDir() string
	ImagesDir() string
	PluginsDir() string

	PluginDir(pluginName string) string
	MachinePluginsDir(machineUID types.UID) string
	MachinePluginDir(machineUID types.UID, pluginName string) string

	MachineDir(machineUID types.UID) string
	MachineRootFSDir(machineUID types.UID) string
	MachineRootFSFile(machineUID types.UID) string
	MachineVolumesDir(machineUID types.UID) string

	MachineVolumesPluginDir(machineUID types.UID, pluginName string) string
	MachineVolumeDir(machineUID types.UID, pluginName, volumeName string) string

	MachineNetworkInterfacesDir(machineUID types.UID) string
	MachineNetworkInterfaceDir(machineUID types.UID, networkInterfaceName string) string

	MachineIgnitionsDir(machineUID types.UID) string
	MachineIgnitionFile(machineUID types.UID) string
}

type paths struct {
	rootDir string
}

func (p *paths) RootDir() string {
	return p.rootDir
}

func (p *paths) MachinesDir() string {
	return filepath.Join(p.rootDir, DefaultMachinesDir)
}

func (p *paths) ImagesDir() string {
	return filepath.Join(p.rootDir, DefaultImagesDir)
}

func (p *paths) PluginsDir() string {
	return filepath.Join(p.rootDir, DefaultPluginsDir)
}

func (p *paths) PluginDir(pluginName string) string {
	return filepath.Join(p.PluginsDir(), pluginName)
}

func (p *paths) MachineDir(machineUID types.UID) string {
	return filepath.Join(p.MachinesDir(), string(machineUID))
}

func (p *paths) MachineRootFSDir(machineUID types.UID) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineRootFSDir)
}

func (p *paths) MachineRootFSFile(machineUID types.UID) string {
	return filepath.Join(p.MachineRootFSDir(machineUID), DefaultMachineRootFSFile)
}

func (p *paths) MachineVolumesDir(machineUID types.UID) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineVolumesDir)
}

func (p *paths) MachineVolumesPluginDir(machineUID types.UID, pluginName string) string {
	return filepath.Join(p.MachineVolumesDir(machineUID), pluginName)
}

func (p *paths) MachineVolumeDir(machineUID types.UID, pluginName, volumeName string) string {
	return filepath.Join(p.MachineVolumesPluginDir(machineUID, pluginName), volumeName)
}

func (p *paths) MachinePluginsDir(machineUID types.UID) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachinePluginsDir)
}

func (p *paths) MachinePluginDir(machineUID types.UID, pluginName string) string {
	return filepath.Join(p.MachinePluginsDir(machineUID), pluginName)
}

func (p *paths) MachineNetworkInterfacesDir(machineUID types.UID) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineNetworkInterfacesDir)
}

func (p *paths) MachineNetworkInterfaceDir(machineUID types.UID, networkInterfaceName string) string {
	return filepath.Join(p.MachineNetworkInterfacesDir(machineUID), networkInterfaceName)
}

func (p *paths) MachineIgnitionsDir(machineUID types.UID) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineIgnitionsDir)
}

func (p *paths) MachineIgnitionFile(machineUID types.UID) string {
	return filepath.Join(p.MachineIgnitionsDir(machineUID), DefaultMachineIgnitionFile)
}

type Host interface {
	Paths
	Client() client.Client
	APINetClient() client.Client
	OCIStore() *ocistore.Store
}

type LibvirtHost interface {
	Host
	Libvirt() *libvirt.Libvirt
}

type host struct {
	Paths
	client       client.Client
	apinetClient client.Client
	ociStore     *ocistore.Store
}

func (h *host) Client() client.Client {
	return h.client
}

func (h *host) APINetClient() client.Client {
	return h.apinetClient
}

func (h *host) OCIStore() *ocistore.Store {
	return h.ociStore
}

const perm = 0777

func PathsAt(rootDir string) (Paths, error) {
	p := &paths{rootDir}
	if err := os.MkdirAll(p.RootDir(), perm); err != nil {
		return nil, fmt.Errorf("error creating root directory: %w", err)
	}
	if err := os.MkdirAll(p.ImagesDir(), perm); err != nil {
		return nil, fmt.Errorf("error creating images directory: %w", err)
	}
	if err := os.MkdirAll(p.MachinesDir(), perm); err != nil {
		return nil, fmt.Errorf("error creating machines directory: %w", err)
	}
	return p, nil
}

func NewAt(client client.Client, apinetClient client.Client, rootDir string) (Host, error) {
	p, err := PathsAt(rootDir)
	if err != nil {
		return nil, err
	}

	ociStore, err := ocistore.New(p.ImagesDir())
	if err != nil {
		return nil, fmt.Errorf("error creating oci store: %w", err)
	}

	return &host{
		Paths:        p,
		client:       client,
		apinetClient: apinetClient,
		ociStore:     ociStore,
	}, nil
}

type libvirtHost struct {
	Host
	libvirt *libvirt.Libvirt
}

func (h *libvirtHost) Libvirt() *libvirt.Libvirt {
	return h.libvirt
}

func NewLibvirtAt(client client.Client, apinetClient client.Client, rootDir string, libvirt *libvirt.Libvirt) (LibvirtHost, error) {
	host, err := NewAt(client, apinetClient, rootDir)
	if err != nil {
		return nil, err
	}

	return &libvirtHost{host, libvirt}, nil
}

type MachineVolume struct {
	PluginName        string
	ComputeVolumeName string
}

type MachineNetworkInterface struct {
	NetworkInterfaceName string
}

func ReadMachineUIDs(paths Paths) ([]types.UID, error) {
	var res []types.UID
	if err := IterateMachines(paths, func(machineUID types.UID) error {
		res = append(res, machineUID)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

func IterateMachines(paths Paths, f func(machineUID types.UID) error) error {
	entries, err := os.ReadDir(paths.MachinesDir())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		machineUID := types.UID(entry.Name())
		if err := f(machineUID); err != nil {
			return fmt.Errorf("[machine uid %s] %w", machineUID, err)
		}
	}
	return nil
}

func IterateMachineNetworkInterfaces(paths Paths, machineUID types.UID, f func(networkInterfaceName string) error) error {
	entries, err := os.ReadDir(paths.MachineNetworkInterfacesDir(machineUID))
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		networkInterfaceName := entry.Name()
		if err := f(networkInterfaceName); err != nil {
			return fmt.Errorf("[network interface %s] %w", networkInterfaceName, err)
		}
	}
	return nil
}

func ReadMachineNetworkInterfaces(paths Paths, machineUID types.UID) ([]MachineNetworkInterface, error) {
	entries, err := os.ReadDir(paths.MachineNetworkInterfacesDir(machineUID))
	if err != nil {
		return nil, err
	}

	var machineNics []MachineNetworkInterface
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		machineNics = append(machineNics, MachineNetworkInterface{
			NetworkInterfaceName: entry.Name(),
		})
	}
	return machineNics, nil
}

func MakeMachineDirs(paths Paths, machineUID types.UID) error {
	if err := os.MkdirAll(paths.MachineDir(machineUID), perm); err != nil {
		return fmt.Errorf("error creating machine directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineRootFSDir(machineUID), perm); err != nil {
		return fmt.Errorf("error creating machine rootfs directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineVolumesDir(machineUID), perm); err != nil {
		return fmt.Errorf("error creating machine disks directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineIgnitionsDir(machineUID), perm); err != nil {
		return fmt.Errorf("error creating machine ignitions directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineNetworkInterfacesDir(machineUID), perm); err != nil {
		return fmt.Errorf("error creating machine network interfaces directory: %w", err)
	}
	return nil
}
