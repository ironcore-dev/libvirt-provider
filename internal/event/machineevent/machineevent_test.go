// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package machineevent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ironcore-dev/libvirt-provider/api"

	"github.com/go-logr/logr/funcr"
	. "github.com/ironcore-dev/libvirt-provider/internal/event/machineevent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Machine Event Suite")
}

var (
	es          *EventStore
	apiMetadata = api.Metadata{
		ID: "test-id-1234",
		Annotations: map[string]string{
			"libvirt-provider.ironcore.dev/annotations": "{\"key1\":\"value1\", \"key2\":\"value2\"}",
			"libvirt-provider.ironcore.dev/labels":      "{\"downward-api.machinepoollet.ironcore.dev/root-machine-namespace\":\"default\", \"downward-api.machinepoollet.ironcore.dev/root-machine-name\":\"machine1\"}",
		}}
	log = funcr.New(func(prefix, args string) {}, funcr.Options{})
)

const (
	maxEvents = 5
	eventTTL  = 2 * time.Second
	eventType = "TestType"
	reason    = "TestReason"
	message   = "TestMessage"
)

var _ = Describe("Machine EventStore", func() {
	BeforeEach(func() {
		es = NewEventStore(log, maxEvents, eventTTL)
	})

	Context("Initialization", func() {
		It("should initialize events slice with no elements", func() {
			Expect(es.ListEvents()).To(BeEmpty())
		})
	})

	Context("AddEvent", func() {
		It("should add an event to the store", func() {
			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())
			Expect(es.ListEvents()).To(HaveLen(1))
		})

		It("should handle error when retrieving metadata", func() {
			badMetadata := api.Metadata{
				ID: "test-id-1234"}
			err := es.AddEvent(badMetadata, eventType, reason, message)
			Expect(err).To(HaveOccurred())
			Expect(es.ListEvents()).To(HaveLen(0))
		})

		It("should override the oldest event when the store is full", func() {
			for i := 0; i < maxEvents; i++ {
				err := es.AddEvent(apiMetadata, eventType, reason, fmt.Sprintf("%s %d", message, i))
				Expect(err).ToNot(HaveOccurred())
				Expect(es.ListEvents()).To(HaveLen(i + 1))
			}

			err := es.AddEvent(apiMetadata, eventType, reason, "New Event")
			Expect(err).ToNot(HaveOccurred())

			events := es.ListEvents()
			Expect(events).To(HaveLen(maxEvents))
			Expect(events[maxEvents-1].Spec.Message).To(Equal("New Event"))
		})
	})

	Context("RemoveExpiredEvents", func() {
		It("should remove events whose TTL has expired", func() {
			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())
			Expect(es.ListEvents()).To(HaveLen(1))

			Eventually(func(g Gomega) bool {
				es.RemoveExpiredEvents()
				return g.Expect(es.ListEvents()).To(HaveLen(0))
			}, eventTTL+1*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("should not remove events whose TTL has not expired", func() {
			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())
			Expect(es.ListEvents()).To(HaveLen(1))

			es.RemoveExpiredEvents()
			Expect(es.ListEvents()).To(HaveLen(1))
		})
	})

	Context("Start", func() {
		It("should periodically remove expired events", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			resyncInterval := 2 * time.Second
			go es.Start(ctx, log, resyncInterval)

			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())
			Expect(es.ListEvents()).To(HaveLen(1))

			Eventually(func(g Gomega) bool {
				return g.Expect(es.ListEvents()).To(HaveLen(0))
			}, resyncInterval+1*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	Context("ListEvents", func() {
		It("should return all current events", func() {
			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())

			events := es.ListEvents()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Spec.Message).To(Equal(message))
		})

		It("should return a copy of events", func() {
			err := es.AddEvent(apiMetadata, eventType, reason, message)
			Expect(err).ToNot(HaveOccurred())
			events := es.ListEvents()
			Expect(events).To(HaveLen(1))

			events[0].Spec.Message = "Changed Message"

			storedEvents := es.ListEvents()
			Expect(storedEvents[0].Spec.Message).To(Equal(message))
		})
	})
})
