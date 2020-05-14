/*
 * Copyright 2019 The Knative Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"go.uber.org/zap"
	"knative.dev/eventing/pkg/kncloudevents"
	broker "knative.dev/eventing/pkg/mtbroker"
	"knative.dev/eventing/pkg/utils"
)

const (
	// noDuration signals that the dispatch step hasn't started
	noDuration = -1
)

type Handler struct {
	// Receiver receives incoming HTTP requests
	Receiver *kncloudevents.HttpMessageReceiver
	// Sender sends requests to the broker
	Sender broker.Sender
	// Defaults sets default values to incoming events
	Defaulter client.EventDefaulter
	// Reporter reports stats of status code and dispatch time
	Reporter StatsReporter

	Logger *zap.Logger
}

func (h *Handler) Start(ctx context.Context) error {
	return h.Receiver.StartListen(ctx, broker.WithLivenessCheck(h))
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// validate request method
	if request.Method != http.MethodPost {
		h.Logger.Warn("unexpected request method", zap.String("method", request.Method))
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// validate request URI
	if request.RequestURI == "/" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	nsBrokerName := strings.Split(request.RequestURI, "/")
	if len(nsBrokerName) != 3 {
		h.Logger.Info("Malformed uri", zap.String("URI", request.RequestURI))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	event, err := binding.ToEvent(request.Context(), cehttp.NewMessageFromHttpRequest(request))
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	brokerNamespace := nsBrokerName[1]
	brokerName := nsBrokerName[2]
	reporterArgs := &ReportArgs{
		ns:        brokerNamespace,
		broker:    brokerName,
		eventType: event.Type(),
	}

	statusCode, dispatchTime := h.receive(request.Context(), request.Header, event, brokerNamespace, brokerName)
	if dispatchTime > noDuration {
		_ = h.Reporter.ReportEventDispatchTime(reporterArgs, statusCode, dispatchTime)
	}
	_ = h.Reporter.ReportEventCount(reporterArgs, statusCode)

	writer.WriteHeader(statusCode)
}

func (h *Handler) receive(
	ctx context.Context,
	headers http.Header,
	event *cloudevents.Event,
	brokerNamespace,
	brokerName string,
) (int, time.Duration) {

	// Setting the extension as a string as the CloudEvents sdk does not support non-string extensions.
	event.SetExtension(broker.EventArrivalTime, cloudevents.Timestamp{Time: time.Now()})
	if h.Defaulter != nil {
		newEvent := h.Defaulter(ctx, *event)
		event = &newEvent
	}

	if ttl, err := broker.GetTTLv2(event.Context); err != nil || ttl <= 0 {
		h.Logger.Debug("dropping event based on TTL status.", zap.Int32("TTL", ttl), zap.String("event.id", event.ID()), zap.Error(err))
		return http.StatusBadRequest, noDuration
	}

	// TODO: Today these are pre-deterministic, change this watch for
	//  	 channels and look up from the channels Status
	channelURI := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s-kne-trigger-kn-channel.%s.svc.%s", brokerName, brokerNamespace, utils.GetClusterDomainName()),
		Path:   "/",
	}

	request, err := h.Sender.NewCloudEventRequestWithTarget(ctx, channelURI.String())
	if err != nil {
		return http.StatusInternalServerError, noDuration
	}
	request.Header = utils.PassThroughHeaders(headers) // TODO add unit test
	err = cehttp.WriteRequest(ctx, binding.ToMessage(event), request)
	if err != nil {
		return http.StatusInternalServerError, noDuration
	}

	start := time.Now()
	resp, err := h.Sender.Send(request)
	dispatchTime := time.Since(start)
	if err != nil {
		return http.StatusInternalServerError, dispatchTime
	}

	return resp.StatusCode, dispatchTime
}
