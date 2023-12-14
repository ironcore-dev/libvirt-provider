// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package meta

import "encoding/xml"

type LibvirtProviderMetadata struct {
	Namespace string `xml:"namespace"`
	Name      string `xml:"name"`
	SGXMemory *int64 `xml:"sgx_memory,omitempty"`
	SGXNode   *int   `xml:"sgx_node,omitempty"`
}

// Since go does not support XML namespaces easily (see https://github.com/golang/go/issues/9519),
// we need to have two separate structs: One for marshalling, one for unmarshalling.
// TODO: Watch the issue and clean up once resolved.

func (m *LibvirtProviderMetadata) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "libvirtprovider:metadata"
	return e.EncodeElement(&marshalMetadata{
		XMLNS:     "https://github.com/ironcore-dev/libvirt-provider",
		Namespace: m.Namespace,
		Name:      m.Name,
		SGXMemory: m.SGXMemory,
		SGXNode:   m.SGXNode,
	}, start)
}

func (m *LibvirtProviderMetadata) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var unmarshal unmarshalMetadata
	if err := d.DecodeElement(&unmarshal, &start); err != nil {
		return err
	}

	m.Namespace = unmarshal.Namespace
	m.Name = unmarshal.Name
	m.SGXMemory = unmarshal.SGXMemory
	m.SGXNode = unmarshal.SGXNode
	return nil
}

type marshalMetadata struct {
	XMLName   xml.Name `xml:"libvirtprovider:metadata"`
	XMLNS     string   `xml:"xmlns:libvirtprovider,attr"`
	Namespace string   `xml:"libvirtprovider:namespace"`
	Name      string   `xml:"libvirtprovider:name"`
	SGXMemory *int64   `xml:"libvirtprovider:sgx_memory,omitempty"`
	SGXNode   *int     `xml:"libvirtprovider:sgx_node,omitempty"`
}

type unmarshalMetadata struct {
	XMLName   xml.Name `xml:"metadata"`
	Namespace string   `xml:"namespace"`
	Name      string   `xml:"name"`
	SGXMemory *int64   `xml:"sgx_memory,omitempty"`
	SGXNode   *int     `xml:"sgx_node,omitempty"`
}
