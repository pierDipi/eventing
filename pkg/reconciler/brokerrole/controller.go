package brokerrole

import (
	"context"

	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	pkgreconciler "knative.dev/pkg/reconciler"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	brokerinformer "knative.dev/eventing/pkg/client/injection/informers/eventing/v1/broker"
	brokerreconciler "knative.dev/eventing/pkg/client/injection/reconciler/eventing/v1/broker"
)

// NewController initializes the controller and is called by the generated code
// Registers event handlers to enqueue events
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
	class string,
) *controller.Impl {
	brokerInformer := brokerinformer.Get(ctx)

	r := &Reconciler{
		rbacClient: kubeclient.Get(ctx).RbacV1(),
	}
	impl := brokerreconciler.NewImpl(ctx, r, class)

	brokerFilter := pkgreconciler.AnnotationFilterFunc(brokerreconciler.ClassAnnotationKey, class, false /*allowUnset*/)
	brokerInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: brokerFilter,
		Handler: controller.HandleAll(controller.EnsureTypeMeta(
			impl.Enqueue,
			eventingv1.SchemeGroupVersion.WithKind("Broker"),
		)),
	})

	return impl
}
