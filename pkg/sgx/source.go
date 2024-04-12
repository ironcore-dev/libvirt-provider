// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package sgx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	core "github.com/ironcore-dev/ironcore/api/core/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/resources/sources"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	ResourceMemorySGX           core.ResourceName = "memory.epc.sgx"
	ResourceMemorySGXNumaPrefix core.ResourceName = "memory.epc.sgx.numa."

	nodeDir          = "/sys/devices/system/node"
	SourceSGX string = "sgx"
)

var ErrSourceSGXNumaUnavailable = errors.New("numa zones aren't available")

type SGX struct {
	availableResources core.ResourceList
	log                logr.Logger
}

func NewSourceSGX(_ sources.Options) *SGX {
	return &SGX{}
}

func (c *SGX) Init(_ context.Context) (sets.Set[core.ResourceName], error) {
	c.log = ctrl.Log.WithName("source-sgx")

	// copy from https://github.com/lrita/numa/blob/master/numa_linux.go#L236
	_, _, e1 := syscall.Syscall6(syscall.SYS_GET_MEMPOLICY, 0, 0, 0, 0, 0, 0)
	if e1 == syscall.ENOSYS {
		return nil, ErrSourceSGXNumaUnavailable
	}

	err := c.loadTotalMemory()
	if err != nil {
		return nil, err
	}

	return c.getSupportedResources(), nil
}

func (c *SGX) GetName() string {
	return "sgx"
}

func (c *SGX) Modify(resources core.ResourceList) error {
	return nil
}

func (c *SGX) CalculateMachineClassQuantity(requiredResources core.ResourceList) int64 {
	sgx, ok := requiredResources[ResourceMemorySGX]
	if !ok {
		return sources.QuantityCountIgnore
	}
	count := int64(0)
	for _, quantity := range c.availableResources {
		numaCount := int64(math.Floor(float64(quantity.Value()) / float64(sgx.Value())))
		if numaCount < 0 {
			numaCount = 0
		}

		count += numaCount
	}

	return count
}

func (c *SGX) Allocate(requiredResources core.ResourceList) (core.ResourceList, error) {
	key, requiredQuantity, found := FindSGXNumaResource(requiredResources)
	if found {
		quantity, ok := c.availableResources[key]
		if !ok {
			return nil, fmt.Errorf("failed to allocate resource %s: %w", key, sources.ErrSourceResourceUnsupport)
		}

		quantity.Sub(*requiredQuantity)
		c.availableResources[key] = quantity
		return core.ResourceList{key: *requiredQuantity}, nil
	}

	requiredSGXMem, ok := requiredResources[ResourceMemorySGX]
	if !ok {
		// machine doesn't need sgx
		return nil, nil
	}

	zone, quantity := c.findNumaWithMostAvailableResources()
	quantity.Sub(requiredSGXMem)
	if quantity.Sign() == -1 {
		return nil, fmt.Errorf("failed to allocate resource %s: %w", zone, sources.ErrResourceNotAvailable)
	}

	c.availableResources[zone] = quantity
	return core.ResourceList{zone: requiredSGXMem}, nil
}

func (c *SGX) Deallocate(deallocateResources core.ResourceList) []core.ResourceName {
	key, requiredQuantity, found := FindSGXNumaResource(deallocateResources)
	if !found {
		return nil
	}

	quantity := c.availableResources[key]
	quantity.Add(*requiredQuantity)
	c.availableResources[key] = quantity

	return []core.ResourceName{key}
}

func (c *SGX) GetAvailableResources() core.ResourceList {
	return c.availableResources.DeepCopy()
}

func (c *SGX) loadTotalMemory() error {
	files, err := os.ReadDir(nodeDir)
	if err != nil {
		return err
	}

	c.availableResources = make(core.ResourceList)

	for _, f := range files {
		if !strings.HasPrefix(f.Name(), "node") {
			continue
		}

		totalMemory, err := c.getNumaTotalBytes(f.Name())
		if err != nil {
			return err
		}

		// behind node is number of numa zone
		numaZoneResourceName := ResourceMemorySGXNumaPrefix + core.ResourceName(f.Name()[4:])

		quantity := resource.NewQuantity(totalMemory, resource.BinarySI)
		c.availableResources[numaZoneResourceName] = *quantity
	}

	return nil
}

func (c *SGX) getNumaTotalBytes(node string) (int64, error) {
	const maxCharacters = 20
	fd, err := os.Open(filepath.Join(nodeDir, node, "x86", "sgx_total_bytes"))
	if err != nil {
		return 0, err
	}

	defer func() {
		defErr := fd.Close()
		if defErr != nil {
			c.log.Error(defErr, "cannot close file %s", fd.Name())
		}
	}()

	// additional protection for reading corrupted files
	reader := io.LimitReader(fd, maxCharacters)
	content, err := io.ReadAll(reader)
	if err != nil {
		return 0, nil
	}

	content = bytes.TrimSpace(content)

	return strconv.ParseInt(string(content), 10, 64)
}

func (c *SGX) findNumaWithMostAvailableResources() (core.ResourceName, resource.Quantity) {
	var numa core.ResourceName
	var availableQuantity resource.Quantity
	for key, quantity := range c.availableResources {
		if !strings.HasPrefix(string(key), string(ResourceMemorySGXNumaPrefix)) {
			continue
		}

		if availableQuantity.Cmp(quantity) == -1 {
			availableQuantity = quantity
			numa = key
		}
	}

	return numa, availableQuantity
}

func (c *SGX) getSupportedResources() sets.Set[core.ResourceName] {
	resources := sets.New[core.ResourceName](ResourceMemorySGX)
	for key := range c.availableResources {
		resources.Insert(key)
	}

	return resources
}

func FindSGXNumaResource(resources core.ResourceList) (core.ResourceName, *resource.Quantity, bool) {
	for key, quantity := range resources {
		if strings.HasPrefix(string(key), string(ResourceMemorySGXNumaPrefix)) {
			return key, &quantity, true
		}
	}

	return "", nil, false
}
