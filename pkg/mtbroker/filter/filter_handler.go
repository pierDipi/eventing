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

package filter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	cloudeventsv2 "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"go.uber.org/zap"
	eventingv1alpha1 "knative.dev/eventing/pkg/apis/eventing/v1alpha1"
	eventinglisters "knative.dev/eventing/pkg/client/listers/eventing/v1alpha1"
	"knative.dev/eventing/pkg/kncloudevents"
	"knative.dev/eventing/pkg/logging"
	broker "knative.dev/eventing/pkg/mtbroker"
	"knative.dev/eventing/pkg/reconciler/trigger/path"
	"knative.dev/eventing/pkg/utils"
)

const (
	passFilter FilterResult = "pass"
	failFilter FilterResult = "fail"
	noFilter   FilterResult = "no_filter"

	// TODO make these constants configurable (either as env variables, config map, or part of broker spec).
	//  Issue: https://github.com/knative/eventing/issues/1777
	// Constants for the underlying HTTP Client transport. These would enable better connection reuse.
	// Set them on a 10:1 ratio, but this would actually depend on the Triggers' subscribers and the workload itself.
	// These are magic numbers, partly set based on empirical evidence running performance workloads, and partly
	// based on what serving is doing. See https://github.com/knative/serving/blob/master/pkg/network/transports.go.
	defaultMaxIdleConnections        = 1000
	defaultMaxIdleConnectionsPerHost = 100
)

// Handler parses Cloud Events, determines if they pass a filter, and sends them to a subscriber.
type Handler struct {
	// Receiver receives incoming HTTP requests
	Receiver *kncloudevents.HttpMessageReceiver
	// Sender sends requests to downstream services
	Sender broker.Sender
	// Defaults sets default values to incoming events
	Defaulter client.EventDefaulter
	// Reporter reports stats of status code and dispatch time
	Reporter StatsReporter

	TriggerLister eventinglisters.TriggerLister
	Logger        *zap.Logger
}

type sendError struct {
	Err    error
	Status int
}

func (e sendError) Error() string {
	return e.Err.Error()
}

func (e sendError) Unwrap() error {
	return e.Err
}

// FilterResult has the result of the filtering operation.
type FilterResult string

