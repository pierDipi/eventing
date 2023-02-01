package authorizer

import (
	"fmt"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
)

func BrokerResourceAttributesProvider() ResourceAttributesProviderFunc {
	return func(req *AuthorizationRequest) (ResourceAttributes, error) {
		if req.Action.Verb == VerbPostEvent {
			return ResourceAttributes{
				Verb:     string(VerbPostEvent),
				Group:    eventingv1.SchemeGroupVersion.Group,
				Resource: "brokers",
			}, nil
		}

		return ResourceAttributes{}, fmt.Errorf("unknown authorizer action verb %s", req.Action.Verb)
	}
}
