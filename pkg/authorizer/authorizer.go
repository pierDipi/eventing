package authorizer

import (
	"context"
	"net/http"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AuthorizationRequest struct {
	Action Action
	Req    *http.Request
	Object metav1.Object
}

type AuthorizationResponse struct {
	Authorized    bool
	Authenticated bool
	Error         error

	TokenReview              *authenticationv1.TokenReview
	LocalSubjectAccessReview *authorizationv1.LocalSubjectAccessReview
}

type verb string

var (
	VerbPostEvent verb = "post"
)

type Action struct {
	Verb        verb
	Description string
}

type ResourceAttributesProvider interface {
	Get(req *AuthorizationRequest) (ResourceAttributes, error)
}

type ResourceAttributesProviderFunc func(req *AuthorizationRequest) (ResourceAttributes, error)

func (r ResourceAttributesProviderFunc) Get(req *AuthorizationRequest) (ResourceAttributes, error) {
	return r(req)
}

type Authorizer interface {
	Authorize(ctx context.Context, req *AuthorizationRequest) (*AuthorizationResponse, error)
}

// ResourceAttributes includes the authorization attributes available for resource requests to the Authorizer interface
type ResourceAttributes struct {
	// Verb is a kubernetes resource API verb, like: get, list, watch, create, update, delete, proxy.  "*" means all.
	// +optional
	Verb string `json:"verb,omitempty" protobuf:"bytes,2,opt,name=verb"`
	// Group is the API Group of the Resource.  "*" means all.
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,3,opt,name=group"`
	// Version is the API Version of the Resource.  "*" means all.
	// +optional
	Version string `json:"version,omitempty" protobuf:"bytes,4,opt,name=version"`
	// Resource is one of the existing resource types.  "*" means all.
	// +optional
	Resource string `json:"resource,omitempty" protobuf:"bytes,5,opt,name=resource"`
	// Subresource is one of the existing resource types.  "" means none.
	// +optional
	Subresource string `json:"subresource,omitempty" protobuf:"bytes,6,opt,name=subresource"`
}
