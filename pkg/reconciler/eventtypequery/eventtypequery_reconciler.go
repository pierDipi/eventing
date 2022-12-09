package eventtypequery

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr"
	v2 "github.com/cloudevents/sdk-go/sql/v2"
	"github.com/cloudevents/sdk-go/sql/v2/gen"
	cesqlparser "github.com/cloudevents/sdk-go/sql/v2/parser"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	eventing "knative.dev/eventing/pkg/apis/eventing/v1beta1"
	"knative.dev/eventing/pkg/broker/filter"
	clientset "knative.dev/eventing/pkg/client/clientset/versioned"
	eventingv1listers "knative.dev/eventing/pkg/client/listers/eventing/v1"
	eventingv1beta1listers "knative.dev/eventing/pkg/client/listers/eventing/v1beta1"
	"knative.dev/eventing/pkg/eventfilter"
	"knative.dev/eventing/pkg/eventfilter/subscriptionsapi"
	"knative.dev/eventing/pkg/utils"
)

type Reconciler struct {
	EventTypeLister   eventingv1beta1listers.EventTypeLister
	TriggerLister     eventingv1listers.TriggerLister
	EventingClientSet clientset.Interface
}

func (r *Reconciler) ReconcileKind(ctx context.Context, query *eventing.EventTypeQuery) reconciler.Event {

	query.Status.EventTypes = nil

	var eventTypes []*eventing.EventType

	filters := subscriptionsapi.NewAllFilter(filter.MaterializeFiltersList(ctx, query.Spec.Filters)...)

	registeredEventTypes, err := r.EventTypeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list event types: %w", err)
	}

	for _, eventType := range registeredEventTypes {

		if len(eventType.Spec.Broker) == 0 {
			// Event type isn't bound to a broker, skip it.
			continue
		}

		event := cloudevents.NewEvent(cloudevents.VersionV1)
		event.SetID(uuid.New().String())
		event.SetType(eventType.Spec.Type)
		event.SetDataSchema(eventType.Spec.Schema.String())
		event.SetSource(eventType.Spec.Source.String())

		if err := event.Validate(); err != nil { // Safety check, it should be always valid.
			return fmt.Errorf("example event built from event type %s/%s: %w", eventType.GetNamespace(), eventType.GetName(), err)
		}

		match := filters.Filter(ctx, event)

		if match == eventfilter.PassFilter {
			query.Status.EventTypes = append(query.Status.EventTypes, duckv1.KReference{
				APIVersion: eventType.TypeMeta.APIVersion,
				Kind:       eventType.TypeMeta.Kind,
				Namespace:  eventType.Namespace,
				Name:       eventType.Name,
			})
			eventTypes = append(eventTypes, eventType)
		}

		logging.FromContext(ctx).Desugar().Debug("Event type query match result",
			zap.String("eventtype.name", eventType.GetName()),
			zap.String("eventtype.namespace", eventType.GetNamespace()),
			zap.String("match", string(match)),
		)
	}
	query.MarkEventTypesListed()

	if query.Spec.Continuous != nil && *query.Spec.Continuous {
		if err := r.reconcileTriggers(ctx, query, eventTypes); err != nil {
			query.MarkTriggersReconciledFailed(err)
			return err
		}
		query.MarkTriggersReconciled()
	} else {
		query.MarkTriggersReconciledNoContinuous()
	}

	return nil
}

func (r *Reconciler) reconcileTriggers(ctx context.Context, query *eventing.EventTypeQuery, eventTypes []*eventing.EventType) error {
	triggers, err := r.TriggerLister.List(query.Selector())
	if err != nil {
		return fmt.Errorf("failed to list existing triggers: %w", err)
	}

	brokers, err := collectBrokers(eventTypes)
	if err != nil {
		return fmt.Errorf("failed to collect brokers for query: %w", err)
	}

	for _, tr := range triggers {
		brokerNamespacedName := types.NamespacedName{Namespace: tr.GetNamespace(), Name: tr.Spec.Broker}
		key := brokerNamespacedName.String()
		if brokers.Has(key) {
			brokers.Delete(key)
		} else {
			if err := r.deleteTrigger(ctx, tr); err != nil {
				return err
			}
		}

		expected := r.expectedTrigger(query, brokerNamespacedName)

		if !equality.Semantic.DeepDerivative(expected, tr) {
			tr = tr.DeepCopy()                            // Do not modify informer copy!
			tr.OwnerReferences = expected.OwnerReferences // TODO append expected to existing
			tr.Labels = expected.Labels
			tr.Spec.Filters = expected.Spec.Filters
			tr.Spec.Subscriber = expected.Spec.Subscriber

			updated, err := r.EventingClientSet.EventingV1().
				Triggers(tr.GetNamespace()).
				Update(ctx, tr, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to update trigger %s/%s: %w", tr.GetNamespace(), tr.GetName(), err)
			}
			controller.GetEventRecorder(ctx).Event(updated, "TriggerUpdated", "", "")
		}
	}

	for _, br := range brokers.UnsortedList() {
		nsName := strings.SplitN(br, string(types.Separator), 2)
		brokerNamespacedName := types.NamespacedName{Namespace: nsName[0], Name: nsName[1]}

		tr := r.expectedTrigger(query, brokerNamespacedName)
		// TODO algorithm should be aligned with the CLI
		tr.Name = utils.ToDNS1123Subdomain(kmeta.ChildName(brokerNamespacedName.Name, subscriberString(tr.Spec.Subscriber)))

		created, err := r.EventingClientSet.EventingV1().
			Triggers(tr.GetNamespace()).
			Create(ctx, &tr, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create trigger %s/%s: %w", tr.GetNamespace(), tr.GetName(), err)
		}
		controller.GetEventRecorder(ctx).Event(created, "TriggerCreated", "", "")
	}

	return nil
}

