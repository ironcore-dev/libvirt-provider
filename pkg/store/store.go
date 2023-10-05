// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"context"
	"errors"
	"github.com/onmetal/libvirt-driver/pkg/api"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
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
