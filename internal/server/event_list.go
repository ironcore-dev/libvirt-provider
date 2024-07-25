// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	irievent "github.com/ironcore-dev/ironcore/iri/apis/event/v1alpha1"
	iri "github.com/ironcore-dev/ironcore/iri/apis/machine/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
)

func (s *Server) filterEvents(events []*irievent.Event, filter *iri.EventFilter) []*irievent.Event {
	if filter == nil {
		return events
	}

	var (
		res []*irievent.Event
		sel = labels.SelectorFromSet(filter.LabelSelector)
	)
	for _, iriEvent := range events {
		if !sel.Matches(labels.Set(iriEvent.Spec.InvolvedObjectMeta.Labels)) {
			continue
		}

		if filter.EventsFromTime > 0 && filter.EventsToTime > 0 {
			if iriEvent.Spec.EventTime < filter.EventsFromTime || iriEvent.Spec.EventTime > filter.EventsToTime {
				continue
			}
		}

		res = append(res, iriEvent)
	}
	return res
}

func (s *Server) ListEvents(ctx context.Context, req *iri.ListEventsRequest) (*iri.ListEventsResponse, error) {
	iriEvents := s.filterEvents(s.eventStore.ListEvents(), req.Filter)

	return &iri.ListEventsResponse{
		Events: iriEvents,
	}, nil
}
