// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"

	"github.com/ironcore-dev/libvirt-provider/api"
)

var (
	ErrNotFound                 = errors.New("not found")
	ErrAlreadyExists            = errors.New("already exists")
	ErrResourceVersionNotLatest = errors.New("resourceVersion is not latest")
)

func IgnoreErrNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}

	return err
}

type Watch[E api.Object] interface {
	Stop()
	Events() <-chan WatchEvent[E]
}

type WatchEvent[E api.Object] struct {
	Type   WatchEventType
	Object E
}

type WatchEventType string

const (
	WatchEventTypeCreated WatchEventType = "Created"
	WatchEventTypeUpdated WatchEventType = "Updated"
	WatchEventTypeDeleted WatchEventType = "Deleted"
)

type Store[E api.Object] interface {
	Create(ctx context.Context, obj E) (E, error)
	Get(ctx context.Context, id string) (E, error)
	Update(ctx context.Context, obj E) (E, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]E, error)

	Watch(ctx context.Context) (Watch[E], error)
}
