// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

import "time"

type Metadata struct {
	ID          string            `json:"id"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`

	CreatedAt       time.Time  `json:"createdAt"`
	DeletedAt       *time.Time `json:"deletedAt,omitempty"`
	Generation      int64      `json:"generation"`
	ResourceVersion uint64     `json:"resourceVersion"`

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

func (m *Metadata) GetResourceVersion() uint64 {
	return m.ResourceVersion
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

func (m *Metadata) IncrementResourceVersion() {
	m.ResourceVersion++
}

type Object interface {
	GetID() string
	GetAnnotations() map[string]string
	GetLabels() map[string]string
	GetCreatedAt() time.Time
	GetDeletedAt() *time.Time
	GetGeneration() int64
	GetFinalizers() []string
	GetResourceVersion() uint64

	SetID(id string)
	SetAnnotations(annotations map[string]string)
	SetLabels(labels map[string]string)
	SetCreatedAt(createdAt time.Time)
	SetDeletedAt(deleted *time.Time)
	SetGeneration(generation int64)
	SetFinalizers(finalizers []string)
	IncrementResourceVersion()
}
