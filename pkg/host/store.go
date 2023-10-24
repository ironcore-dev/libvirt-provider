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

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/onmetal/libvirt-driver/pkg/api"
	"github.com/onmetal/libvirt-driver/pkg/store"
	utilssync "github.com/onmetal/libvirt-driver/pkg/sync"
	"github.com/onmetal/libvirt-driver/pkg/utils"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
)

const perm = 0777

type Options[E api.Object] struct {
	//TODO
	Dir            string
	NewFunc        func() E
	CreateStrategy CreateStrategy[E]
}

func NewStore[E api.Object](opts Options[E]) (*Store[E], error) {

	if opts.NewFunc == nil {
		return nil, fmt.Errorf("must specify opts.NewFunc")
	}

	if err := os.MkdirAll(opts.Dir, perm); err != nil {
		return nil, fmt.Errorf("error creating store directory: %w", err)
	}

	return &Store[E]{
		dir: opts.Dir,

		idMu: utilssync.NewMutexMap[string](),

		newFunc:        opts.NewFunc,
		createStrategy: opts.CreateStrategy,

		watches: sets.New[*watch[E]](),
	}, nil
}

type Store[E api.Object] struct {
	dir string

	idMu *utilssync.MutexMap[string]

	newFunc        func() E
	createStrategy CreateStrategy[E]

	watchesMu sync.RWMutex
	watches   sets.Set[*watch[E]]
}

type CreateStrategy[E api.Object] interface {
	PrepareForCreate(obj E)
}

func (s *Store[E]) Create(ctx context.Context, obj E) (E, error) {
	s.idMu.Lock(obj.GetID())
	defer s.idMu.Unlock(obj.GetID())

	_, err := s.get(obj.GetID())
	switch {
	case err == nil:
		return utils.Zero[E](), fmt.Errorf("object with id %q %w", obj.GetID(), store.ErrAlreadyExists)
	case errors.Is(err, store.ErrNotFound):
	default:
		return utils.Zero[E](), fmt.Errorf("failed to get object with id %q %w", obj.GetID(), err)
	}

	if s.createStrategy != nil {
		s.createStrategy.PrepareForCreate(obj)
	}

	obj.SetCreatedAt(time.Now())

	obj, err = s.set(obj)
	if err != nil {
		return utils.Zero[E](), err
	}

	s.enqueue(store.WatchEvent[E]{
		Type:   store.WatchEventTypeCreated,
		Object: obj,
	})

	return obj, nil
}

func (s *Store[E]) Get(ctx context.Context, id string) (E, error) {
	object, err := s.get(id)
	if err != nil {
		return utils.Zero[E](), fmt.Errorf("failed to read object: %w", err)
	}

	return object, nil
}

func (s *Store[E]) Update(ctx context.Context, obj E) (E, error) {
	s.idMu.Lock(obj.GetID())
	defer s.idMu.Unlock(obj.GetID())

	_, err := s.get(obj.GetID())
	if err != nil {
		return utils.Zero[E](), err
	}

	if obj.GetDeletedAt() != nil && len(obj.GetFinalizers()) == 0 {
		if err := s.delete(obj.GetID()); err != nil {
			return utils.Zero[E](), fmt.Errorf("failed to delete object metadata: %w", err)
		}
		return obj, nil
	}

	//Todo: update version
	obj, err = s.set(obj)
	if err != nil {
		return utils.Zero[E](), err
	}

	s.enqueue(store.WatchEvent[E]{
		Type:   store.WatchEventTypeUpdated,
		Object: obj,
	})

	return obj, nil
}

func (s *Store[E]) Delete(ctx context.Context, id string) error {
	s.idMu.Lock(id)
	defer s.idMu.Unlock(id)

	obj, err := s.get(id)
	if err != nil {
		return err
	}

	if len(obj.GetFinalizers()) == 0 {
		return s.delete(id)
	}

	now := time.Now()
	obj.SetDeletedAt(&now)

	if _, err := s.set(obj); err != nil {
		return fmt.Errorf("failed to set object metadata: %w", err)
	}

	s.enqueue(store.WatchEvent[E]{
		Type:   store.WatchEventTypeDeleted,
		Object: obj,
	})

	return nil
}

func (s *Store[E]) List(ctx context.Context) ([]E, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var objs []E
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		object, err := s.get(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read object: %w", err)
		}

		objs = append(objs, object)
	}

	return objs, nil
}

func (s *Store[E]) Watch(ctx context.Context) (store.Watch[E], error) {
	//TODO make configurable
	const bufferSize = 10
	s.watchesMu.Lock()
	defer s.watchesMu.Unlock()

	w := &watch[E]{
		store:  s,
		events: make(chan store.WatchEvent[E], bufferSize),
	}

	s.watches.Insert(w)

	return w, nil
}

func (s *Store[E]) get(id string) (E, error) {
	file, err := os.ReadFile(filepath.Join(s.dir, id))
	if err != nil {
		if !os.IsNotExist(err) {
			return utils.Zero[E](), fmt.Errorf("failed to read file: %w", err)
		}

		return utils.Zero[E](), fmt.Errorf("object with id %q %w", id, store.ErrNotFound)
	}

	obj := s.newFunc()
	if err := json.Unmarshal(file, &obj); err != nil {
		return utils.Zero[E](), fmt.Errorf("failed to unmarshal object: %w", err)
	}

	return obj, err
}

func (s *Store[E]) set(obj E) (E, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return utils.Zero[E](), fmt.Errorf("failed to marshal obj: %w", err)
	}

	if err := os.WriteFile(filepath.Join(s.dir, obj.GetID()), data, 0666); err != nil {
		return utils.Zero[E](), nil
	}

	return obj, nil
}

func (s *Store[E]) delete(id string) error {
	if err := os.Remove(filepath.Join(s.dir, id)); err != nil {
		return fmt.Errorf("failed to delete object from store: %w", err)
	}

	return nil
}

func (s *Store[E]) watchHandlers() []*watch[E] {
	s.watchesMu.RLock()
	defer s.watchesMu.RUnlock()

	return s.watches.UnsortedList()
}

func (s *Store[E]) enqueue(evt store.WatchEvent[E]) {
	for _, handler := range s.watchHandlers() {
		select {
		case handler.events <- evt:
		default:
		}
	}
}
