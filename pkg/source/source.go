// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"sync"

	"github.com/digitalocean/go-libvirt"
	"github.com/ironcore-dev/ironcore/api/compute/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/meta"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
	"libvirt.org/go/libvirtxml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var domainLifecycleLog = ctrl.Log.WithName("libvirt").WithName("domain-lifecycle-events")

type DomainLifecycle struct {
	once    sync.Once
	onceErr error

	client  client.Client
	libvirt *libvirt.Libvirt

	out     chan<- event.GenericEvent
	channel *source.Channel
}

func (s *DomainLifecycle) String() string {
	return "libvirt source: domain-lifecycle"
}

func (s *DomainLifecycle) handle(ctx context.Context, evt libvirt.DomainEventLifecycleMsg) {
	domainXML, err := s.libvirt.DomainGetXMLDesc(evt.Dom, libvirt.DomainXMLSecure)
	if err != nil {
		if !libvirt.IsNotFound(err) {
			domainLifecycleLog.Error(err, "Error looking up domain by uuid", "Domain", evt.Dom)
		}
		return
	}

	domain := libvirtxml.Domain{}
	if err := domain.Unmarshal(domainXML); err != nil {
		domainLifecycleLog.Error(err, "Error unmarshalling domain", "Domain", evt.Dom)
		return
	}

	if domain.Metadata == nil || domain.Metadata.XML == "" {
		domainLifecycleLog.V(2).Info("Domain has no metadata and is thus not considered to be managed by virtlet", "Domain", evt.Dom)
		return
	}

	machineMeta := &meta.VirtletMetadata{}
	if err := xml.Unmarshal([]byte(domain.Metadata.XML), machineMeta); err != nil {
		domainLifecycleLog.Error(err, "Error unmarshalling domain metadata")
		return
	}

	machine := &v1alpha1.Machine{}
	machineKey := client.ObjectKey{Namespace: machineMeta.Namespace, Name: machineMeta.Name}
	if err := s.client.Get(ctx, machineKey, machine); err != nil {
		if !errors.IsNotFound(err) {
			domainLifecycleLog.Error(err, "Error getting machine", "MachineKey", machineKey)
			return
		}

		domainLifecycleLog.V(1).Info("Corresponding machine not found", "MachineKey", machineKey)
		return
	}

	select {
	case <-ctx.Done():
		return
	case s.out <- event.GenericEvent{Object: machine}:
	}
}

func (s *DomainLifecycle) syncLoop(ctx context.Context, events <-chan libvirt.DomainEventLifecycleMsg) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}

			s.handle(ctx, evt)
		}
	}
}

func (s *DomainLifecycle) Start(ctx context.Context, handler handler.EventHandler, queue workqueue.RateLimitingInterface, prct ...predicate.Predicate) error {
	if s.libvirt == nil {
		return fmt.Errorf("must initialize with Libvirt")
	}
	if s.client == nil {
		return fmt.Errorf("must initialize with cient")
	}
	if s.out == nil {
		return fmt.Errorf("must initialize out channel")
	}

	s.once.Do(func() {
		var events <-chan libvirt.DomainEventLifecycleMsg
		events, s.onceErr = s.libvirt.LifecycleEvents(ctx)
		if s.onceErr != nil {
			return
		}

		go s.syncLoop(ctx, events)
	})
	if s.onceErr != nil {
		return s.onceErr
	}

	return s.channel.Start(ctx, handler, queue, prct...)
}

func NewDomainLifecycle(libvirt *libvirt.Libvirt, c client.Client) *DomainLifecycle {
	out := make(chan event.GenericEvent, 1024)

	return &DomainLifecycle{
		client:  c,
		libvirt: libvirt,
		out:     out,
		channel: &source.Channel{Source: out},
	}
}
