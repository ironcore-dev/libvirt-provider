// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/ironcore-dev/controller-utils/metautils"
	irimeta "github.com/ironcore-dev/ironcore/iri/apis/meta/v1alpha1"
	apiutils "github.com/ironcore-dev/provider-utils/apiutils/api"
)

func GetObjectMetadata(o apiutils.Metadata) (*irimeta.ObjectMetadata, error) {
	annotations, err := apiutils.GetAnnotationsAnnotation(o, AnnotationsAnnotation)
	if err != nil {
		return nil, err
	}

	labels, err := apiutils.GetLabelsAnnotation(o, LabelsAnnotation)
	if err != nil {
		return nil, err
	}

	var deletedAt int64
	if o.DeletedAt != nil && !o.DeletedAt.IsZero() {
		deletedAt = o.DeletedAt.UnixNano()
	}

	return &irimeta.ObjectMetadata{
		Id:          o.ID,
		Annotations: annotations,
		Labels:      labels,
		Generation:  o.GetGeneration(),
		CreatedAt:   o.CreatedAt.UnixNano(),
		DeletedAt:   deletedAt,
	}, nil
}

func SetObjectMetadata(o apiutils.Object, metadata *irimeta.ObjectMetadata) error {
	if err := apiutils.SetAnnotationsAnnotation(o, AnnotationsAnnotation, metadata.Annotations); err != nil {
		return err
	}
	if err := apiutils.SetLabelsAnnotation(o, LabelsAnnotation, metadata.Labels); err != nil {
		return err
	}
	return nil
}

func SetManagerLabel(o apiutils.Object, manager string) {
	metautils.SetLabel(o, ManagerLabel, manager)
}

func SetClassLabel(o apiutils.Object, class string) {
	metautils.SetLabel(o, ClassLabel, class)
}

func GetClassLabel(o apiutils.Object) (string, bool) {
	class, found := o.GetLabels()[ClassLabel]
	return class, found
}

func IsManagedBy(o apiutils.Object, manager string) bool {
	actual, ok := o.GetLabels()[ManagerLabel]
	return ok && actual == manager
}
