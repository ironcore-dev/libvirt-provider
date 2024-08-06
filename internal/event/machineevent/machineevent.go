// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package machineevent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gogo/protobuf/proto"
	irievent "github.com/ironcore-dev/ironcore/iri/apis/event/v1alpha1"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/api"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Interface for recording events
type EventRecorder interface {
	RecordEvent(logr.Logger, api.Metadata, string, string, string)
}

// Interface for listing events
type EventStore interface {
	ListEvents() []*irievent.Event
}

// Options to initialize the machine event store
type EventStoreOptions struct {
	MachineEventMaxEvents      int
	MachineEventTTL            time.Duration
	MachineEventResyncInterval time.Duration
}

// eventStore implements the EventRecorder interface and represents an in-memory event store with TTL for events.
type eventStore struct {
	maxEvents           int               // Maximum number of events in the store
	events              []*irievent.Event // Slice of events
	mutex               sync.Mutex        // Mutex for thread safety
	eventTTL            time.Duration     // TTL for events
	eventResyncInterval time.Duration     // Resync interval for event store's TTL expiration check
	head                int               // Index of the oldest event
	count               int               // Current number of events in the store
	log                 logr.Logger       // Logger for logging overridden events
}

// NewEventStore creates a new EventStore with a fixed number of events and set TTL for events.
func NewEventStore(log logr.Logger, opts EventStoreOptions) *eventStore {
	return &eventStore{
		maxEvents:           opts.MachineEventMaxEvents,
		events:              make([]*irievent.Event, opts.MachineEventMaxEvents),
		eventTTL:            opts.MachineEventTTL,
		eventResyncInterval: opts.MachineEventResyncInterval,
		head:                0,
		count:               0,
		log:                 log,
	}
}

// RecordEvent retrieves metadata from the provided API, logs any errors encountered,
// and records an event with the obtained metadata if successful.
func (es *eventStore) RecordEvent(log logr.Logger, apiMetadata api.Metadata, eventType, reason, message string) {
	if metadata, err := api.GetObjectMetadata(apiMetadata); err != nil {
		log.Error(err, "error getting iri metadata")
	} else {
		es.recordEvent(metadata, eventType, reason, message)
	}
}

// recordEvent adds a new Event to the store. Implements the EventRecorder interface.
func (es *eventStore) recordEvent(metadata *irimeta.ObjectMetadata, eventType, reason, message string) {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	// Calculate the index where the new event will be inserted
	index := (es.head + es.count) % es.maxEvents

	// If the store is full, log and overwrite the oldest event and move the head
	if es.count == es.maxEvents {
		es.log.V(1).Info("Overriding event", "event", es.events[es.head])
		es.head = (es.head + 1) % es.maxEvents
	} else {
		es.count++
	}

	event := &irievent.Event{
		Spec: &irievent.EventSpec{
			InvolvedObjectMeta: metadata,
			Type:               eventType,
			Reason:             reason,
			Message:            message,
			EventTime:          time.Now().Unix(),
		},
	}

	es.events[index] = event
}

// removeExpiredEvents checks and removes events whose TTL has expired.
func (es *eventStore) removeExpiredEvents() {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	now := time.Now()

	for es.count > 0 {
		index := es.head % es.maxEvents
		event := es.events[index]
		eventTime := time.Unix(event.Spec.EventTime, 0)
		eventTimeWithDuration := eventTime.Add(es.eventTTL)

		if eventTimeWithDuration.After(now) {
			break
		}

		// Clear the reference to the expired event
		es.events[index] = nil
		es.head = (es.head + 1) % es.maxEvents
		es.count--
	}
}

// Start initializes and starts the event store's TTL expiration check.
func (es *eventStore) Start(ctx context.Context) {
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		es.removeExpiredEvents()
	}, es.eventResyncInterval)
}

// ListEvents returns a copy of all events currently in the store.
func (es *eventStore) ListEvents() []*irievent.Event {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	result := make([]*irievent.Event, 0, es.count)
	for i := 0; i < es.count; i++ {
		index := (es.head + i) % es.maxEvents
		// Create a deep copy of the event to break the reference
		clone, ok := proto.Clone(es.events[index]).(*irievent.Event)
		if !ok {
			es.log.Error(fmt.Errorf("failed to clone event: %s", es.events[index]), "assertion error")
			continue
		}
		result = append(result, clone)
	}

	return result
}
