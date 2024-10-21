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

package statefulset

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/integer"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/controller"

	duckv1alpha1 "knative.dev/eventing/pkg/apis/duck/v1alpha1"
	"knative.dev/eventing/pkg/scheduler"
	st "knative.dev/eventing/pkg/scheduler/state"
)

type GetReserved func() map[types.NamespacedName]map[string]int32

type Config struct {
	StatefulSetNamespace string `json:"statefulSetNamespace"`
	StatefulSetName      string `json:"statefulSetName"`

	ScaleCacheConfig scheduler.ScaleCacheConfig `json:"scaleCacheConfig"`
	// PodCapacity max capacity for each StatefulSet's pod.
	PodCapacity int32 `json:"podCapacity"`
	// Autoscaler refresh period
	RefreshPeriod time.Duration `json:"refreshPeriod"`
	// Autoscaler retry period
	RetryPeriod time.Duration `json:"retryPeriod"`

	Evictor scheduler.Evictor `json:"-"`

	VPodLister scheduler.VPodLister     `json:"-"`
	NodeLister corev1listers.NodeLister `json:"-"`
	// Pod lister for statefulset: StatefulSetNamespace / StatefulSetName
	PodLister corev1listers.PodNamespaceLister `json:"-"`
}

func New(ctx context.Context, cfg *Config) (scheduler.Scheduler, error) {

	if cfg.PodLister == nil {
		return nil, fmt.Errorf("Config.PodLister is required")
	}

	scaleCache := scheduler.NewScaleCache(ctx, cfg.StatefulSetNamespace, kubeclient.Get(ctx).AppsV1().StatefulSets(cfg.StatefulSetNamespace), cfg.ScaleCacheConfig)

	stateAccessor := st.NewStateBuilder(cfg.StatefulSetName, cfg.VPodLister, cfg.PodCapacity, cfg.PodLister, scaleCache)

	autoscaler := newAutoscaler(cfg, stateAccessor, scaleCache)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Wait()
		autoscaler.Start(ctx)
	}()

	s := newStatefulSetScheduler(ctx, cfg, stateAccessor, autoscaler)
	wg.Done()

	return s, nil
}

// StatefulSetScheduler is a scheduler placing VPod into statefulset-managed set of pods
type StatefulSetScheduler struct {
	statefulSetName      string
	statefulSetNamespace string
	statefulSetClient    clientappsv1.StatefulSetInterface
	vpodLister           scheduler.VPodLister
	lock                 sync.Locker
	stateAccessor        st.StateAccessor
	autoscaler           Autoscaler

	// replicas is the (cached) number of statefulset replicas.
	replicas int32

	// reserved tracks vreplicas that have been placed (ie. scheduled) but haven't been
	// committed yet (ie. not appearing in vpodLister)
	reserved   map[types.NamespacedName]map[string]int32
	reservedMu sync.Mutex
}

var (
	_ reconciler.LeaderAware = &StatefulSetScheduler{}
	_ scheduler.Scheduler    = &StatefulSetScheduler{}
)

// Promote implements reconciler.LeaderAware.
func (s *StatefulSetScheduler) Promote(b reconciler.Bucket, enq func(reconciler.Bucket, types.NamespacedName)) error {
	if v, ok := s.autoscaler.(reconciler.LeaderAware); ok {
		return v.Promote(b, enq)
	}
	return nil
}

// Demote implements reconciler.LeaderAware.
func (s *StatefulSetScheduler) Demote(b reconciler.Bucket) {
	if v, ok := s.autoscaler.(reconciler.LeaderAware); ok {
		v.Demote(b)
	}
}

