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

package v1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// Agent is the type for the Agent API
//
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=agents
// +kubebuilder:storageversion
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

type AgentSpec struct {
	// SystemPrompt for the agent.
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// LLM configures which LLM service will be used to process incoming requests.
	// The used LLM must support function calling.
	// +optional
	LLM *LLMSpec `json:"llm,omitempty"`

	Tools Tools `json:",omitempty"`
}

type Tools struct {
	// AgentSelectors selects "sub-agents", each selector is evaluated individually (OR).
	AgentSelectors []AgentSelector `json:"agents,omitempty"`

	// Tools is the list of tools.
	Tools []ToolSpec `json:"tools,omitempty"`
}

type AgentSelector struct {
	metav1.LabelSelector `json:",inline"`
}

type LLMSpec struct {
	// Reference to an HTTP addressable service (including inference service or external service URL).
	// For example: "uri: https://api.openai.com/v1/chat/completions" or reference to inference service
	Service duckv1.Destination `json:"service,omitempty"`

	// Configuration for the LLM connection, it configures how to connect to the LLM Service.
	Config LLMConfig `json:"config,omitempty"`
}

type ToolSpec struct {
	// Name of the tool.
	Name string `json:"name"`

	// Description describes the tool.
	Description string `json:"description"`

	// Parameters are the tool parameters.
	// +optional
	Parameters *EmbeddedToolParameters `json:"parameters,omitempty"`

	// Service to call.
	Service duckv1.Destination `json:"service"`
}

type ToolParameters struct {
	Embedded *EmbeddedToolParameters `json:",inline"`
	Discover *DiscoverToolParameters `json:"discover,omitempty"`
}

type EmbeddedToolParameters struct {
	// Schema JSONSchema for the HTTP body.
	// +optional
	Schema map[string]interface{} `json:"schema,omitempty"`

	// Method is the HTTP method to use to call the tool.
	// If there is a Schema, defaults to POST, otherwise defaults to GET.
	// +optional
	Method Method `json:"method,omitempty"`
}

type DiscoverToolParameters struct {
	// Schema is the URI for discovering the parameters schema.
	Schema duckv1.Destination `json:"schema,omitempty"`
}

type LLMConfig struct {
	// Type configures the LLM API type
	Type LLMType `json:"type,omitempty"`

	// Model name.
	Model string `json:"model,omitempty"`

	// The secret's key that contains the bearer token to be used by the client
	// for authentication.
	// The secret needs to be in the same namespace as the Agent
	// object and accessible by the controller.
	// +optional
	BearerTokenSecret *corev1.SecretKeySelector `json:"bearerTokenSecret,omitempty"`
}

// LLMType is the enum of supported LLM API types.
type LLMType string

const (
	OpenAI LLMType = "openai"
)

// Method supported HTTP methods.
type Method string

const (
	GET  Method = "get"
	POST Method = "post"
)

type AgentStatus struct {
	// Conditions for the InferenceService <br/>
	// - Ready: aggregated condition; <br/>
	duckv1.Status `json:",inline"`
	// Addressable endpoint for the InferenceService
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *apis.URL `json:"url,omitempty"`
}

func (t LLMType) IsOpenAI() bool {
	return strings.EqualFold(string(t), string(OpenAI))
}
