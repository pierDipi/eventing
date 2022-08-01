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
	"knative.dev/pkg/apis"
)

const (
	ConditionEventTypesListed   = "EventTypesListed"
	ConditionTriggersReconciled = "TriggersReconciled"
)

var (
	conditionSet = apis.NewLivingConditionSet(
		ConditionEventTypesListed,
		ConditionTriggersReconciled,
	)
)

func (p *EventTypeQuery) GetConditionSet() apis.ConditionSet {
	return conditionSet
}

func (p *EventTypeQuery) MarkEventTypesListed() {
	p.GetConditionSet().Manage(p.GetStatus()).MarkTrue(ConditionEventTypesListed)
	p.Status.NumEventTypes = len(p.Status.EventTypes)
}

func (p *EventTypeQuery) MarkTriggersReconciledNoContinuous() {
	p.GetConditionSet().Manage(p.GetStatus()).MarkTrueWithReason(ConditionTriggersReconciled, "NoContinuous", "")
}

func (p *EventTypeQuery) MarkTriggersReconciled() {
	p.GetConditionSet().Manage(p.GetStatus()).MarkTrue(ConditionTriggersReconciled)
}

func (p *EventTypeQuery) MarkTriggersReconciledFailed(err error) {
	p.GetConditionSet().Manage(p.GetStatus()).MarkFalse(ConditionTriggersReconciled, "TriggerReconcilerFailed", err.Error())
}
