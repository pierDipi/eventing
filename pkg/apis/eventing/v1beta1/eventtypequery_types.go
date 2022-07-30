/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EventTypeQuery represents a query for event types.
type EventTypeQuery struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the EventTypeQuery.
	Spec EventTypeQuerySpec `json:"spec,omitempty"`

	// Status represents the current state of the EventTypeQuery.
	// This data may be out of date.
	// +optional
	Status EventTypeQueryStatus `json:"status,omitempty"`
}

type EventTypeQuerySpec struct {
	CESQL *string `json:"CESQL,omitempty"`
}

type EventTypeQueryStatus struct {
	EventTypes []duckv1.KReference
}
