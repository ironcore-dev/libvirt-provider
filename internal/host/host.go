// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/digitalocean/go-libvirt"
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

type LibvirtHost interface {
	Paths
	Libvirt() *libvirt.Libvirt
}

func PathsAt(rootDir string) (Paths, error) {
	p := &paths{rootDir}
	if err := os.MkdirAll(p.RootDir(), os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating root directory: %w", err)
	}
	if err := os.MkdirAll(p.ImagesDir(), os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating images directory: %w", err)
	}
	if err := os.MkdirAll(p.MachinesDir(), os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating machines directory: %w", err)
	}
	return p, nil
}

func NewAt(rootDir string) (Paths, error) {
	p, err := PathsAt(rootDir)
	if err != nil {
		return nil, err
	}

	return p, nil
}

type libvirtHost struct {
	Paths
	libvirt *libvirt.Libvirt
}

func (h *libvirtHost) Libvirt() *libvirt.Libvirt {
	return h.libvirt
}

func NewLibvirtAt(rootDir string, libvirt *libvirt.Libvirt) (LibvirtHost, error) {
	p, err := NewAt(rootDir)
	if err != nil {
		return nil, err
	}

	return &libvirtHost{p, libvirt}, nil
}

type MachineVolume struct {
	PluginName        string
	ComputeVolumeName string
}

type MachineNetworkInterface struct {
	NetworkInterfaceName string
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
	if err := os.MkdirAll(paths.MachineDir(machineUID), os.ModePerm); err != nil {
		return fmt.Errorf("error creating machine directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineRootFSDir(machineUID), os.ModePerm); err != nil {
		return fmt.Errorf("error creating machine rootfs directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineVolumesDir(machineUID), os.ModePerm); err != nil {
		return fmt.Errorf("error creating machine disks directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineIgnitionsDir(machineUID), os.ModePerm); err != nil {
		return fmt.Errorf("error creating machine ignitions directory: %w", err)
	}
	if err := os.MkdirAll(paths.MachineNetworkInterfacesDir(machineUID), os.ModePerm); err != nil {
		return fmt.Errorf("error creating machine network interfaces directory: %w", err)
	}
	return nil
}
