package authorizer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	bearerTokenNotPresent = errors.New("no Bearer token provided")
)

type BearerTokenAuthorizer struct {
	ResourceAttributesProvider ResourceAttributesProvider
	K8s                        kubernetes.Interface
}

func (a *BearerTokenAuthorizer) Authorize(ctx context.Context, req *AuthorizationRequest) (*AuthorizationResponse, error) {
	token := extractBearerToken(req.Req)
	if token == "" {
		return unauthenticated(nil, bearerTokenNotPresent), nil
	}

	tokenReview, err := a.K8s.AuthenticationV1().
		TokenReviews().
		Create(ctx, createTokenReview(token), metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to creat token review: %w", err)
	}
	if !tokenReview.Status.Authenticated {
		return unauthenticated(tokenReview, errors.New("provided token is not valid or not associated with a known user")), nil
	}

	sar, err := a.createLocalSubjectAccessReview(req, tokenReview)
	if err != nil {
		return nil, err
	}
	sar, err = a.K8s.
		AuthorizationV1().
		LocalSubjectAccessReviews(req.Object.GetNamespace()).
		Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to verify permissions for provided token: %w", err)
	}
	if !sar.Status.Allowed {
		return unauthorized(
			tokenReview,
			sar,
			fmt.Errorf("user %s is not allowed to %s to %s/%s: %w",
				sar.Spec.User,
				req.Action.Description,
				req.Object.GetNamespace(),
				req.Object.GetName(),
				err,
			),
		), nil
	}

	return authorized(tokenReview, sar), nil
}

func (a *BearerTokenAuthorizer) createLocalSubjectAccessReview(req *AuthorizationRequest, tokenReview *authenticationv1.TokenReview) (*authorizationv1.LocalSubjectAccessReview, error) {
	ra, err := a.ResourceAttributesProvider.Get(req)
	if err != nil {
		return nil, err
	}
	return &authorizationv1.LocalSubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   req.Object.GetNamespace(),
				Verb:        ra.Verb,
				Group:       ra.Group,
				Version:     ra.Version,
				Resource:    ra.Resource,
				Subresource: ra.Subresource,
				Name:        req.Object.GetName(),
			},
			User:   tokenReview.Status.User.Username,
			Groups: tokenReview.Status.User.Groups,
			Extra:  transformExtra(tokenReview),
			UID:    tokenReview.Status.User.UID,
		},
	}, nil
}

func createTokenReview(token string) *authenticationv1.TokenReview {
	return &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     token,
			Audiences: nil,
		},
	}
}

func unauthorized(tokenReview *authenticationv1.TokenReview, sar *authorizationv1.LocalSubjectAccessReview, err error) *AuthorizationResponse {
	return &AuthorizationResponse{
		Authorized:               false,
		Authenticated:            true,
		Error:                    err,
		TokenReview:              tokenReview,
		LocalSubjectAccessReview: sar,
	}
}

func unauthenticated(tokenReview *authenticationv1.TokenReview, err error) *AuthorizationResponse {
	return &AuthorizationResponse{
		Authorized:    false,
		Authenticated: false,
		Error:         err,
		TokenReview:   tokenReview,
	}
}

func authorized(tokenReview *authenticationv1.TokenReview, sar *authorizationv1.LocalSubjectAccessReview) *AuthorizationResponse {
	return &AuthorizationResponse{
		Authorized:               true,
		Authenticated:            true,
		Error:                    nil,
		TokenReview:              tokenReview,
		LocalSubjectAccessReview: sar,
	}
}

func extractBearerToken(req *http.Request) string {
	authorizationHeader := req.Header.Values("Authorization")
	if len(authorizationHeader) == 0 {
		return ""
	}
	for _, s := range authorizationHeader {
		if strings.HasPrefix(s, "Bearer") {
			return strings.TrimSpace(strings.TrimPrefix(s, "Bearer"))
		}
	}
	return ""
}

func transformExtra(tokenReview *authenticationv1.TokenReview) map[string]authorizationv1.ExtraValue {
	r := make(map[string]authorizationv1.ExtraValue, len(tokenReview.Status.User.Extra))
	for k, v := range tokenReview.Status.User.Extra {
		r[k] = authorizationv1.ExtraValue(v)
	}
	return r
}
