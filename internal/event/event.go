// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/libvirt-provider/api"
	"github.com/ironcore-dev/libvirt-provider/internal/store"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Handler[e api.Object] interface {
	Handle(event Event[e])
}

type HandlerFunc[E api.Object] func(event Event[E])

func (f HandlerFunc[E]) Handle(event Event[E]) {
	f(event)
}

type HandlerRegistration interface{}

type Source[E api.Object] interface {
	AddHandler(handler Handler[E]) (HandlerRegistration, error)
	RemoveHandler(registration HandlerRegistration) error
}

type Type string

const (
	TypeCreated Type = "Created"
	TypeUpdated Type = "Updated"
	TypeDeleted Type = "Deleted"
	TypeGeneric Type = "Generic"
)

type Event[E api.Object] struct {
	Type   Type
	Object E
}

type ListWatchSourceOptions struct {
	ResyncDuration time.Duration
}

func setListWatchSourceOptionsDefaults(o *ListWatchSourceOptions) {
	if o.ResyncDuration == 0 {
		o.ResyncDuration = 1 * time.Hour
	}
}

func NewListWatchSource[E api.Object](listFunc func(ctx context.Context) ([]E, error), watchFunc func(ctx context.Context) (store.Watch[E], error), opts ListWatchSourceOptions) (*ListWatchSource[E], error) {
	setListWatchSourceOptionsDefaults(&opts)

	return &ListWatchSource[E]{
		listFunc:       listFunc,
		watchFunc:      watchFunc,
		handles:        sets.New[*handle[E]](),
		resyncDuration: opts.ResyncDuration,
	}, nil
}

type ListWatchSource[E api.Object] struct {
	listFunc  func(ctx context.Context) ([]E, error)
	watchFunc func(ctx context.Context) (store.Watch[E], error)

	handlesMu sync.RWMutex
	handles   sets.Set[*handle[E]]

	resyncDuration time.Duration
}

func (s *ListWatchSource[E]) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	var wg sync.WaitGroup

	watch, err := s.watchFunc(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watch: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer watch.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-watch.Events():
				eventType, err := typeFromWatchType(evt.Type)
				if err != nil {
					log.Error(err, "error converting watch event type")
					continue
				}

				s.enqueue(Event[E]{
					Type:   eventType,
					Object: evt.Object,
				})
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		wait.UntilWithContext(ctx, func(ctx context.Context) {
			objs, err := s.listFunc(ctx)
			if err != nil {
				log.Error(err, "failed to list objects")
				return
			}

			for _, obj := range objs {
				s.enqueue(Event[E]{
					Type:   TypeGeneric,
					Object: obj,
				})
			}
		}, s.resyncDuration)
	}()

	return nil
}

func (s *ListWatchSource[E]) AddHandler(handler Handler[E]) (HandlerRegistration, error) {
	s.handlesMu.Lock()
	defer s.handlesMu.Unlock()

	reg := &handle[E]{
		handler: handler,
	}

	s.handles.Insert(reg)

	return reg, nil
}

func (s *ListWatchSource[E]) RemoveHandler(registration HandlerRegistration) error {
	s.handlesMu.Lock()
	defer s.handlesMu.Unlock()

	h, ok := registration.(*handle[E])
	if !ok {
		return fmt.Errorf("invalid handler registration")
	}

	s.handles.Delete(h)

	return nil
}

func (s *ListWatchSource[E]) enqueue(evt Event[E]) {
	for _, handler := range s.handlers() {
		handler.Handle(evt)
	}
}

func (s *ListWatchSource[E]) handlers() []Handler[E] {
	s.handlesMu.RLock()
	defer s.handlesMu.RUnlock()

	handlers := make([]Handler[E], 0, s.handles.Len())
	for hdl := range s.handles {
		handlers = append(handlers, hdl.handler)
	}

	return handlers
}

type handle[E api.Object] struct {
	handler Handler[E]
}

func typeFromWatchType(eventType store.WatchEventType) (Type, error) {
	switch eventType {
	case store.WatchEventTypeCreated:
		return TypeCreated, nil
	case store.WatchEventTypeUpdated:
		return TypeUpdated, nil
	case store.WatchEventTypeDeleted:
		return TypeDeleted, nil
	default:
		return "", fmt.Errorf("unknown watch event type %q", eventType)
	}
}
