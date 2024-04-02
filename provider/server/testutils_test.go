// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"fmt"
	"net"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"
)

func getAnyDomainIPForNetwork(libvirtConn *libvirt.Libvirt, network *libvirt.Network, domain *libvirt.Domain) (string, error) {
	domainXMLData, err := libvirtConn.DomainGetXMLDesc(*domain, 0)
	if err != nil {
		return "", err
	}

	domainXML := &libvirtxml.Domain{}
	err = domainXML.Unmarshal(domainXMLData)
	if err != nil {
		return "", nil
	}

	mac, err := getDomainMAC(domainXML, network.Name)
	if err != nil {
		return "", nil
	}

	leases, _, err := libvirtConn.NetworkGetDhcpLeases(*network, libvirt.OptString{mac}, 1, 0)
	if err != nil {
		return "", err
	}

	// return first ip
	for _, lease := range leases {
		if lease.Mac[0] == mac {
			return lease.Ipaddr, nil
		}
	}

	return "", fmt.Errorf("failed to find ip address for domain %s", domain.Name)
}

func getDomainMAC(domain *libvirtxml.Domain, networkName string) (string, error) {
	for _, netIF := range domain.Devices.Interfaces {
		if isInterfaceInSpecificNetwork(netIF.Source, networkName) && isMACValid(netIF.MAC) {
			return netIF.MAC.Address, nil
		}
	}

	return "", fmt.Errorf("failed to find mac address for network %s", networkName)
}

func isInterfaceInSpecificNetwork(source *libvirtxml.DomainInterfaceSource, networkName string) bool {
	return source != nil && source.Network != nil && source.Network.Network == networkName
}

func isMACValid(mac *libvirtxml.DomainInterfaceMAC) bool {
	return mac != nil && mac.Address != ""
}

func isSSHListenToDefualtPort(ip string) bool {
	timeout := time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "22"), timeout)
	if err != nil {
		return false
	}

	if conn != nil {
		// tests aren't long running so we can ignore errors
		_ = conn.Close()
		return true
	}

	return false
}

func isDomainVMUpAndRunning(g gomega.Gomega, libvirtConn *libvirt.Libvirt, domain *libvirt.Domain, network *libvirt.Network) {
	ip, err := getAnyDomainIPForNetwork(libvirtConn, network, domain)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(isSSHListenToDefualtPort(ip)).Should(gomega.BeTrue())
}

func createOrGetNetwork(networkID *libvirt.Network) error {
	currentNetwork, err := libvirtConn.NetworkLookupByName(networkID.Name)
	if err != nil {
		libvirtErr, ok := err.(libvirt.Error)
		if !ok {
			return err
		}

		if libvirtErr.Code != uint32(libvirt.ErrNoNetwork) {
			return err
		}

		newNetworkXML, err := generateDefaultNetworkXML(networkID.Name)
		if err != nil {
			return err
		}

		newNetworkID, err := libvirtConn.NetworkDefineXML(newNetworkXML)
		if err != nil {
			return err
		}

		networkID.UUID = newNetworkID.UUID
	} else {
		networkID.UUID = currentNetwork.UUID
	}

	// start existing network
	active, err := libvirtConn.NetworkIsActive(*networkID)
	if err != nil {
		return fmt.Errorf("failed to get network '%s' active state: %w", networkID.Name, err)
	}

	if active == 1 {
		return nil
	}

	// create and start defined network
	err = libvirtConn.NetworkCreate(*networkID)
	if err != nil {
		return fmt.Errorf("failed to start network '%s': %w", networkID.Name, err)
	}

	return nil
}

func generateDefaultNetworkXML(name string) (string, error) {
	newNetwork := libvirtxml.Network{
		Name: name,
		Forward: &libvirtxml.NetworkForward{
			Mode: "nat",
			NAT: &libvirtxml.NetworkForwardNAT{
				Ports: []libvirtxml.NetworkForwardNATPort{{Start: 1024, End: 65535}},
			},
		},
		IPs: []libvirtxml.NetworkIP{
			{
				// randomly choosed, hopefully it can be potencial problem for somebody
				Address: "192.168.168.1",
				Netmask: "255.255.255.0",
				DHCP: &libvirtxml.NetworkDHCP{
					Ranges: []libvirtxml.NetworkDHCPRange{
						{
							Start: "192.168.168.2",
							End:   "192.168.168.254",
						},
					},
				},
			},
		},
		Bridge: &libvirtxml.NetworkBridge{
			Name:  "virbrtest0",
			STP:   "on",
			Delay: "0",
		},
	}

	return newNetwork.Marshal()
}

func deleteNetwork(libvirtConn *libvirt.Libvirt, networkID *libvirt.Network) error {
	err := libvirtConn.NetworkDestroy(*networkID)
	if err != nil {
		return err
	}

	return libvirtConn.NetworkUndefine(*networkID)
}