// Start begins to receive messages for the handler.
//
// Only HTTP POST requests to the root path (/) are accepted. If other paths or
// methods are needed, use the HandleRequest method directly with another HTTP
// server.
//
// This method will block until ctx is done.
func (h *Handler) Start(ctx context.Context) error {
	return h.Receiver.StartListen(ctx, broker.WithLivenessCheck(broker.WithReadinessCheck(h)))
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	triggerRef, err := path.Parse(request.RequestURI)
	if err != nil {
		h.Logger.Info("Unable to parse path as trigger", zap.Error(err), zap.String("path", request.RequestURI))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := binding.ToEvent(request.Context(), cehttp.NewMessageFromHttpRequest(request))
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Remove the TTL attribute that is used by the Broker.
	ttl, err := broker.GetTTLv2(event.Context)
	if err != nil {
		// Only messages sent by the Broker should be here. If the attribute isn't here, then the
		// event wasn't sent by the Broker, so we can drop it.
		h.Logger.Warn("No TTL seen, dropping", zap.Any("triggerRef", triggerRef), zap.Any("event", event))
		// Return a BadRequest error, so the upstream can decide how to handle it, e.g. sending
		// the message to a DLQ.
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := broker.DeleteTTLv2(event.Context); err != nil {
		h.Logger.Warn("Failed to delete TTL.", zap.Error(err))
	}

	h.Logger.Debug("Received message", zap.Any("triggerRef", triggerRef))

	ctx := request.Context()

	t, err := h.getTrigger(ctx, triggerRef)
	if err != nil {
		h.Logger.Info("Unable to get the Trigger", zap.Error(err), zap.Any("triggerRef", triggerRef))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	reportArgs := &ReportArgs{
		ns:         t.Namespace,
		trigger:    t.Name,
		broker:     t.Spec.Broker,
		filterType: triggerFilterAttribute(t.Spec.Filter, "type"),
	}

	subscriberURI := t.Status.SubscriberURI
	if subscriberURI == nil {
		// Record the event count.
		_ = h.Reporter.ReportEventCount(reportArgs, http.StatusNotFound)
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte("unable to read subscriberURI"))
		return
	}

	// Check if the event should be sent.
	filterResult := h.shouldSendEvent(ctx, &t.Spec, event)

	if filterResult == failFilter {
		h.Logger.Debug("Event did not pass filter", zap.Any("triggerRef", triggerRef))
		// We do not count the event. The event will be counted in the broker ingress.
		// If the filter didn't pass, it means that the event wasn't meant for this Trigger.
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Record the event processing time. This might be off if the receiver and the filter pods are running in
	// different nodes with different clocks.
	var arrivalTimeStr string
	if extErr := event.ExtensionAs(broker.EventArrivalTime, &arrivalTimeStr); extErr == nil {
		arrivalTime, err := time.Parse(time.RFC3339, arrivalTimeStr)
		if err == nil {
			_ = h.Reporter.ReportEventProcessingTime(reportArgs, time.Since(arrivalTime))
		}
	}

	target := subscriberURI.String()
	req, err := h.Sender.NewCloudEventRequestWithTarget(ctx, target)
	if err != nil {
		h.Logger.Info("failed to create the request", zap.String("target", target))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	req.Header = utils.PassThroughHeaders(request.Header)
	err = cehttp.WriteRequest(ctx, binding.ToMessage(event), req)
	if err != nil {
		h.Logger.Info("failed to write request")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	start := time.Now()
	resp, err := h.Sender.Send(req)
	dispatchTime := time.Since(start)
	if err != nil {
		h.Logger.Error("failed to dispatch message", zap.Error(err))
		_ = h.Reporter.ReportEventDispatchTime(reportArgs, http.StatusInternalServerError, dispatchTime)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	// TODO response might contain another event
	// 		if it doesn't just return otherwise
	// 		it should be send back (btw, I dunno where)

	// Reattach the TTL (with the same value) to the response event before sending it to the Broker.
	if err := broker.SetTTLv2(nil, ttl); err != nil {
		h.Logger.Error("failed to reset TTL", zap.Error(err))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (r *Handler) serveHTTP(ctx context.Context, event cloudevents.Event, resp *cloudevents.EventResponse) error {
	tctx := cloudevents.HTTPTransportContextFrom(ctx)
	if tctx.Method != http.MethodPost {
		resp.Status = http.StatusMethodNotAllowed
		return nil
	}

	// tctx.URI is actually the path...
	triggerRef, err := path.Parse(tctx.URI)
	if err != nil {
		r.logger.Info("Unable to parse path as a trigger", zap.Error(err), zap.String("path", tctx.URI))
		return errors.New("unable to parse path as a Trigger")
	}

	// Remove the TTL attribute that is used by the Broker.
	ttl, err := broker.GetTTL(event.Context)
	if err != nil {
		// Only messages sent by the Broker should be here. If the attribute isn't here, then the
		// event wasn't sent by the Broker, so we can drop it.
		r.logger.Warn("No TTL seen, dropping", zap.Any("triggerRef", triggerRef), zap.Any("event", event))
		// Return a BadRequest error, so the upstream can decide how to handle it, e.g. sending
		// the message to a DLQ.
		resp.Status = http.StatusBadRequest
		return nil
	}
	if err := broker.DeleteTTL(event.Context); err != nil {
		r.logger.Warn("Failed to delete TTL.", zap.Error(err))
	}

	r.logger.Debug("Received message", zap.Any("triggerRef", triggerRef))

	responseEvent, err := r.sendEvent(ctx, tctx, triggerRef, &event)
	if err != nil {
		// Propagate any error codes from the invoke back upstram.
		var httpError sendError
		if errors.As(err, &httpError) {
			resp.Status = httpError.Status
		}
		r.logger.Error("Error sending the event", zap.Error(err))
		return err
	}

	resp.Status = http.StatusAccepted
	if responseEvent == nil {
		return nil
	}

	// Reattach the TTL (with the same value) to the response event before sending it to the Broker.

	if err := broker.SetTTL(responseEvent.Context, ttl); err != nil {
		return err
	}
	resp.Event = responseEvent
	resp.Context = &cloudevents.HTTPTransportResponseContext{
		Header: utils.PassThroughHeaders(tctx.Header),
	}

	return nil
}

// sendEvent sends an event to a subscriber if the trigger filter passes.
func (r *Handler) sendEvent(ctx context.Context, tctx cloudevents.HTTPTransportContext, trigger path.NamespacedNameUID, event *cloudevents.Event) (*cloudevents.Event, error) {
	t, err := r.getTrigger(ctx, trigger)
	if err != nil {
		r.logger.Info("Unable to get the Trigger", zap.Error(err), zap.Any("triggerRef", trigger))
		return nil, err
	}

	reportArgs := &ReportArgs{
		ns:         t.Namespace,
		trigger:    t.Name,
		broker:     t.Spec.Broker,
		filterType: triggerFilterAttribute(t.Spec.Filter, "type"),
	}

	subscriberURI := t.Status.SubscriberURI
	if subscriberURI == nil {
		err = errors.New("unable to read subscriberURI")
		// Record the event count.
		r.reporter.ReportEventCount(reportArgs, http.StatusNotFound)
		return nil, err
	}

	// Check if the event should be sent.
	filterResult := r.shouldSendEvent(ctx, &t.Spec, event)

	if filterResult == failFilter {
		r.logger.Debug("Event did not pass filter", zap.Any("triggerRef", trigger))
		// We do not count the event. The event will be counted in the broker ingress.
		// If the filter didn't pass, it means that the event wasn't meant for this Trigger.
		return nil, nil
	}

	// Record the event processing time. This might be off if the receiver and the filter pods are running in
	// different nodes with different clocks.
	var arrivalTimeStr string
	if extErr := event.ExtensionAs(broker.EventArrivalTime, &arrivalTimeStr); extErr == nil {
		arrivalTime, err := time.Parse(time.RFC3339, arrivalTimeStr)
		if err == nil {
			r.reporter.ReportEventProcessingTime(reportArgs, time.Since(arrivalTime))
		}
	}

	sendingCTX := utils.SendingContextFrom(ctx, tctx, subscriberURI.URL())

	start := time.Now()
	rctx, replyEvent, err := r.ceClient.Send(sendingCTX, *event)
	rtctx := cloudevents.HTTPTransportContextFrom(rctx)
	// Record the dispatch time.
	r.reporter.ReportEventDispatchTime(reportArgs, rtctx.StatusCode, time.Since(start))
	// Record the event count.
	r.reporter.ReportEventCount(reportArgs, rtctx.StatusCode)
	// Wrap any errors along with the response status code so that can be propagated upstream.
	if err != nil {
		err = sendError{err, rtctx.StatusCode}
	}
	return replyEvent, err
}

func (h *Handler) getTrigger(ctx context.Context, ref path.NamespacedNameUID) (*eventingv1alpha1.Trigger, error) {
	t, err := h.TriggerLister.Triggers(ref.Namespace).Get(ref.Name)
	if err != nil {
		return nil, err
	}
	if t.UID != ref.UID {
		return nil, fmt.Errorf("trigger had a different UID. From ref '%s'. From Kubernetes '%s'", ref.UID, t.UID)
	}
	return t, nil
}

// shouldSendEvent determines whether event 'event' should be sent based on the triggerSpec 'ts'.
// Currently it supports exact matching on event context attributes and extension attributes.
// If no filter is present, shouldSendEvent returns passFilter.
func (r *Handler) shouldSendEvent(ctx context.Context, ts *eventingv1alpha1.TriggerSpec, event *cloudeventsv2.Event) FilterResult {
	// No filter specified, default to passing everything.
	if ts.Filter == nil || (ts.Filter.DeprecatedSourceAndType == nil && ts.Filter.Attributes == nil) {
		return noFilter
	}

	attrs := map[string]string{}
	// Since the filters cannot distinguish presence, filtering for an empty
	// string is impossible.
	if ts.Filter.DeprecatedSourceAndType != nil {
		attrs["type"] = ts.Filter.DeprecatedSourceAndType.Type
		attrs["source"] = ts.Filter.DeprecatedSourceAndType.Source
	} else if ts.Filter.Attributes != nil {
		attrs = *ts.Filter.Attributes
	}

	return filterEventByAttributes(ctx, attrs, event)
}

func filterEventByAttributes(ctx context.Context, attrs map[string]string, event *cloudeventsv2.Event) FilterResult {
	// Set standard context attributes. The attributes available may not be
	// exactly the same as the attributes defined in the current version of the
	// CloudEvents spec.
	ce := map[string]interface{}{
		"specversion":     event.SpecVersion(),
		"type":            event.Type(),
		"source":          event.Source(),
		"subject":         event.Subject(),
		"id":              event.ID(),
		"time":            event.Time().String(),
		"schemaurl":       event.DataSchema(),
		"datacontenttype": event.DataContentType(),
		"datamediatype":   event.DataMediaType(),
		// TODO: use data_base64 when SDK supports it.
		"datacontentencoding": event.DeprecatedDataContentEncoding(),
	}
	ext := event.Extensions()
	for k, v := range ext {
		ce[k] = v
	}

	for k, v := range attrs {
		var value interface{}
		value, ok := ce[k]
		// If the attribute does not exist in the event, return false.
		if !ok {
			logging.FromContext(ctx).Debug("Attribute not found", zap.String("attribute", k))
			return failFilter
		}
		// If the attribute is not set to any and is different than the one from the event, return false.
		if v != eventingv1alpha1.TriggerAnyFilter && v != value {
			logging.FromContext(ctx).Debug("Attribute had non-matching value", zap.String("attribute", k), zap.String("filter", v), zap.Any("received", value))
			return failFilter
		}
	}
	return passFilter
}

// triggerFilterAttribute returns the filter attribute value for a given `attributeName`. If it doesn't not exist,
// returns the any value filter.
func triggerFilterAttribute(filter *eventingv1alpha1.TriggerFilter, attributeName string) string {
	attributeValue := eventingv1alpha1.TriggerAnyFilter
	if filter != nil {
		if filter.DeprecatedSourceAndType != nil {
			if attributeName == "type" {
				attributeValue = filter.DeprecatedSourceAndType.Type
			} else if attributeName == "source" {
				attributeValue = filter.DeprecatedSourceAndType.Source
			}
		} else if filter.Attributes != nil {
			attrs := map[string]string(*filter.Attributes)
			if v, ok := attrs[attributeName]; ok {
				attributeValue = v
			}
		}
	}
	return attributeValue
}
