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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultImagesDir  = "images"
	DefaultPluginsDir = "plugins"

	DefaultMachinesDir                 = "machines"
	DefaultStoreDir                    = "store"
	DefaultMachineStoreDir             = "machines"
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
	StoreDir() string

	MachinesDir() string
	MachineStoreDir() string
	ImagesDir() string
	PluginsDir() string

	PluginDir(pluginName string) string
	MachinePluginsDir(machineUID string) string
	MachinePluginDir(machineUID string, pluginName string) string

	MachineDir(machineUID string) string
	MachineRootFSDir(machineUID string) string
	MachineRootFSFile(machineUID string) string
	MachineVolumesDir(machineUID string) string

	MachineVolumesPluginDir(machineUID string, pluginName string) string
	MachineVolumeDir(machineUID string, pluginName, volumeName string) string

	MachineNetworkInterfacesDir(machineUID string) string
	MachineNetworkInterfaceDir(machineUID string, networkInterfaceName string) string

	MachineIgnitionsDir(machineUID string) string
	MachineIgnitionFile(machineUID string) string
}

type paths struct {
	rootDir string
}

func (p *paths) RootDir() string {
	return p.rootDir
}

func (p *paths) StoreDir() string {
	return filepath.Join(p.rootDir, DefaultStoreDir)
}

func (p *paths) MachinesDir() string {
	return filepath.Join(p.rootDir, DefaultMachinesDir)
}

func (p *paths) MachineStoreDir() string {
	return filepath.Join(p.StoreDir(), DefaultMachineStoreDir)
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

func (p *paths) MachineDir(machineUID string) string {
	return filepath.Join(p.MachinesDir(), string(machineUID))
}

func (p *paths) MachineRootFSDir(machineUID string) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineRootFSDir)
}

func (p *paths) MachineRootFSFile(machineUID string) string {
	return filepath.Join(p.MachineRootFSDir(machineUID), DefaultMachineRootFSFile)
}

func (p *paths) MachineVolumesDir(machineUID string) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineVolumesDir)
}

func (p *paths) MachineVolumesPluginDir(machineUID string, pluginName string) string {
	return filepath.Join(p.MachineVolumesDir(machineUID), pluginName)
}

func (p *paths) MachineVolumeDir(machineUID string, pluginName, volumeName string) string {
	return filepath.Join(p.MachineVolumesPluginDir(machineUID, pluginName), volumeName)
}

func (p *paths) MachinePluginsDir(machineUID string) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachinePluginsDir)
}

func (p *paths) MachinePluginDir(machineUID string, pluginName string) string {
	return filepath.Join(p.MachinePluginsDir(machineUID), pluginName)
}

func (p *paths) MachineNetworkInterfacesDir(machineUID string) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineNetworkInterfacesDir)
}

func (p *paths) MachineNetworkInterfaceDir(machineUID string, networkInterfaceName string) string {
	return filepath.Join(p.MachineNetworkInterfacesDir(machineUID), networkInterfaceName)
}

func (p *paths) MachineIgnitionsDir(machineUID string) string {
	return filepath.Join(p.MachineDir(machineUID), DefaultMachineIgnitionsDir)
}

func (p *paths) MachineIgnitionFile(machineUID string) string {
	return filepath.Join(p.MachineIgnitionsDir(machineUID), DefaultMachineIgnitionFile)
}

type Host interface {
	Paths
	APINetClient() client.Client
	OCIStore() *ocistore.Store
}

type LibvirtHost interface {
	Host
	Libvirt() *libvirt.Libvirt
}

type host struct {
	Paths
	apinetClient client.Client
	ociStore     *ocistore.Store
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

func NewAt(apinetClient client.Client, rootDir string) (Host, error) {
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

func NewLibvirtAt(apinetClient client.Client, rootDir string, libvirt *libvirt.Libvirt) (LibvirtHost, error) {
	host, err := NewAt(apinetClient, rootDir)
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

func ReadMachineUIDs(paths Paths) ([]string, error) {
	var res []string
	if err := IterateMachines(paths, func(machineUID string) error {
		res = append(res, machineUID)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

func IterateMachines(paths Paths, f func(machineUID string) error) error {
	entries, err := os.ReadDir(paths.MachinesDir())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		machineUID := string(entry.Name())
		if err := f(machineUID); err != nil {
			return fmt.Errorf("[machine uid %s] %w", machineUID, err)
		}
	}
	return nil
}

func IterateMachineNetworkInterfaces(paths Paths, machineUID string, f func(networkInterfaceName string) error) error {
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

func ReadMachineNetworkInterfaces(paths Paths, machineUID string) ([]MachineNetworkInterface, error) {
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

func MakeMachineDirs(paths Paths, machineUID string) error {
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
