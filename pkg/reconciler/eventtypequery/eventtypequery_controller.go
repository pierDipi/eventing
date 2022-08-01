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

package eventtypequery

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"

	eventing "knative.dev/eventing/pkg/apis/eventing/v1beta1"
	eventingclient "knative.dev/eventing/pkg/client/injection/client"
	triggerinformer "knative.dev/eventing/pkg/client/injection/informers/eventing/v1/trigger"
	"knative.dev/eventing/pkg/client/injection/informers/eventing/v1beta1/eventtype"
	"knative.dev/eventing/pkg/client/injection/informers/eventing/v1beta1/eventtypequery"
	eventtypequeryreconciler "knative.dev/eventing/pkg/client/injection/reconciler/eventing/v1beta1/eventtypequery"
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {

	r := &Reconciler{
		EventTypeLister:   eventtype.Get(ctx).Lister(),
		EventingClientSet: eventingclient.Get(ctx),
		TriggerLister:     triggerinformer.Get(ctx).Lister(),
	}

	impl := eventtypequeryreconciler.NewImpl(ctx, r)

	gvk := schema.GroupVersionKind{
		Group:   eventing.SchemeGroupVersion.Group,
		Version: eventing.SchemeGroupVersion.Version,
		Kind:    "EventTypeQuery",
	}

	eventTypeQueryInformer := eventtypequery.Get(ctx).Informer()
	eventTypeQueryInformer.AddEventHandler(controller.HandleAll(controller.EnsureTypeMeta(
		impl.Enqueue,
		gvk,
	)))

	globalResync := func(i interface{}) {
		impl.GlobalResync(eventTypeQueryInformer)
	}
	eventtype.Get(ctx).Informer().AddEventHandler(controller.HandleAll(globalResync))
	triggerinformer.Get(ctx).Informer().AddEventHandler(controller.HandleAll(globalResync))

	return impl
}
