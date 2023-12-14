// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package libvirt_dialer

import (
	"bytes"
	"encoding/binary"

	xdr "github.com/davecgh/go-xdr/xdr2"
	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket"
	"github.com/google/uuid"
)

const (
	Program         = 0x20008086
	ProtocolVersion = 1
)

var testAuthReply = []byte{
	0x00, 0x00, 0x00, 0x24,
	0x20, 0x00, 0x80, 0x86,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x42,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00,
}

var testConnectReply = []byte{
	0x00, 0x00, 0x00, 0x1c,
	0x20, 0x00, 0x80, 0x86,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

var testDisconnectReply = []byte{
	0x00, 0x00, 0x00, 0x1c,
	0x20, 0x00, 0x80, 0x86,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x02,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

func NewUUID() libvirt.UUID {
	b, _ := uuid.New().MarshalBinary()

	var id libvirt.UUID
	copy(id[:], b)

	return id
}

func getHeader(procedure uint32) []byte {
	return getHeaderWithStatus(procedure, socket.StatusOK)
}

func getHeaderWithStatus(procedure uint32, status int) []byte {
	var header bytes.Buffer
	encoder := xdr.NewEncoder(&header)
	encoder.Encode(socket.Header{ //nolint:errcheck
		Program:   Program,
		Version:   ProtocolVersion,
		Procedure: procedure,
		Type:      socket.Reply,
		Serial:    0,
		Status:    uint32(status),
	})

	return header.Bytes()
}

func defineXML(domainsList []libvirt.Domain) []byte {
	header := getHeader(ProcDomainDefineXML)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	encoder.Encode(domainsList[0]) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func successReplay(procedure uint32) []byte {
	header := getHeader(procedure)

	length := 4 + len(header)
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length)) //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)         //nolint:errcheck

	return res.Bytes()
}

func getDomainState() []byte {
	header := getHeader(ProcDomainGetState)

	length := 4 + len(header) + 4 + 4
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))                      //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)                              //nolint:errcheck
	binary.Write(res, binary.BigEndian, uint32(libvirt.DomainRunning))       //nolint:errcheck
	binary.Write(res, binary.BigEndian, uint32(libvirt.DomainRunningBooted)) //nolint:errcheck

	return res.Bytes()
}

func callbackRegisterAny() []byte {
	header := getHeader(ProcConnectDomainEventCallbackRegisterAny)

	length := 4 + len(header) + 4
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length)) //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)         //nolint:errcheck
	binary.Write(res, binary.BigEndian, uint32(1))      //nolint:errcheck

	return res.Bytes()
}

