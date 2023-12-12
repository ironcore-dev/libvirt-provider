// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package libvirt_dialer

import (
	"encoding/binary"
	"net"

	"github.com/digitalocean/go-libvirt"
)

const (
	ProcConnectOpen                           = 1
	ProcConnectClose                          = 2
	ProcNodeGetInfo                           = 6
	ProcConnectGetCapabilities                = 7
	ProcDomainCreate                          = 9
	ProcDomainCreateXML                       = 10
	ProcDomainDefineXML                       = 11
	ProcDomainDestroy                         = 12
	ProcDomainGetXMLDesc                      = 14
	ProcDomainGetInfo                         = 16
	ProcDomainLookupByName                    = 23
	ProcDomainLookupByUUID                    = 24
	ProcDomainUndefine                        = 35
	ProcAuthList                              = 66
	ProcSecretUndefine                        = 146
	ProcDomainGetState                        = 212
	ProcDomainGetVCPUPinInfo                  = 230
	ProcConnectListAllDomains                 = 273
	ProcConnectDomainEventCallbackRegisterAny = 316
	ProcNodeGetFreePages                      = 340
)

type MockLibvirtDilaer struct {
	disconnected chan struct{}
	domains      []libvirt.Domain
}

func NewMockDialer(domains []libvirt.Domain) *MockLibvirtDilaer {
	m := &MockLibvirtDilaer{
		disconnected: make(chan struct{}),
		domains:      domains,
	}
	close(m.disconnected)
	return m
}

func (m *MockLibvirtDilaer) handle(conn net.Conn) {
	for {
		buf := make([]byte, 28)
		conn.Read(buf) //nolint:errcheck
		procedure := binary.BigEndian.Uint32(buf[12:16])
		serial := binary.BigEndian.Uint32(buf[20:24])

		m.handleRemote(procedure, serial, conn)

		select {
		case <-m.disconnected:
			return
		default:

		}
	}
}

func (m *MockLibvirtDilaer) reply(buf []byte, serial uint32) []byte {
	binary.BigEndian.PutUint32(buf[20:24], serial)
	return buf
}

func (m *MockLibvirtDilaer) handleRemote(procedure, serial uint32, conn net.Conn) {
	switch procedure {
	case ProcConnectOpen:
		conn.Write(m.reply(testConnectReply, serial)) //nolint:errcheck
	case ProcConnectClose:
		conn.Write(m.reply(testDisconnectReply, serial)) //nolint:errcheck
	case ProcNodeGetInfo:
		conn.Write(m.reply(nodeInfo(), serial)) //nolint:errcheck
	case ProcConnectGetCapabilities:
		conn.Write(m.reply(capabilities(), serial)) //nolint:errcheck
	case ProcAuthList:
		conn.Write(m.reply(testAuthReply, serial)) //nolint:errcheck
	case ProcDomainDefineXML:
		conn.Write(m.reply(defineXML(m.domains), serial)) //nolint:errcheck
	case ProcDomainDestroy:
		conn.Write(m.reply(successReplay(ProcDomainDestroy), serial)) //nolint:errcheck
	case ProcDomainGetXMLDesc:
		conn.Write(m.reply(getXML(), serial)) //nolint:errcheck
	case ProcDomainGetInfo:
		conn.Write(m.reply(domainInfo(), serial)) //nolint:errcheck
	case ProcDomainLookupByName:
		conn.Write(m.reply(lookupByName(), serial)) //nolint:errcheck
	case ProcDomainLookupByUUID:
		conn.Write(m.reply(lookupByUUID(), serial)) //nolint:errcheck
	case ProcDomainCreateXML:
		conn.Write(m.reply(createXML(), serial)) //nolint:errcheck
	case ProcDomainCreate:
		conn.Write(m.reply(successReplay(ProcDomainCreate), serial)) //nolint:errcheck
	case ProcDomainGetState:
		conn.Write(m.reply(getDomainState(), serial)) //nolint:errcheck
	case ProcDomainUndefine:
		conn.Write(m.reply(successReplay(ProcDomainUndefine), serial)) //nolint:errcheck
	case ProcDomainGetVCPUPinInfo:
		conn.Write(m.reply(vcpuPinInfo(), serial)) //nolint:errcheck
	case ProcConnectListAllDomains:
		conn.Write(m.reply(emptyDomainList(m.domains), serial)) //nolint:errcheck
	case ProcConnectDomainEventCallbackRegisterAny:
		conn.Write(m.reply(callbackRegisterAny(), serial)) //nolint:errcheck
	case ProcNodeGetFreePages:
		conn.Write(m.reply(freeNodes(), serial)) //nolint:errcheck
	case ProcSecretUndefine:
		conn.Write(m.reply(successReplay(ProcSecretUndefine), serial)) //nolint:errcheck
	}
}

func (m *MockLibvirtDilaer) Close() error {
	select {
	case <-m.disconnected:
		return nil
	default:
		close(m.disconnected)
	}
	return nil
}

func (m *MockLibvirtDilaer) Dial() (net.Conn, error) {
	serv, clnt := net.Pipe()
	m.disconnected = make(chan struct{})

	go func() {
		m.handle(serv)
	}()
	return clnt, nil
}
