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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
)

const (
	QueryLabelKey = "eventing.knative.dev/query"
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
	Filters []eventingv1.SubscriptionsAPIFilter `json:"filters,omitempty"`

	// TODO immutable
	Continuous *bool `json:"continuous,omitempty"`

	// TODO must be set when continuous=true
	// Sink is a reference to an object that will resolve to a uri to use as the sink.
	Subscriber *duckv1.Destination `json:"subscriber,omitempty"`
}

type EventTypeQueryStatus struct {
	EventTypes    []duckv1.KReference `json:"eventTypes,omitempty"`
	NumEventTypes *int                `json:"numEventTypes,omitempty"`

	// inherits duck/v1 Status, which currently provides:
	// * ObservedGeneration - the 'Generation' of the Service that was last processed by the controller.
	// * Conditions - the latest available observations of a resource's current state.
	duckv1.Status `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EventTypeQueryList is a collection of EventTypeQuery.
type EventTypeQueryList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventTypeQuery `json:"items"`
}

// GetGroupVersionKind returns GroupVersionKind for EventType
func (e *EventTypeQuery) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("EventTypeQuery")
}

// GetUntypedSpec returns the spec of the EventType.
func (e *EventTypeQuery) GetUntypedSpec() interface{} {
	return e.Spec
}

// GetStatus retrieves the status of the EventType. Implements the KRShaped interface.
func (e *EventTypeQuery) GetStatus() *duckv1.Status {
	return &e.Status.Status
}

func (e *EventTypeQuery) Selector() labels.Selector {
	return labels.SelectorFromSet(map[string]string{QueryLabelKey: e.QueryLabelValue()})
}

func (e *EventTypeQuery) QueryLabelValue() string {
	return string(e.UID)
}

var (
	//// Check that EventTypeQuery can be validated, can be defaulted, and has immutable fields.
	//_ apis.Validatable = (*EventTypeQuery)(nil)
	//_ apis.Defaultable = (*EventTypeQuery)(nil)

	// Check that EventTypeQuery can return its spec untyped.
	_ apis.HasSpec = (*EventTypeQuery)(nil)

	_ runtime.Object = (*EventTypeQuery)(nil)

	// Check that we can create OwnerReferences to an EventTypeQuery.
	_ kmeta.OwnerRefable = (*EventTypeQuery)(nil)

	// Check that the type conforms to the duck Knative Resource shape.
	_ duckv1.KRShaped = (*EventTypeQuery)(nil)
)