func lookupByName() []byte {
	header := getHeader(ProcDomainLookupByName)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	domain := libvirt.Domain{
		Name: "Domain1",
		UUID: NewUUID(),
		ID:   1,
	}
	encoder.Encode(domain) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func lookupByUUID() []byte {
	header := getHeaderWithStatus(ProcDomainLookupByUUID, socket.StatusError)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	libvirtError := struct {
		Code     uint32
		DomainID uint32
		Padding  uint8
		Message  string
		Level    uint32
	}{
		Code: uint32(libvirt.ErrNoDomain),
	}
	encoder.Encode(libvirtError) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func createXML() []byte {
	header := getHeader(ProcDomainCreateXML)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	domain := libvirt.Domain{
		Name: "Domain1",
		UUID: NewUUID(),
		ID:   1,
	}
	encoder.Encode(domain) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func getXML() []byte {
	header := getHeader(ProcDomainGetXMLDesc)

	xml := `<domain type='kvm' id='1' xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>
  <name>68c505d2-1a80-4def-87bb-1d246d71fb11-machine-bxnsb</name>
  <uuid>68c505d2-1a80-4def-87bb-1d246d71fb11</uuid>
  <metadata><virtlet:metadata xmlns:virtlet="https://github.com/onmetal/virtlet"><virtlet:namespace>testns-jz2vc</virtlet:namespace><virtlet:name>machine-bxnsb</virtlet:name><virtlet:sgx_memory>68719476736</virtlet:sgx_memory><virtlet:sgx_node>0</virtlet:sgx_node></virtlet:metadata></metadata>
  <memory unit="Byte">34359738368</memory>
  <memoryBacking>
    <hugepages></hugepages>
  </memoryBacking>
  <vcpu>4</vcpu>
  <os firmware="efi">
    <type arch="x86_64" machine="pc-q35-6.2">hvm</type>
    <firmware>
      <feature enabled="no" name="secure-boot"></feature>
    </firmware>
    <boot dev="hd"></boot>
  </os>
  <features>
    <acpi></acpi>
  </features>
  <on_poweroff>restart</on_poweroff>
  <on_reboot>restart</on_reboot>
  <on_crash>restart</on_crash>
  <devices>
    <controller type="pci" model="pcie-root"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <controller type="pci" model="pcie-root-port"></controller>
    <serial>
      <target type="pci-serial"></target>
    </serial>
    <console tty="pty">
      <target type="serial"></target>
    </console>
    <channel type="unix">
      <source mode="bind" path="/var/lib/libvirt/qemu/f16x86_64.agent"></source>
      <target type="virtio" name="org.qemu.guest_agent.0"></target>
    </channel>
    <watchdog model="i6300esb" action="reset"></watchdog>
    <rng model="virtio">
      <rate bytes="512"></rate>
      <backend model="random"></backend>
    </rng>
  </devices>
  <qemu:commandline>
    <arg value="-cpu"></arg>
    <arg value="host,+sgx,+sgx2"></arg>
    <arg value="-object"></arg>
    <arg value="memory-backend-epc,id=mem1,size=68719476736B,prealloc=on"></arg>
    <arg value="-M"></arg>
    <arg value="sgx-epc.0.memdev=mem1,sgx-epc.0.node=0"></arg>
  </qemu:commandline>
</domain>`

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	encoder.Encode(xml) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func emptyDomainList(domainsList []libvirt.Domain) []byte {
	header := getHeader(ProcConnectListAllDomains)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	encoder.Encode(domainsList) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes()) + 4
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))           //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)                   //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes())          //nolint:errcheck
	binary.Write(res, binary.BigEndian, uint32(len(domainsList))) //nolint:errcheck

	return res.Bytes()
}

func capabilities() []byte {
	xml := `
<capabilities>

  <host>
    <uuid>3c2ccdec-4995-11ea-9f1a-0a94efa937a9</uuid>
    <cpu>
      <arch>x86_64</arch>
      <model>Cascadelake-Server-noTSX</model>
      <vendor>Intel</vendor>
      <microcode version='83899138'/>
      <signature family='6' model='85' stepping='7'/>
      <counter name='tsc' frequency='2194843000' scaling='yes'/>
      <topology sockets='1' dies='1' cores='4' threads='2'/>
      <maxphysaddr mode='emulate' bits='46'/>
      <feature name='ibrs-all'/>
      <feature name='skip-l1dfl-vmentry'/>
      <feature name='mds-no'/>
      <feature name='pschange-mc-no'/>
      <pages unit='KiB' size='4'/>
      <pages unit='KiB' size='2048'/>
      <pages unit='KiB' size='1048576'/>
    </cpu>
    <power_management>
      <suspend_mem/>
    </power_management>
    <iommu support='yes'/>
    <migration_features>
      <live/>
      <uri_transports>
        <uri_transport>tcp</uri_transport>
        <uri_transport>rdma</uri_transport>
      </uri_transports>
    </migration_features>
    <topology>
      <cells num='2'>
        <cell id='0'>
          <memory unit='KiB'>395965260</memory>
          <pages unit='KiB' size='4'>46562515</pages>
          <pages unit='KiB' size='2048'>0</pages>
          <pages unit='KiB' size='1048576'>200</pages>
          <distances>
            <sibling id='0' value='10'/>
            <sibling id='1' value='21'/>
          </distances>
          <cpus num='4'>
            <cpu id='0' socket_id='0' die_id='0' core_id='0' siblings='0,2'/>
            <cpu id='1' socket_id='0' die_id='0' core_id='1' siblings='1,3'/>
            <cpu id='2' socket_id='0' die_id='0' core_id='2' siblings='0,2'/>
            <cpu id='3' socket_id='0' die_id='0' core_id='3' siblings='1,3'/>
          </cpus>
        </cell>
        <cell id='1'>
          <memory unit='KiB'>396322232</memory>
          <pages unit='KiB' size='4'>46646638</pages>
          <pages unit='KiB' size='2048'>10</pages>
          <pages unit='KiB' size='1048576'>200</pages>
          <distances>
            <sibling id='0' value='21'/>
            <sibling id='1' value='10'/>
          </distances>
          <cpus num='4'>
            <cpu id='4' socket_id='1' die_id='0' core_id='0' siblings='4,6'/>
            <cpu id='5' socket_id='1' die_id='0' core_id='1' siblings='5,7'/>
            <cpu id='6' socket_id='1' die_id='0' core_id='2' siblings='4,6'/>
            <cpu id='7' socket_id='1' die_id='0' core_id='3' siblings='5,7'/>
          </cpus>
        </cell>
      </cells>
    </topology>
    <cache>
      <bank id='0' level='3' type='both' size='16896' unit='KiB' cpus='0-3'/>
      <bank id='1' level='3' type='both' size='16896' unit='KiB' cpus='4-7'/>
    </cache>
    <secmodel>
      <model>none</model>
      <doi>0</doi>
    </secmodel>
    <secmodel>
      <model>dac</model>
      <doi>0</doi>
      <baselabel type='kvm'>+64055:+64055</baselabel>
      <baselabel type='qemu'>+64055:+64055</baselabel>
    </secmodel>
  </host>

  <guest>
    <os_type>hvm</os_type>
    <arch name='x86_64'>
      <wordsize>64</wordsize>
      <emulator>/usr/bin/qemu-system-x86_64</emulator>
      <machine maxCpus='288'>pc-q35-6.2</machine>
      <machine maxCpus='255'>pc-q35-2.5</machine>
      <machine maxCpus='255'>pc-i440fx-3.0</machine>
      <machine maxCpus='288'>pc-q35-2.11</machine>
      <domain type='qemu'/>
      <domain type='kvm'/>
    </arch>
    <features>
      <acpi default='on' toggle='yes'/>
      <apic default='on' toggle='no'/>
      <cpuselection/>
      <deviceboot/>
      <disksnapshot default='on' toggle='no'/>
    </features>
  </guest>

</capabilities>`

	header := getHeader(ProcDomainDefineXML)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	encoder.Encode(xml) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func nodeInfo() []byte {
	header := getHeader(ProcNodeGetInfo)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	nodeGetInfo := libvirt.NodeGetInfoRet{
		Nodes:   2,
		Sockets: 1,
		Cores:   2,
		Threads: 2,
	}
	encoder.Encode(nodeGetInfo) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func domainInfo() []byte {
	header := getHeader(ProcDomainGetInfo)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	nodeGetInfo := libvirt.DomainGetInfoRet{
		NrVirtCPU: 1,
	}
	encoder.Encode(nodeGetInfo) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func vcpuPinInfo() []byte {
	header := getHeader(ProcDomainGetVCPUPinInfo)

	var payload bytes.Buffer
	encoder := xdr.NewEncoder(&payload)
	nodeGetInfo := libvirt.DomainGetVcpuPinInfoRet{
		Cpumaps: []byte{0x03},
		Num:     1,
	}
	encoder.Encode(nodeGetInfo) //nolint:errcheck

	length := 4 + len(header) + len(payload.Bytes())
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))  //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)          //nolint:errcheck
	binary.Write(res, binary.BigEndian, payload.Bytes()) //nolint:errcheck

	return res.Bytes()
}

func freeNodes() []byte {
	header := getHeader(ProcNodeGetFreePages)

	pages := []uint64{100, 100, 100, 150, 150, 150}

	length := 4 + len(header) + 6*8 + 4
	buf := make([]byte, 0, length)
	res := bytes.NewBuffer(buf)

	binary.Write(res, binary.BigEndian, uint32(length))     //nolint:errcheck
	binary.Write(res, binary.BigEndian, header)             //nolint:errcheck
	binary.Write(res, binary.BigEndian, uint32(len(pages))) //nolint:errcheck
	binary.Write(res, binary.BigEndian, pages)              //nolint:errcheck

	return res.Bytes()
}
