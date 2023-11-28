// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package apiutils

import (
	"encoding/json"
	"fmt"

	"github.com/ironcore-dev/controller-utils/metautils"
	orimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	"github.com/ironcore-dev/libvirt-provider/pkg/api"
	machinev1alpha1 "github.com/ironcore-dev/libvirt-provider/provider/api/v1alpha1"
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

func GetClassLabel(o api.Object) (string, bool) {
	class, found := o.GetLabels()[machinev1alpha1.ClassLabel]
	return class, found
}

func IsManagedBy(o api.Object, manager string) bool {
	actual, ok := o.GetLabels()[machinev1alpha1.ManagerLabel]
	return ok && actual == manager
}