func newStatefulSetScheduler(ctx context.Context,
	cfg *Config,
	stateAccessor st.StateAccessor,
	autoscaler Autoscaler) *StatefulSetScheduler {

	s := &StatefulSetScheduler{
		statefulSetNamespace: cfg.StatefulSetNamespace,
		statefulSetName:      cfg.StatefulSetName,
		statefulSetClient:    kubeclient.Get(ctx).AppsV1().StatefulSets(cfg.StatefulSetNamespace),
		vpodLister:           cfg.VPodLister,
		lock:                 new(sync.Mutex),
		stateAccessor:        stateAccessor,
		reserved:             make(map[types.NamespacedName]map[string]int32, 8),
		autoscaler:           autoscaler,
	}

	// Monitor our statefulset
	c := kubeclient.Get(ctx)
	sif := informers.NewSharedInformerFactoryWithOptions(c,
		controller.GetResyncPeriod(ctx),
		informers.WithNamespace(cfg.StatefulSetNamespace),
	)

	sif.Apps().V1().StatefulSets().Informer().
		AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: controller.FilterWithNameAndNamespace(cfg.StatefulSetNamespace, cfg.StatefulSetName),
			Handler: controller.HandleAll(func(i interface{}) {
				s.updateStatefulset(ctx, i)
			}),
		})

	sif.Start(ctx.Done())
	_ = sif.WaitForCacheSync(ctx.Done())

	go func() {
		<-ctx.Done()
		sif.Shutdown()
	}()

	return s
}

func (s *StatefulSetScheduler) Schedule(ctx context.Context, vpod scheduler.VPod) ([]duckv1alpha1.Placement, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.reservedMu.Lock()
	defer s.reservedMu.Unlock()

	placements, err := s.scheduleVPod(ctx, vpod)
	if placements == nil {
		return placements, err
	}

	sort.SliceStable(placements, func(i int, j int) bool {
		return st.OrdinalFromPodName(placements[i].PodName) < st.OrdinalFromPodName(placements[j].PodName)
	})

	return placements, err
}

func (s *StatefulSetScheduler) scheduleVPod(ctx context.Context, vpod scheduler.VPod) ([]duckv1alpha1.Placement, error) {
	logger := logging.FromContext(ctx).With("key", vpod.GetKey(), zap.String("component", "scheduler"))
	ctx = logging.WithLogger(ctx, logger)

	// Get the current placements state
	// Quite an expensive operation but safe and simple.
	state, err := s.stateAccessor.State(ctx)
	if err != nil {
		logger.Debug("error while refreshing scheduler state (will retry)", zap.Error(err))
		return nil, err
	}

	logger.Debugw("scheduling", zap.Any("state", state))

	existingPlacements := vpod.GetPlacements()

	// Remove unschedulable or adjust overcommitted pods from placements
	var placements []duckv1alpha1.Placement
	if len(existingPlacements) > 0 {
		placements = make([]duckv1alpha1.Placement, 0, len(existingPlacements))
		for _, p := range existingPlacements {
			p := p.DeepCopy()
			ordinal := st.OrdinalFromPodName(p.PodName)

			if !state.IsSchedulablePod(ordinal) {
				continue
			}

			// Handle overcommitted pods.
			if state.Free(ordinal) < 0 {
				// vr > free => vr: 9, overcommit 4 -> free: 0, vr: 5
				// vr = free => vr: 4, overcommit 4 -> free: 0, vr: 0
				// vr < free => vr: 3, overcommit 4 -> free: -1, vr: 0

				overcommit := -state.Free(ordinal)

				logger.Debugw("overcommit", zap.Any("overcommit", overcommit), zap.Any("placement", p))

				if p.VReplicas >= overcommit {
					state.SetFree(ordinal, 0)
					p.VReplicas = p.VReplicas - overcommit
				} else {
					state.SetFree(ordinal, p.VReplicas-overcommit)
					p.VReplicas = 0
				}
			}

			if p.VReplicas > 0 {
				placements = append(placements, *p)
			}
		}
	}

	// Exact number of vreplicas => do nothing
	totalCommittedReplicas := scheduler.GetTotalVReplicas(placements)
	if totalCommittedReplicas == vpod.GetVReplicas() {
		logger.Debug("scheduling succeeded (already scheduled)")
		// Fully placed. Remove reserved.
		delete(s.reserved, vpod.GetKey())
		return placements, nil
	}

	// Need less => scale down
	if totalCommittedReplicas > vpod.GetVReplicas() {
		logger.Debugw("scaling down", zap.Int32("vreplicas", totalCommittedReplicas), zap.Int32("new vreplicas", vpod.GetVReplicas()),
			zap.Any("placements", placements),
			zap.Any("existingPlacements", existingPlacements))

		placements = s.removeReplicas(totalCommittedReplicas-vpod.GetVReplicas(), placements)

		// Remove reserved from this vpod key.
		delete(s.reserved, vpod.GetKey())
		// Do not trigger the autoscaler to avoid unnecessary churn
		return placements, nil
	}

	// Need more => scale up
	logger.Debugw("scaling up", zap.Int32("vreplicas", totalCommittedReplicas), zap.Int32("new vreplicas", vpod.GetVReplicas()),
		zap.Any("placements", placements),
		zap.Any("existingPlacements", existingPlacements))

	placements, left := s.addReplicas(state, vpod.GetVReplicas()-totalCommittedReplicas, placements)
	if left > 0 {
		// Give time for the autoscaler to do its job
		logger.Infow("not enough pod replicas to schedule")

		// Trigger the autoscaler
		if s.autoscaler != nil {
			logger.Infow("Awaiting autoscaler", zap.Any("placement", placements), zap.Int32("left", left))
			s.autoscaler.Autoscale(ctx)
		}
		return placements, s.notEnoughPodReplicas(left)
	}

	logger.Infow("scheduling successful", zap.Any("placement", placements))

	return placements, nil
}

