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

package networkbridges

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/digitalocean/go-libvirt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"libvirt.org/go/libvirtxml"
)

type Manager interface {
	Allocate(networkUID types.UID) (string, error)
	Free(networkUID types.UID)
}

type manager struct {
	mu sync.RWMutex

	libvirt *libvirt.Libvirt

	allocatedByUID map[types.UID]int
	allocated      sets.Set[int]

	prefix       string
	maxAllocated int
}

func (m *manager) Allocate(networkUID types.UID) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.allocatedByUID[networkUID]; ok {
		return "", fmt.Errorf("network %s already is allocated", networkUID)
	}

	for i := 0; i < m.maxAllocated; i++ {
		if !m.allocated.Has(i) {
			name := fmt.Sprintf("%s%d", m.prefix, i)
			m.allocatedByUID[networkUID] = i
			m.allocated.Insert(i)
			return name, nil
		}
	}
	return "", fmt.Errorf("no bridge available")
}

func (m *manager) Free(networkUID types.UID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	allocated, ok := m.allocatedByUID[networkUID]
	if !ok {
		return
	}

	delete(m.allocatedByUID, networkUID)
	m.allocated.Delete(allocated)
}

type Options struct {
	Prefix       string
	MaxAllocated int
}

func setOptionsDefaults(o *Options) {
	if o.Prefix == "" {
		o.Prefix = "bridge"
	}
	if o.MaxAllocated <= 0 {
		o.MaxAllocated = 100
	}
}

func NewManager(lv *libvirt.Libvirt, opts Options) (Manager, error) {
	setOptionsDefaults(&opts)
	nets, _, err := lv.ConnectListAllNetworks(1000, 0)
	if err != nil {
		return nil, err
	}

	mgr := &manager{
		libvirt:        lv,
		allocatedByUID: make(map[types.UID]int),
		allocated:      sets.New[int](),
		prefix:         opts.Prefix,
		maxAllocated:   opts.MaxAllocated,
	}

	for _, net := range nets {
		data, err := lv.NetworkGetXMLDesc(net, 0)
		if err != nil {
			return nil, err
		}

		networkDesc := &libvirtxml.Network{}
		if err := networkDesc.Unmarshal(data); err != nil {
			return nil, err
		}

		bridge := networkDesc.Bridge
		if bridge == nil || !strings.HasPrefix(bridge.Name, opts.Prefix) {
			continue
		}

		id, err := strconv.Atoi(strings.TrimPrefix(bridge.Name, opts.Prefix))
		if err != nil {
			continue
		}

		mgr.allocated.Insert(id)
		mgr.allocatedByUID[types.UID(net.UUID[:])] = id
	}
	return mgr, nil
}
