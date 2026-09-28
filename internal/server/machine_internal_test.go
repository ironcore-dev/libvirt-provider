// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

// Internal test, plain stdlib: getVolumeFromIRIVolume is unexported, and a ginkgo spec in this
// directory would be collected by the libvirt-backed suite in package server_test.
package server

import (
	"errors"
	"testing"

	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume"
	"github.com/ironcore-dev/libvirt-provider/internal/plugins/volume/localdisk"
)

func TestVolumeConversionRejectsNegativeLocalDiskSize(t *testing.T) {
	server := serverWithLocalDiskPlugin(t)

	spec, err := server.getVolumeFromIRIVolume(&iri.Volume{
		Name:      "disk-1",
		LocalDisk: &iri.LocalDisk{SizeBytes: -1},
	})

	if err == nil {
		t.Fatalf("accepted a negative disk size, expected error for SizeBytes=%d", spec.LocalDisk.Size)
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected %v got %v for invalid negative request", ErrInvalidRequest, err)
	}
}

func TestVolumeConversionAcceptsZeroLocalDiskSize(t *testing.T) {
	server := serverWithLocalDiskPlugin(t)

	spec, err := server.getVolumeFromIRIVolume(&iri.Volume{
		Name:      "disk-1",
		LocalDisk: &iri.LocalDisk{SizeBytes: 0},
	})

	if err != nil {
		t.Fatalf("rejected size 0, which means let the image or the default decide: %v", err)
	}

	if spec.LocalDisk.Size != 0 {
		t.Errorf("expected size 0 passed through untouched, got %d", spec.LocalDisk.Size)
	}
}

func serverWithLocalDiskPlugin(t *testing.T) *Server {
	t.Helper()

	plugins := volume.NewPluginManager()

	if err := plugins.InitPlugins(nil, []volume.Plugin{localdisk.NewPlugin(nil, nil)}); err != nil {
		t.Fatalf("initialising volume plugins: %v", err)
	}

	return &Server{volumePlugins: plugins}
}
