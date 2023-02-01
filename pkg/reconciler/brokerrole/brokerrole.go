package brokerrole

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/utils/pointer"
	"knative.dev/pkg/reconciler"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	"knative.dev/eventing/pkg/authorizer"
)

type Reconciler struct {
	rbacClient rbacv1client.RbacV1Interface
}

func (r *Reconciler) ReconcileKind(ctx context.Context, br *eventingv1.Broker) reconciler.Event {
	role := roleFromBroker(br)

	if _, err := r.rbacClient.Roles(br.GetNamespace()).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create broker RBAC role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func roleFromBroker(br *eventingv1.Broker) *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: br.GetNamespace(),
			Name:      br.GetName(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         br.APIVersion,
					Kind:               br.Kind,
					Name:               br.Name,
					UID:                br.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(false),
				},
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					string(authorizer.VerbPostEvent),
					"get",
				},
				APIGroups: []string{
					eventingv1.SchemeGroupVersion.Group,
				},
				Resources: []string{
					"brokers",
				},
				ResourceNames: []string{
					br.GetName(),
				},
			},
		},
	}
}
