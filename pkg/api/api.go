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

package api

import "time"

type Metadata struct {
	ID          string            `json:"id"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`

	CreatedAt  time.Time  `json:"createdAt"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
	Generation int64      `json:"generation"`

	Finalizers []string `json:"finalizers,omitempty"`
}

func (m *Metadata) GetID() string {
	return m.ID
}

func (m *Metadata) GetAnnotations() map[string]string {
	return m.Annotations
}

func (m *Metadata) GetLabels() map[string]string {
	return m.Labels
}

func (m *Metadata) GetCreatedAt() time.Time {
	return m.CreatedAt
}

func (m *Metadata) GetDeletedAt() *time.Time {
	return m.DeletedAt
}

func (m *Metadata) GetGeneration() int64 {
	return m.Generation
}

func (m *Metadata) GetFinalizers() []string {
	return m.Finalizers
}

func (m *Metadata) SetID(id string) {
	m.ID = id
}

func (m *Metadata) SetAnnotations(annotations map[string]string) {
	m.Annotations = annotations
}

func (m *Metadata) SetLabels(labels map[string]string) {
	m.Labels = labels
}

func (m *Metadata) SetCreatedAt(createdAt time.Time) {
	m.CreatedAt = createdAt
}

func (m *Metadata) SetDeletedAt(deleted *time.Time) {
	m.DeletedAt = deleted
}

func (m *Metadata) SetGeneration(generation int64) {
	m.Generation = generation
}

func (m *Metadata) SetFinalizers(finalizers []string) {
	m.Finalizers = finalizers
}

type Object interface {
	GetID() string
	GetAnnotations() map[string]string
	GetLabels() map[string]string
	GetCreatedAt() time.Time
	GetDeletedAt() *time.Time
	GetGeneration() int64
	GetFinalizers() []string

	SetID(id string)
	SetAnnotations(annotations map[string]string)
	SetLabels(labels map[string]string)
	SetCreatedAt(createdAt time.Time)
	SetDeletedAt(deleted *time.Time)
	SetGeneration(generation int64)
	SetFinalizers(finalizers []string)
}
