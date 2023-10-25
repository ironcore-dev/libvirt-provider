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

package apiutils

import (
	"encoding/json"
	"fmt"

	"github.com/onmetal/controller-utils/metautils"
	machinev1alpha1 "github.com/onmetal/libvirt-driver/driver/api/v1alpha1"
	"github.com/onmetal/libvirt-driver/pkg/api"
	orimeta "github.com/onmetal/onmetal-api/ori/apis/meta/v1alpha1"
)

func GetObjectMetadata(o api.Metadata) (*orimeta.ObjectMetadata, error) {
	annotations, err := GetAnnotationsAnnotation(o)
	if err != nil {
		return nil, err
	}

	labels, err := GetLabelsAnnotation(o)
	if err != nil {
		return nil, err
	}

	var deletedAt int64
	if o.DeletedAt != nil && !o.DeletedAt.IsZero() {
		deletedAt = o.DeletedAt.UnixNano()
	}

	return &orimeta.ObjectMetadata{
		Id:          o.ID,
		Annotations: annotations,
		Labels:      labels,
		Generation:  o.GetGeneration(),
		CreatedAt:   o.CreatedAt.UnixNano(),
		DeletedAt:   deletedAt,
	}, nil
}

func SetObjectMetadata(o api.Object, metadata *orimeta.ObjectMetadata) error {
	if err := SetAnnotationsAnnotation(o, metadata.Annotations); err != nil {
		return err
	}
	if err := SetLabelsAnnotation(o, metadata.Labels); err != nil {
		return err
	}
	return nil
}

func SetLabelsAnnotation(o api.Object, labels map[string]string) error {
	data, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("error marshalling labels: %w", err)
	}
	metautils.SetAnnotation(o, machinev1alpha1.LabelsAnnotation, string(data))
	return nil
}

func GetLabelsAnnotation(o api.Metadata) (map[string]string, error) {
	data, ok := o.GetAnnotations()[machinev1alpha1.LabelsAnnotation]
	if !ok {
		return nil, fmt.Errorf("object has no labels at %s", machinev1alpha1.LabelsAnnotation)
	}

	var labels map[string]string
	if err := json.Unmarshal([]byte(data), &labels); err != nil {
		return nil, err
	}

	return labels, nil
}

func SetAnnotationsAnnotation(o api.Object, annotations map[string]string) error {
	data, err := json.Marshal(annotations)
	if err != nil {
		return fmt.Errorf("error marshalling annotations: %w", err)
	}
	metautils.SetAnnotation(o, machinev1alpha1.AnnotationsAnnotation, string(data))

	return nil
}

func GetAnnotationsAnnotation(o api.Metadata) (map[string]string, error) {
	data, ok := o.GetAnnotations()[machinev1alpha1.AnnotationsAnnotation]
	if !ok {
		return nil, fmt.Errorf("object has no annotations at %s", machinev1alpha1.AnnotationsAnnotation)
	}

	var annotations map[string]string
	if err := json.Unmarshal([]byte(data), &annotations); err != nil {
		return nil, err
	}

	return annotations, nil
}

func SetManagerLabel(o api.Object, manager string) {
	metautils.SetLabel(o, machinev1alpha1.ManagerLabel, manager)
}

func SetClassLabel(o api.Object, class string) {
	metautils.SetLabel(o, machinev1alpha1.ClassLabel, class)
}

func SetMachineRefLabel(o api.Object, machineID string) {
	metautils.SetLabel(o, machinev1alpha1.MachineRefLabel, machineID)
}

func GetClassLabel(o api.Object) (string, bool) {
	class, found := o.GetLabels()[machinev1alpha1.ClassLabel]
	return class, found
}

func GetMachineRefLabel(o api.Object) (string, bool) {
	machineRef, found := o.GetLabels()[machinev1alpha1.MachineRefLabel]
	return machineRef, found
}

func IsManagedBy(o api.Object, manager string) bool {
	actual, ok := o.GetLabels()[machinev1alpha1.ManagerLabel]
	return ok && actual == manager
}