func (s *StatefulSetScheduler) removeReplicas(diff int32, placements []duckv1alpha1.Placement) []duckv1alpha1.Placement {
	newPlacements := make([]duckv1alpha1.Placement, 0, len(placements))
	for i := len(placements) - 1; i > -1; i-- {
		if diff >= placements[i].VReplicas {
			// remove the entire placement
			diff -= placements[i].VReplicas
		} else {
			newPlacements = append(newPlacements, duckv1alpha1.Placement{
				PodName:   placements[i].PodName,
				VReplicas: placements[i].VReplicas - diff,
			})
			diff = 0
		}
	}
	return newPlacements
}

func (s *StatefulSetScheduler) addReplicas(states *st.State, diff int32, placements []duckv1alpha1.Placement) ([]duckv1alpha1.Placement, int32) {
	// Pod affinity algorithm: prefer adding replicas to existing pods before considering other replicas
	newPlacements := make([]duckv1alpha1.Placement, 0, len(placements))

	// Add to existing
	for i := 0; i < len(placements); i++ {
		podName := placements[i].PodName
		ordinal := st.OrdinalFromPodName(podName)

		// Is there space in PodName?
		f := states.Free(ordinal)
		if diff >= 0 && f > 0 {
			allocation := integer.Int32Min(f, diff)
			newPlacements = append(newPlacements, duckv1alpha1.Placement{
				PodName:   podName,
				VReplicas: placements[i].VReplicas + allocation,
			})

			diff -= allocation
			states.SetFree(ordinal, f-allocation)
		} else {
			newPlacements = append(newPlacements, placements[i])
		}
	}

	if diff > 0 {
		// Needs to allocate replicas to additional pods
		for ordinal := int32(0); ordinal < s.replicas; ordinal++ {
			f := states.Free(ordinal)
			if f > 0 {
				allocation := integer.Int32Min(f, diff)
				newPlacements = append(newPlacements, duckv1alpha1.Placement{
					PodName:   st.PodNameFromOrdinal(s.statefulSetName, ordinal),
					VReplicas: allocation,
				})

				diff -= allocation
				states.SetFree(ordinal, f-allocation)
			}

			if diff == 0 {
				break
			}
		}
	}

	return newPlacements, diff
}

func (s *StatefulSetScheduler) updateStatefulset(ctx context.Context, obj interface{}) {
	statefulset, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		logging.FromContext(ctx).Warnw("expected a Statefulset object", zap.Any("object", obj))
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if statefulset.Spec.Replicas == nil {
		s.replicas = 1
	} else if s.replicas != *statefulset.Spec.Replicas {
		s.replicas = *statefulset.Spec.Replicas
		logging.FromContext(ctx).Infow("statefulset replicas updated", zap.Int32("replicas", s.replicas))
	}
}

// newNotEnoughPodReplicas returns an error explaining what is the problem, what are the actions we're taking
// to try to fix it (retry), wrapping a controller.requeueKeyError which signals to ReconcileKind to requeue the
// object after a given delay.
func (s *StatefulSetScheduler) notEnoughPodReplicas(left int32) error {
	// Wrap controller.requeueKeyError error to wait for the autoscaler to do its job.
	return fmt.Errorf("insufficient running pods replicas for StatefulSet %s/%s to schedule resource replicas (left: %d): retry %w",
		s.statefulSetNamespace, s.statefulSetName,
		left,
		controller.NewRequeueAfter(5*time.Second),
	)
}