func subscriberString(subscriber duckv1.Destination) string {
	str := ""
	if subscriber.GetRef() != nil {
		str += subscriber.Ref.Namespace + subscriber.Ref.Name
		str += fmt.Sprint(md5.Sum([]byte(subscriber.Ref.APIVersion + subscriber.Ref.Kind)))
	}
	if subscriber.URI != nil {
		str += subscriber.URI.String()
	}
	return str
}

func (r *Reconciler) expectedTrigger(query *eventing.EventTypeQuery, brokerNamespacedName types.NamespacedName) eventingv1.Trigger {
	return eventingv1.Trigger{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				eventing.QueryLabelKey: query.QueryLabelValue(),
			},
			Namespace: brokerNamespacedName.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         eventing.SchemeGroupVersion.String(),
				Kind:               reflect.TypeOf(eventing.EventTypeQuery{}).Name(),
				Name:               query.Name,
				UID:                query.UID,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}},
		},
		Spec: eventingv1.TriggerSpec{
			Broker:     brokerNamespacedName.Name,
			Filters:    query.Spec.Filters,
			Subscriber: *query.Spec.Subscriber,
		},
	}
}

func (r *Reconciler) deleteTrigger(ctx context.Context, tr *eventingv1.Trigger) error {
	err := r.EventingClientSet.EventingV1().
		Triggers(tr.GetNamespace()).
		Delete(ctx, tr.GetName(), metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete trigger %s/%s: %w", tr.GetNamespace(), tr.GetName(), err)
	}
	controller.GetEventRecorder(ctx).Event(tr, "TriggerDeleted", "", "")
	return nil
}

func Parse(ctx context.Context, input string) (v2.Expression, error) {
	var is antlr.CharStream = antlr.NewInputStream(input)
	is = cesqlparser.NewCaseChangingStream(is, true)

	// Create the CE SQL Lexer
	lexer := gen.NewCESQLParserLexer(is)
	var stream antlr.TokenStream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

	// Create the JSON Parser
	antlrParser := gen.NewCESQLParserParser(stream)
	antlrParser.RemoveErrorListeners()
	antlrParser.AddErrorListener(antlr.NewDiagnosticErrorListener(false))
	antlrParser.AddErrorListener(NewConsoleErrorListener(ctx))

	// Finally, walk the tree
	visitor := cesqlparser.NewExpressionVisitor()
	result := antlrParser.Cesql().Accept(visitor)

	return result.(v2.Expression), nil
}

type ConsoleErrorListener struct {
	logger *zap.SugaredLogger
	*antlr.DefaultErrorListener
}

func NewConsoleErrorListener(ctx context.Context) *ConsoleErrorListener {
	return &ConsoleErrorListener{
		logger:               logging.FromContext(ctx),
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

func (c *ConsoleErrorListener) SyntaxError(_ antlr.Recognizer, _ interface{}, line, column int, msg string, _ antlr.RecognitionException) {
	c.logger.Error("line " + strconv.Itoa(line) + ":" + strconv.Itoa(column) + " " + msg)
}

func collectBrokers(eventTypes []*eventing.EventType) (sets.String, error) {
	brokerSet := sets.NewString()
	for _, et := range eventTypes {
		if len(et.Spec.Broker) == 0 {
			continue
		}

		key := et.GetNamespace() + string(types.Separator) + et.Spec.Broker
		if brokerSet.Has(key) {
			continue
		}
		brokerSet.Insert(key)
	}
	return brokerSet, nil
}
