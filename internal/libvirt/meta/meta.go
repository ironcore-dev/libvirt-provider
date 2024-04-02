// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type LibvirtProviderMetadata struct {
	IRIMmachineLabels string `xml:"irimachinelabels"`
}

// Since go does not support XML namespaces easily (see https://github.com/golang/go/issues/9519),
// we need to have two separate structs: One for marshalling, one for unmarshalling.
// TODO: Watch the issue and clean up once resolved.

func (m *LibvirtProviderMetadata) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "libvirtprovider:metadata"
	return e.EncodeElement(&marshalMetadata{
		XMLNS:             "https://github.com/ironcore-dev/libvirt-provider",
		IRIMmachineLabels: m.IRIMmachineLabels,
	}, start)
}

func (m *LibvirtProviderMetadata) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var unmarshal unmarshalMetadata
	if err := d.DecodeElement(&unmarshal, &start); err != nil {
		return err
	}

	m.IRIMmachineLabels = unmarshal.IRIMmachineLabels
	return nil
}

type marshalMetadata struct {
	XMLName           xml.Name `xml:"libvirtprovider:metadata"`
	XMLNS             string   `xml:"xmlns:libvirtprovider,attr"`
	IRIMmachineLabels string   `xml:"libvirtprovider:irimachinelabels"`
}

type unmarshalMetadata struct {
	XMLName           xml.Name `xml:"metadata"`
	IRIMmachineLabels string   `xml:"irimachinelabels"`
}

func IRIMachineLabelsEncoder(data map[string]string) string {
	var builder strings.Builder

	for key, value := range data {
		builder.WriteString(fmt.Sprintf("\n\"%s\": \"%s\"", key, value))
	}

	return builder.String()
}
