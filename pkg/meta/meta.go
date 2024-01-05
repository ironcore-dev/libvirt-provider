// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

type LibvirtProviderMetadata struct {
	IRIMmachineLabels string    `xml:"irimachinelabels"`
	ShutdownTimestamp time.Time `xml:"shutdown_timestamp,omitempty"`
}

// Since go does not support XML namespaces easily (see https://github.com/golang/go/issues/9519),
// we need to have two separate structs: One for marshalling, one for unmarshalling.
// TODO: Watch the issue and clean up once resolved.

func (m *LibvirtProviderMetadata) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "libvirtprovider:metadata"
	return e.EncodeElement(&providerPrefixMetadata{
		XMLNS:             "https://github.com/ironcore-dev/libvirt-provider",
		IRIMmachineLabels: m.IRIMmachineLabels,
		ShutdownTimestamp: timeFormatRFC3339String(m.ShutdownTimestamp),
	}, start)
}

// MarshalXMLWithoutNamespace generate xml string without namespace prefix.
// Namespace prefix cannot be use during update of domain metadata.
func (m *LibvirtProviderMetadata) MarshalXMLWithoutNamespace(e *xml.Encoder) error {
	start := xml.StartElement{}
	start.Name.Local = "metadata"
	return e.EncodeElement(&withoutPrefixMetadata{
		XMLName:           start.Name,
		IRIMmachineLabels: m.IRIMmachineLabels,
		ShutdownTimestamp: timeFormatRFC3339String(m.ShutdownTimestamp),
	}, start)
}

func (m *LibvirtProviderMetadata) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var unmarshal withoutPrefixMetadata
	err := d.DecodeElement(&unmarshal, &start)
	if err != nil {
		return err
	}

	if unmarshal.ShutdownTimestamp != "" {
		m.ShutdownTimestamp, err = time.Parse(time.RFC3339, unmarshal.ShutdownTimestamp)
		if err != nil {
			return fmt.Errorf("cannot unmarshal shutdown timestamp: %w", err)
		}
	}

	m.IRIMmachineLabels = unmarshal.IRIMmachineLabels
	return nil
}

type providerPrefixMetadata struct {
	XMLName           xml.Name `xml:"libvirtprovider:metadata"`
	XMLNS             string   `xml:"xmlns:libvirtprovider,attr"`
	IRIMmachineLabels string   `xml:"libvirtprovider:irimachinelabels"`
	ShutdownTimestamp string   `xml:"libvirtprovider:shutdown_timestamp,omitempty"`
}

type withoutPrefixMetadata struct {
	XMLName           xml.Name `xml:"metadata"`
	IRIMmachineLabels string   `xml:"irimachinelabels"`
	ShutdownTimestamp string   `xml:"shutdown_timestamp,omitempty"`
}

func IRIMachineLabelsEncoder(data map[string]string) string {
	var builder strings.Builder

	for key, value := range data {
		builder.WriteString(fmt.Sprintf("\n\"%s\": \"%s\"", key, value))
	}

	return builder.String()
}

// timeFormatRFC3339String serve for omit zero timestamp.
// .Format() function return 0001.00... string
func timeFormatRFC3339String(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.Format(time.RFC3339)
}
