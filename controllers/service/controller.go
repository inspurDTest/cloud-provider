/*
Copyright 2015 The Kubernetes Authors.

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

package service

import (
	"context"
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strings"
	"sync"
	"time"

	cloudprovider "github.com/inspurDTest/cloud-provider"
	"github.com/inspurDTest/cloud-provider/api"
	endpointSliceHelper "github.com/inspurDTest/cloud-provider/endpointslice/helpers"
	servicehelper "github.com/inspurDTest/cloud-provider/service/helpers"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	coreinformers "k8s.io/client-go/informers/core/v1"
	discoveryinformers "k8s.io/client-go/informers/discovery/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	discoverylisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/featuregate"
	controllersmetrics "k8s.io/component-base/metrics/prometheus/controllers"
	"k8s.io/controller-manager/pkg/features"
	"k8s.io/klog/v2"
)

const (
	// Interval of synchronizing service status from apiserver
	serviceSyncPeriod = 30 * time.Second

	// Interval of synchronizing endpointSlice status from apiserver
	endpointSliceSyncPeriod = 30 * time.Second

	// Interval of synchronizing node status from apiserver
	nodeSyncPeriod = 100 * time.Second

	// How long to wait before retrying the processing of a service change.
	// If this changes, the sleep in hack/jenkins/e2e.sh before downing a cluster
	// should be changed appropriately.
	minRetryDelay = 5 * time.Second
	maxRetryDelay = 300 * time.Second
	// ToBeDeletedTaint is a taint used by the CLuster Autoscaler before marking a node for deletion. Defined in
	// https://github.com/kubernetes/autoscaler/blob/e80ab518340f88f364fe3ef063f8303755125971/cluster-autoscaler/utils/deletetaint/delete.go#L36
	ToBeDeletedTaint = "ToBeDeletedByClusterAutoscaler"

	KubernetesServiceName = "kubernetes.io/service-name"

	ServiceAnnotationLoadBalancerID    = "inspur.com/load-balancer-id"
	ServiceAnnotationLoadBalancerOldID = "inspur.com/load-balancer-old-id"
)

type cachedService struct {
	// The cached state of the service
	state *v1.Service
}

type serviceCache struct {
	mu         sync.RWMutex // protects serviceMap
	serviceMap map[string]*cachedService
}

// Controller keeps cloud provider service resources
// (like load balancers) in sync with the registry.
type Controller struct {
	cloud       cloudprovider.Interface
	kubeClient  clientset.Interface
	clusterName string
	balancer    cloudprovider.LoadBalancer
	// TODO(#85155): Stop relying on this and remove the cache completely.
	cache *serviceCache
	//endpointsliceCache nodesSynced cache.InformerSynced
	serviceLister             corelisters.ServiceLister
	endpointSliceLister       discoverylisters.EndpointSliceLister
	serviceListerSynced       cache.InformerSynced
	endpointSliceListerSynced cache.InformerSynced
	eventBroadcaster          record.EventBroadcaster
	eventRecorder             record.EventRecorder
	nodeLister                corelisters.NodeLister
	nodeListerSynced          cache.InformerSynced
	// services and nodes that need to be synced
	serviceQueue       workqueue.RateLimitingInterface
	endpointsliceQueue workqueue.RateLimitingInterface
	nodeQueue          workqueue.RateLimitingInterface
	// lastSyncedNodes is used when reconciling node state and keeps track of
	// the last synced set of nodes per service key. This is accessed from the
	// service and node controllers, hence it is protected by a lock.
	lastSyncedNodes     map[string][]*v1.Node
	lastSyncedNodesLock sync.Mutex
}

// New returns a new service controller to keep cloud provider service resources
// (like load balancers) in sync with the registry.
func New(
	cloud cloudprovider.Interface,
	kubeClient clientset.Interface,
	serviceInformer coreinformers.ServiceInformer,
	endpointSliceInformer discoveryinformers.EndpointSliceInformer,
	nodeInformer coreinformers.NodeInformer,
	clusterName string,
	featureGate featuregate.FeatureGate,
) (*Controller, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "service-controller"})

	registerMetrics()
	s := &Controller{
		cloud:               cloud,
		kubeClient:          kubeClient,
		clusterName:         clusterName,
		cache:               &serviceCache{serviceMap: make(map[string]*cachedService)},
		eventBroadcaster:    broadcaster,
		eventRecorder:       recorder,
		nodeLister:          nodeInformer.Lister(),
		endpointSliceLister: endpointSliceInformer.Lister(),
		nodeListerSynced:    nodeInformer.Informer().HasSynced,
		serviceQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(minRetryDelay, maxRetryDelay), "service"),
		endpointsliceQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(minRetryDelay, maxRetryDelay), "endpointslice"),
		nodeQueue:           workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(minRetryDelay, maxRetryDelay), "node"),
		lastSyncedNodes:     make(map[string][]*v1.Node),
	}

	serviceInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(cur interface{}) {
				svc, ok := cur.(*v1.Service)
				// Check cleanup here can provide a remedy when controller failed to handle
				// changes before it exiting (e.g. crashing, restart, etc.).
				if ok && (wantsLoadBalancer(svc) || needsCleanup(svc)) {
					s.enqueueService(cur)
				}
			},
			UpdateFunc: func(old, cur interface{}) {

				oldSvc, ok1 := old.(*v1.Service)
				curSvc, ok2 := cur.(*v1.Service)
				if !(ok1 && ok2) {
					return
				}
				oldSvcId := oldSvc.Annotations[ServiceAnnotationLoadBalancerID]
				oldSvcNewId := oldSvc.Annotations[ServiceAnnotationLoadBalancerOldID]
				newSvcId := curSvc.Annotations[ServiceAnnotationLoadBalancerID]
				newSvcNewId := curSvc.Annotations[ServiceAnnotationLoadBalancerOldID]

				if len(oldSvcId) == 0 && len(oldSvcNewId) == 0 &&
					len(newSvcId) == 0 && len(newSvcNewId) == 0 &&
					!(wantsLoadBalancer(curSvc) || needsCleanup(curSvc)) {
					return
				}
				// 兜底所有svc玉lb的绑定关系
				s.enqueueService(cur)
			},
			// No need to handle deletion event because the deletion would be handled by
			// the update path when the deletion timestamp is added.
		},
		serviceSyncPeriod,
	)
	s.serviceLister = serviceInformer.Lister()
	s.serviceListerSynced = serviceInformer.Informer().HasSynced

	endpointSliceInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(cur interface{}) {
				// 王玉东屏蔽
				//klog.V(1).Infof("begin endpointSliceInformer--addFunc" )
				eps, ok := cur.(*discoveryv1.EndpointSlice)
				// 王玉东屏蔽
				//klog.V(1).Infof("begin endpointSliceInformer--addFunc,eps:%v+",eps )
				// Check cleanup here can provide a remedy when controller failed to handle
				// changes before it exiting (e.g. crashing, restart, etc.).
				// TODO 是否判断service是否有效,以及service
				if ok && epsHaveServiceName(eps) {
					// serviceName :=
					svc, err := serviceInformer.Lister().Services(eps.Namespace).Get(eps.Labels[KubernetesServiceName])
					if err != nil || svc == nil {
						// TODO 判断err是否存在
						return
					}

					// skip掉svc do not  belong lb 管理情况
					if !(wantsLoadBalancer(svc) || needsCleanup(svc)) {
						return
					}
					s.enqueueService(svc)
				}
			},
			UpdateFunc: func(old, cur interface{}) {
				oldEps, ok1 := old.(*discoveryv1.EndpointSlice)
				curEps, ok2 := cur.(*discoveryv1.EndpointSlice)
				if ok1 && ok2 && (epsHaveServiceName(oldEps) || epsHaveServiceName(curEps)) && (s.epsNeedsUpdate(oldEps, curEps) || epsNeedsCleanup(curEps)) {
					klog.Info("endpoint update, enqueueService")
					var svc *v1.Service
					var err error
					if epsHaveServiceName(curEps) {
						svc, err = serviceInformer.Lister().Services(curEps.Namespace).Get(curEps.Labels[KubernetesServiceName])
					} else if epsHaveServiceName(oldEps) {
						svc, err = serviceInformer.Lister().Services(curEps.Namespace).Get(oldEps.Labels[KubernetesServiceName])
					}

					if err != nil || svc == nil {
						// TODO 判断err是否存在
						return
					}

					// skip掉svc do not  belong lb 管理情况
					if !(wantsLoadBalancer(svc) || needsCleanup(svc)) {
						return
					}
					s.enqueueService(svc)
				}
			},
			// No need to handle deletion event because the deletion would be handled by
			// the update path when the deletion timestamp is added.
		},
		30*endpointSliceSyncPeriod,
	)

	s.endpointSliceLister = endpointSliceInformer.Lister()
	s.endpointSliceListerSynced = endpointSliceInformer.Informer().HasSynced

	nodeInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(cur interface{}) {
				s.enqueueNode(cur)
			},
			UpdateFunc: func(old, cur interface{}) {
				oldNode, ok := old.(*v1.Node)
				if !ok {
					return
				}

				curNode, ok := cur.(*v1.Node)
				if !ok {
					return
				}

				if !shouldSyncUpdatedNode(oldNode, curNode) {
					return
				}

				s.enqueueNode(curNode)
			},
			DeleteFunc: func(old interface{}) {
				s.enqueueNode(old)
			},
		},
		nodeSyncPeriod,
	)

	if err := s.init(); err != nil {
		return nil, err
	}

	return s, nil
}

// obj could be an *v1.Service, or a DeletionFinalStateUnknown marker item.
func (c *Controller) enqueueService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	c.serviceQueue.Add(key)
}

// obj could be an *v1.Service, or a DeletionFinalStateUnknown marker item.
func (c *Controller) enqueueNode(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	c.nodeQueue.Add(key)
}

// Run starts a background goroutine that watches for changes to services that
// have (or had) LoadBalancers=true and ensures that they have
// load balancers created and deleted appropriately.
// serviceSyncPeriod controls how often we check the cluster's services to
// ensure that the correct load balancers exist.
// nodeSyncPeriod controls how often we check the cluster's nodes to determine
// if load balancers need to be updated to point to a new set.
//
// It's an error to call Run() more than once for a given ServiceController
// object.
func (c *Controller) Run(ctx context.Context, workers int, controllerManagerMetrics *controllersmetrics.ControllerManagerMetrics) {
	defer runtime.HandleCrash()
	defer c.serviceQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.endpointsliceQueue.ShutDown()

	// Start event processing pipeline.
	c.eventBroadcaster.StartStructuredLogging(0)
	c.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: c.kubeClient.CoreV1().Events("")})
	defer c.eventBroadcaster.Shutdown()

	klog.Info("Starting service controller")
	defer klog.Info("Shutting down service controller")
	controllerManagerMetrics.ControllerStarted("service")
	defer controllerManagerMetrics.ControllerStopped("service")

	if !cache.WaitForNamedCacheSync("service", ctx.Done(), c.serviceListerSynced, c.nodeListerSynced, c.endpointSliceListerSynced) {
		return
	}

	cm, err := c.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "icks-cluster-info", metav1.GetOptions{})
	if err == nil || cm == nil || cm.Data["cni"] == "calico" {
		klog.Warningf("cloud-provider-openstack  not support cni: %v ", "calico")
		runtime.HandleCrash()
	}

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.serviceWorker, time.Second)
	}

	// Initialize one go-routine servicing node events. This ensure we only
	// process one node at any given moment in time
	// TODO  wangyudong 屏蔽
	go wait.UntilWithContext(ctx, func(ctx context.Context) { c.nodeWorker(ctx, workers) }, time.Second)

	<-ctx.Done()
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Controller) serviceWorker(ctx context.Context) {
	for c.processNextServiceItem(ctx) {
	}
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Controller) nodeWorker(ctx context.Context, workers int) {
	for c.processNextNodeItem(ctx, workers) {
	}
}

func (c *Controller) processNextNodeItem(ctx context.Context, workers int) bool {
	key, quit := c.nodeQueue.Get()
	if quit {
		return false
	}
	defer c.nodeQueue.Done(key)

	for serviceToRetry := range c.syncNodes(ctx, workers) {
		c.serviceQueue.Add(serviceToRetry)
	}

	c.nodeQueue.Forget(key)
	return true
}

func (c *Controller) processNextServiceItem(ctx context.Context) bool {
	key, quit := c.serviceQueue.Get()
	if quit {
		return false
	}
	defer c.serviceQueue.Done(key)

	err := c.syncService(ctx, key.(string))
	if err == nil {
		c.serviceQueue.Forget(key)
		return true
	}

	var re *api.RetryError
	if errors.As(err, &re) {
		klog.Warningf("error processing service %v (retrying in %s): %v", key, re.RetryAfter(), err)
		c.serviceQueue.AddAfter(key, re.RetryAfter())
	} else {
		runtime.HandleError(fmt.Errorf("error processing service %v (retrying with exponential backoff): %v", key, err))
		c.serviceQueue.AddRateLimited(key)
	}

	return true
}

func (c *Controller) init() error {
	if c.cloud == nil {
		return fmt.Errorf("WARNING: no cloud provider provided, services of type LoadBalancer will fail")
	}

	balancer, ok := c.cloud.LoadBalancer()
	if !ok {
		return fmt.Errorf("the cloud provider does not support external load balancers")
	}
	c.balancer = balancer

	return nil
}

// processServiceCreateOrUpdate operates loadbalancers for the incoming service accordingly.
// Returns an error if processing the service update failed.
func (c *Controller) processServiceCreateOrUpdate(ctx context.Context, service *v1.Service, key string, endpointSlices []*discoveryv1.EndpointSlice) error {
	// TODO(@MrHohn): Remove the cache once we get rid of the non-finalizer deletion
	// path. Ref https://github.com/kubernetes/enhancements/issues/980.
	cachedService := c.cache.getOrCreate(key)
	if cachedService.state != nil && cachedService.state.UID != service.UID {
		// This happens only when a service is deleted and re-created
		// in a short period, which is only possible when it doesn't
		// contain finalizer.

		lbID := getStringFromServiceAnnotation(cachedService.state, ServiceAnnotationLoadBalancerID, "")
		if err := c.processLoadBalancerDelete(ctx, cachedService.state, key, lbID); err != nil {
			return err
		}

		oldLbID := getStringFromServiceAnnotation(cachedService.state, ServiceAnnotationLoadBalancerOldID, "")
		if err := c.processLoadBalancerDelete(ctx, cachedService.state, key, oldLbID); err != nil {
			return err
		}
	}
	// Always cache the service, we need the info for service deletion in case
	// when load balancer cleanup is not handled via finalizer.
	cachedService.state = service
	op, err := c.syncLoadBalancerIfNeeded(ctx, service, key, endpointSlices)
	if err != nil {
		c.eventRecorder.Eventf(service, v1.EventTypeWarning, "SyncLoadBalancerFailed", "Error syncing load balancer: %v", err)
		return err
	}
	if op == deleteLoadBalancer {
		// Only delete the cache upon successful load balancer deletion.
		c.cache.delete(key)
	}

	return nil
}

type loadBalancerOperation int

const (
	deleteLoadBalancer loadBalancerOperation = iota
	ensureLoadBalancer
	maxNodeNamesToLog = 20
)

// syncLoadBalancerIfNeeded ensures that service's status is synced up with loadbalancer
// i.e. creates loadbalancer for service if requested and deletes loadbalancer if the service
// doesn't want a loadbalancer no more. Returns whatever error occurred.
func (c *Controller) syncLoadBalancerIfNeeded(ctx context.Context, service *v1.Service, key string, endpointSlices []*discoveryv1.EndpointSlice) (loadBalancerOperation, error) {
	// Note: It is safe to just call EnsureLoadBalancer.  But, on some clouds that requires a delete & create,
	// which may involve service interruption.  Also, we would like user-friendly events.

	// Save the state so we can avoid a write if it doesn't change
	previousStatus := service.Status.LoadBalancer.DeepCopy()
	var newStatus *v1.LoadBalancerStatus
	var err error
	var op loadBalancerOperation

	// service 删除操作
	if !wantsLoadBalancer(service) || needsCleanup(service) {
		// Delete the load balancer if service no longer wants one, or if service needs cleanup.
		op = deleteLoadBalancer
		newStatus = &v1.LoadBalancerStatus{}

		oldLbID := getStringFromServiceAnnotation(service, ServiceAnnotationLoadBalancerOldID, "")
		if len(oldLbID) != 0 {
			err := c.processLoadBalancerDelete(ctx, service, "", oldLbID)
			if err != nil {
				return op, fmt.Errorf("failed to ensure load balancer'processLoadBalancerDelete err: %w", err)
			}
		}

		lbID := getStringFromServiceAnnotation(service, ServiceAnnotationLoadBalancerID, "")
		if len(lbID) != 0 {
			err := c.processLoadBalancerDelete(ctx, service, "", lbID)
			if err != nil {
				return op, fmt.Errorf("failed to ensure load balancer'processLoadBalancerDelete err: %w", err)
			}
		}

		// Always remove finalizer when load balancer is deleted, this ensures Services
		// can be deleted after all corresponding load balancer resources are deleted.
		if err := c.removeFinalizer(service); err != nil {
			return op, fmt.Errorf("failed to remove load balancer cleanup finalizer: %v", err)
		}
		// remove new loadbalace annotation
		if err := c.removeAnnotationLbId(service, ServiceAnnotationLoadBalancerID); err != nil {
			return op, err
		}

		c.eventRecorder.Event(service, v1.EventTypeNormal, "DeletedLoadBalancer", "Deleted load balancer")
	} else {
		// service创建、更新操作
		// Create or update the load balancer if service wants one.
		op = ensureLoadBalancer
		klog.V(2).Infof("Ensuring load balancer for service %s", key)

		// Always add a finalizer prior to creating load balancers, this ensures Services
		// can't be deleted until all corresponding load balancer resources are also deleted.
		if err := c.addFinalizer(service); err != nil {
			return op, fmt.Errorf("failed to add load balancer cleanup finalizer: %v", err)
		}

		for _, eps := range endpointSlices {
			if err := c.addEndpointSliceFinalizer(eps); err != nil {
				return op, fmt.Errorf("failed to add load balancer cleanup finalizer to eps:%+v, err: %v", eps, err)
			}
		}

		// 处理旧的oldLoadbalancer 使用ensureLoadBalancerDeleted
		oldLbID := getStringFromServiceAnnotation(service, ServiceAnnotationLoadBalancerOldID, "")
		if len(oldLbID) != 0 {
			err := c.processLoadBalancerDelete(ctx, service, "", oldLbID)
			// only remove oldlbID，newstatus is remove all status. other  newstatus is add or update status
			newStatus = &v1.LoadBalancerStatus{}
			if err != nil {
				return op, fmt.Errorf("failed to delete  old load balancer,loadbalancer id: %s, err: %w", oldLbID, err)
			}
		}

		//  处理新的new Loadbalancer
		lbID := getStringFromServiceAnnotation(service, ServiceAnnotationLoadBalancerID, "")
		if len(lbID) != 0 {
			newStatus, err = c.ensureLoadBalancer(ctx, service, endpointSlices, lbID)
			if err != nil {
				if err == cloudprovider.ImplementedElsewhere {
					// ImplementedElsewhere indicates that the ensureLoadBalancer is a nop and the
					// functionality is implemented by a different controller.  In this case, we
					// return immediately without doing anything.
					klog.V(4).Infof("LoadBalancer for service %s implemented by a different controller %s, Ignoring error", key, c.cloud.ProviderName())
					return op, nil
				}
				if strings.Contains(strings.ToLower(err.Error()), "conflict") {
					// ImplementedElsewhere indicates that the ensureLoadBalancer is a nop and the
					// functionality is implemented by a different controller.  In this case, we
					// return immediately without doing anything.
					c.eventRecorder.Event(service, v1.EventTypeWarning, "conflict", strings.ToLower(err.Error()))
					return op, nil
				}
				// Use %w deliberately so that a returned RetryError can be handled.
				return op, fmt.Errorf("failed to ensure load balancer: %w", err)
			}
			if newStatus == nil {
				return op, fmt.Errorf("service status returned by EnsureLoadBalancer is nil")
			}
		}
	}
	//c.eventRecorder.Event(service, v1.EventTypeNormal, "EnsuringLoadBalancer", "Ensuring load balancer")

	// remove  finalizer, when eps have DeletionTimestamp
	if err := c.removeEndpointSliceFinalizerByService(service); err != nil {
		return op, err
	}

	// remove old loadbalace annotation
	if err := c.removeAnnotationLbId(service, ServiceAnnotationLoadBalancerOldID); err != nil {
		return op, err
	}
	klog.V(4).Infof("previousStatus  %v,newStatus %v", previousStatus, newStatus)

	// TODO 处理service的status,并且一处oldLBID
	if err := c.patchStatus(service, previousStatus, newStatus); err != nil {
		// Only retry error that isn't not found:
		// - Not found error mostly happens when service disappears right after
		//   we remove the finalizer.
		// - We can't patch status on non-exist service anyway.
		if !apierrors.IsNotFound(err) {
			return op, fmt.Errorf("failed to update load balancer status: %v", err)
		}
	}

	return op, nil
}

func (c *Controller) ensureLoadBalancer(ctx context.Context, service *v1.Service, endpointSlices []*discoveryv1.EndpointSlice, lbID string) (*v1.LoadBalancerStatus, error) {
	// - Not all cloud providers support all protocols and the next step is expected to return
	//   an error for unsupported protocols
	status, err := c.balancer.EnsureLoadBalancer(ctx, c.clusterName, service, nil, endpointSlices, lbID)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (c *Controller) storeLastSyncedNodes(svc *v1.Service, nodes []*v1.Node) {
	c.lastSyncedNodesLock.Lock()
	defer c.lastSyncedNodesLock.Unlock()
	key, _ := cache.MetaNamespaceKeyFunc(svc)
	c.lastSyncedNodes[key] = nodes
}

func (c *Controller) getLastSyncedNodes(svc *v1.Service) []*v1.Node {
	c.lastSyncedNodesLock.Lock()
	defer c.lastSyncedNodesLock.Unlock()
	key, _ := cache.MetaNamespaceKeyFunc(svc)
	return c.lastSyncedNodes[key]
}

// ListKeys implements the interface required by DeltaFIFO to list the keys we
// already know about.
func (s *serviceCache) ListKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.serviceMap))
	for k := range s.serviceMap {
		keys = append(keys, k)
	}
	return keys
}

// GetByKey returns the value stored in the serviceMap under the given key
func (s *serviceCache) GetByKey(key string) (interface{}, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.serviceMap[key]; ok {
		return v, true, nil
	}
	return nil, false, nil
}

// ListKeys implements the interface required by DeltaFIFO to list the keys we
// already know about.
func (s *serviceCache) allServices() []*v1.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	services := make([]*v1.Service, 0, len(s.serviceMap))
	for _, v := range s.serviceMap {
		services = append(services, v.state)
	}
	return services
}

func (s *serviceCache) get(serviceName string) (*cachedService, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service, ok := s.serviceMap[serviceName]
	return service, ok
}

func (s *serviceCache) getOrCreate(serviceName string) *cachedService {
	s.mu.Lock()
	defer s.mu.Unlock()
	service, ok := s.serviceMap[serviceName]
	if !ok {
		service = &cachedService{}
		s.serviceMap[serviceName] = service
	}
	return service
}

func (s *serviceCache) set(serviceName string, service *cachedService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceMap[serviceName] = service
}

func (s *serviceCache) delete(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.serviceMap, serviceName)
}

// needsCleanup checks if load balancer needs to be cleaned up as indicated by finalizer.
func needsCleanup(service *v1.Service) bool {
	if !servicehelper.HasLBFinalizer(service) {
		return false
	}

	if service.ObjectMeta.DeletionTimestamp != nil {
		return true
	}

	// Service doesn't want loadBalancer but owns loadBalancer finalizer also need to be cleaned up.
	if service.Spec.Type != v1.ServiceTypeLoadBalancer {
		return true
	}

	return false
}

// epsNeedsCleanup checks if load balancer needs to be cleaned up as indicated by finalizer.
func epsNeedsCleanup(eps *discoveryv1.EndpointSlice) bool {
	if !endpointSliceHelper.HasLBFinalizer(eps) {
		return false
	}

	if eps.ObjectMeta.DeletionTimestamp != nil {
		return true
	}

	// TODO 是否判断其他资源信息
	return false
}

func epsHaveServiceName(eps *discoveryv1.EndpointSlice) bool {
	if len(eps.Labels) > 0 && len(eps.Labels[KubernetesServiceName]) > 0 {
		return true
	}
	return false
}

// needsUpdate checks if load balancer needs to be updated due to change in attributes.
func (c *Controller) needsUpdate(oldService *v1.Service, newService *v1.Service) bool {
	if !wantsLoadBalancer(oldService) && !wantsLoadBalancer(newService) {
		return false
	}
	if wantsLoadBalancer(oldService) != wantsLoadBalancer(newService) {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "Type", "%v -> %v",
			oldService.Spec.Type, newService.Spec.Type)
		return true
	}

	if wantsLoadBalancer(newService) && !reflect.DeepEqual(oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges) {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "LoadBalancerSourceRanges", "%v -> %v",
			oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges)
		return true
	}

	if !portsEqualForLB(oldService, newService) || oldService.Spec.SessionAffinity != newService.Spec.SessionAffinity {
		return true
	}

	if !reflect.DeepEqual(oldService.Spec.SessionAffinityConfig, newService.Spec.SessionAffinityConfig) {
		return true
	}
	if !loadBalancerIPsAreEqual(oldService, newService) {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "LoadbalancerIP", "%v -> %v",
			oldService.Spec.LoadBalancerIP, newService.Spec.LoadBalancerIP)
		return true
	}
	if len(oldService.Spec.ExternalIPs) != len(newService.Spec.ExternalIPs) {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "ExternalIP", "Count: %v -> %v",
			len(oldService.Spec.ExternalIPs), len(newService.Spec.ExternalIPs))
		return true
	}
	for i := range oldService.Spec.ExternalIPs {
		if oldService.Spec.ExternalIPs[i] != newService.Spec.ExternalIPs[i] {
			c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "ExternalIP", "Added: %v",
				newService.Spec.ExternalIPs[i])
			return true
		}
	}
	if !reflect.DeepEqual(oldService.Annotations, newService.Annotations) {
		return true
	}
	if oldService.UID != newService.UID {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "UID", "%v -> %v",
			oldService.UID, newService.UID)
		return true
	}
	if oldService.Spec.ExternalTrafficPolicy != newService.Spec.ExternalTrafficPolicy {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "ExternalTrafficPolicy", "%v -> %v",
			oldService.Spec.ExternalTrafficPolicy, newService.Spec.ExternalTrafficPolicy)
		return true
	}
	if oldService.Spec.HealthCheckNodePort != newService.Spec.HealthCheckNodePort {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "HealthCheckNodePort", "%v -> %v",
			oldService.Spec.HealthCheckNodePort, newService.Spec.HealthCheckNodePort)
		return true
	}

	// User can upgrade (add another clusterIP or ipFamily) or can downgrade (remove secondary clusterIP or ipFamily),
	// but CAN NOT change primary/secondary clusterIP || ipFamily UNLESS they are changing from/to/ON ExternalName
	// so not care about order, only need check the length.
	if len(oldService.Spec.IPFamilies) != len(newService.Spec.IPFamilies) {
		c.eventRecorder.Eventf(newService, v1.EventTypeNormal, "IPFamilies", "Count: %v -> %v",
			len(oldService.Spec.IPFamilies), len(newService.Spec.IPFamilies))
		return true
	}

	return false
}

// needsUpdate checks if load balancer needs to be updated due to change in attributes.
func (c *Controller) epsNeedsUpdate(oldEps *discoveryv1.EndpointSlice, newEps *discoveryv1.EndpointSlice) bool {

	// 排除非ipv4、ipv6类型
	if !epSupportIpProtocol(oldEps) && !epSupportIpProtocol(newEps) {
		return false
	}

	// AddressType、Ports、Endpoints有一个变化则需要改动lb的memeber
	if !reflect.DeepEqual(oldEps.AddressType, newEps.AddressType) ||
		!reflect.DeepEqual(oldEps.Ports, newEps.Ports) ||
		!reflect.DeepEqual(oldEps.Endpoints, newEps.Endpoints) {
		return true
	}

	// TODO 是否需要在eps添加注解

	return false
}

func getPortsForLB(service *v1.Service) []*v1.ServicePort {
	ports := []*v1.ServicePort{}
	for i := range service.Spec.Ports {
		sp := &service.Spec.Ports[i]
		ports = append(ports, sp)
	}
	return ports
}

func portsEqualForLB(x, y *v1.Service) bool {
	xPorts := getPortsForLB(x)
	yPorts := getPortsForLB(y)
	return portSlicesEqualForLB(xPorts, yPorts)
}

func portSlicesEqualForLB(x, y []*v1.ServicePort) bool {
	if len(x) != len(y) {
		return false
	}

	for i := range x {
		if !portEqualForLB(x[i], y[i]) {
			return false
		}
	}
	return true
}

func portEqualForLB(x, y *v1.ServicePort) bool {
	// TODO: Should we check name?  (In theory, an LB could expose it)
	if x.Name != y.Name {
		return false
	}

	if x.Protocol != y.Protocol {
		return false
	}

	if x.Port != y.Port {
		return false
	}

	if x.NodePort != y.NodePort {
		return false
	}

	if x.TargetPort != y.TargetPort {
		return false
	}

	if !reflect.DeepEqual(x.AppProtocol, y.AppProtocol) {
		return false
	}

	return true
}

func nodeNames(nodes []*v1.Node) sets.String {
	ret := sets.NewString()
	for _, node := range nodes {
		ret.Insert(node.Name)
	}
	return ret
}

func loggableNodeNames(nodes []*v1.Node) []string {
	if len(nodes) > maxNodeNamesToLog {
		skipped := len(nodes) - maxNodeNamesToLog
		names := nodeNames(nodes[:maxNodeNamesToLog]).List()
		return append(names, fmt.Sprintf("<%d more>", skipped))
	}
	return nodeNames(nodes).List()
}

func shouldSyncUpdatedNode(oldNode, newNode *v1.Node) bool {
	// Evaluate the individual node exclusion predicate before evaluating the
	// compounded result of all predicates. We don't sync changes on the
	// readiness condition for eTP:Local services or when
	// StableLoadBalancerNodeSet is enabled, hence if a node remains NotReady
	// and a user adds the exclusion label we will need to sync as to make sure
	// this change is reflected correctly on ETP=local services. The sync
	// function compares lastSyncedNodes with the new (existing) set of nodes
	// for each service, so services which are synced with the same set of nodes
	// should be skipped internally in the sync function. This is needed as to
	// trigger a global sync for all services and make sure no service gets
	// skipped due to a changing node predicate.
	if respectsPredicates(oldNode, nodeIncludedPredicate) != respectsPredicates(newNode, nodeIncludedPredicate) {
		return true
	}
	// For the same reason as above, also check for any change to the providerID
	if oldNode.Spec.ProviderID != newNode.Spec.ProviderID {
		return true
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.StableLoadBalancerNodeSet) {
		return respectsPredicates(oldNode, allNodePredicates...) != respectsPredicates(newNode, allNodePredicates...)
	}
	return false
}

// syncNodes handles updating the hosts pointed to by all load
// balancers whenever the set of nodes in the cluster changes.
func (c *Controller) syncNodes(ctx context.Context, workers int) sets.String {
	startTime := time.Now()
	defer func() {
		latency := time.Since(startTime).Seconds()
		klog.V(4).Infof("It took %v seconds to finish syncNodes", latency)
		nodeSyncLatency.Observe(latency)
	}()

	klog.V(2).Infof("Syncing backends for all LB services.")
	servicesToUpdate := c.cache.allServices()
	numServices := len(servicesToUpdate)
	servicesToRetry := c.updateLoadBalancerHosts(ctx, servicesToUpdate, workers)
	klog.V(2).Infof("Successfully updated %d out of %d load balancers to direct traffic to the updated set of nodes",
		numServices-len(servicesToRetry), numServices)
	return servicesToRetry
}

// nodeSyncService syncs the nodes for one load balancer type service. The return value
// indicates if we should retry. Hence, this functions returns false if we've updated
// load balancers and finished doing it successfully, or didn't try to at all because
// there's no need. This function returns true if we tried to update load balancers and
// failed, indicating to the caller that we should try again.
func (c *Controller) nodeSyncService(svc *v1.Service) bool {
	const retSuccess = false
	const retNeedRetry = true
	if svc == nil || !wantsLoadBalancer(svc) {
		return retSuccess
	}
	newNodes, err := listWithPredicates(c.nodeLister)
	if err != nil {
		runtime.HandleError(fmt.Errorf("failed to retrieve node list: %v", err))
		nodeSyncErrorCount.Inc()
		return retNeedRetry
	}
	newNodes = filterWithPredicates(newNodes, getNodePredicatesForService(svc)...)
	oldNodes := filterWithPredicates(c.getLastSyncedNodes(svc), getNodePredicatesForService(svc)...)
	// Store last synced nodes without actually determining if we successfully
	// synced them or not. Failed node syncs are passed off to retries in the
	// service queue, so no need to wait. If we don't store it now, we risk
	// re-syncing all LBs twice, one from another sync in the node sync and
	// from the service sync
	c.storeLastSyncedNodes(svc, newNodes)
	if nodesSufficientlyEqual(oldNodes, newNodes) {
		return retSuccess
	}
	klog.V(4).Infof("nodeSyncService started for service %s/%s", svc.Namespace, svc.Name)
	if err := c.lockedUpdateLoadBalancerHosts(svc, newNodes); err != nil {
		runtime.HandleError(fmt.Errorf("failed to update load balancer hosts for service %s/%s: %v", svc.Namespace, svc.Name, err))
		nodeSyncErrorCount.Inc()
		return retNeedRetry
	}
	klog.V(4).Infof("nodeSyncService finished successfully for service %s/%s", svc.Namespace, svc.Name)
	return retSuccess
}

func nodesSufficientlyEqual(oldNodes, newNodes []*v1.Node) bool {
	if len(oldNodes) != len(newNodes) {
		return false
	}

	// This holds the Node fields which trigger a sync when changed.
	type protoNode struct {
		providerID string
	}
	distill := func(n *v1.Node) protoNode {
		return protoNode{
			providerID: n.Spec.ProviderID,
		}
	}

	mOld := map[string]protoNode{}
	for _, n := range oldNodes {
		mOld[n.Name] = distill(n)
	}

	mNew := map[string]protoNode{}
	for _, n := range newNodes {
		mNew[n.Name] = distill(n)
	}

	return reflect.DeepEqual(mOld, mNew)
}

// updateLoadBalancerHosts updates all existing load balancers so that
// they will match the latest list of nodes with input number of workers.
// Returns the list of services that couldn't be updated.
func (c *Controller) updateLoadBalancerHosts(ctx context.Context, services []*v1.Service, workers int) (servicesToRetry sets.String) {
	klog.V(4).Infof("Running updateLoadBalancerHosts(len(services)==%d, workers==%d)", len(services), workers)

	// lock for servicesToRetry
	servicesToRetry = sets.NewString()
	lock := sync.Mutex{}

	doWork := func(piece int) {
		if shouldRetry := c.nodeSyncService(services[piece]); !shouldRetry {
			return
		}
		lock.Lock()
		defer lock.Unlock()
		key := fmt.Sprintf("%s/%s", services[piece].Namespace, services[piece].Name)
		servicesToRetry.Insert(key)
	}
	workqueue.ParallelizeUntil(ctx, workers, len(services), doWork)
	klog.V(4).Infof("Finished updateLoadBalancerHosts")
	return servicesToRetry
}

// Updates the load balancer of a service, assuming we hold the mutex
// associated with the service.
func (c *Controller) lockedUpdateLoadBalancerHosts(service *v1.Service, hosts []*v1.Node) error {
	startTime := time.Now()
	loadBalancerSyncCount.Inc()
	defer func() {
		latency := time.Since(startTime).Seconds()
		klog.V(4).Infof("It took %v seconds to update load balancer hosts for service %s/%s", latency, service.Namespace, service.Name)
		updateLoadBalancerHostLatency.Observe(latency)
	}()
	klog.V(2).Infof("Updating backends for load balancer %s/%s with %d nodes: %v", service.Namespace, service.Name, len(hosts), loggableNodeNames(hosts))

	// This operation doesn't normally take very long (and happens pretty often), so we only record the final event
	err := c.balancer.UpdateLoadBalancer(context.TODO(), c.clusterName, service, hosts)
	if err == nil {
		// If there are no available nodes for LoadBalancer service, make a EventTypeWarning event for it.
		if len(hosts) == 0 {
			c.eventRecorder.Event(service, v1.EventTypeWarning, "UnAvailableLoadBalancer", "There are no available nodes for LoadBalancer")
		} else {
			c.eventRecorder.Event(service, v1.EventTypeNormal, "UpdatedLoadBalancer", "Updated load balancer with new hosts")
		}
		return nil
	}
	if err == cloudprovider.ImplementedElsewhere {
		// ImplementedElsewhere indicates that the UpdateLoadBalancer is a nop and the
		// functionality is implemented by a different controller.  In this case, we
		// return immediately without doing anything.
		return nil
	}
	// It's only an actual error if the load balancer still exists.
	if _, exists, err := c.balancer.GetLoadBalancer(context.TODO(), c.clusterName, service); err != nil {
		runtime.HandleError(fmt.Errorf("failed to check if load balancer exists for service %s/%s: %v", service.Namespace, service.Name, err))
	} else if !exists {
		return nil
	}

	c.eventRecorder.Eventf(service, v1.EventTypeWarning, "UpdateLoadBalancerFailed", "Error updating load balancer with new hosts %v [node names limited, total number of nodes: %d], error: %v", loggableNodeNames(hosts), len(hosts), err)
	return err
}

func epSupportIpProtocol(eps *discoveryv1.EndpointSlice) bool {
	// if LoadBalancerClass is set, the user does not want the default cloud-provider Load Balancer
	return eps.AddressType == discoveryv1.AddressTypeIPv4 || eps.AddressType == discoveryv1.AddressTypeIPv6
}

func wantsLoadBalancer(service *v1.Service) bool {
	// if LoadBalancerClass is set, the user does not want the default cloud-provider Load Balancer
	return service.Spec.Type == v1.ServiceTypeLoadBalancer && service.Spec.LoadBalancerClass == nil
}

func loadBalancerIPsAreEqual(oldService, newService *v1.Service) bool {
	return oldService.Spec.LoadBalancerIP == newService.Spec.LoadBalancerIP
}

// syncService will sync the Service with the given key if it has had its expectations fulfilled,
// meaning it did not expect to see any more of its pods created or deleted. This function is not meant to be
// invoked concurrently with the same key.
func (c *Controller) syncService(ctx context.Context, key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing service %q (%v)", key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	cm, err := c.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "icks-cluster-info", metav1.GetOptions{})
	if err != nil {
		return err
	}
	clusterId := cm.Data["clusterId"]
	if len(clusterId) == 0 {
		return fmt.Errorf("icks-cluster-info's configmap could not contain clusterId")
	}
	c.clusterName = clusterId

	// service holds the latest service info from apiserver
	service, err := c.serviceLister.Services(namespace).Get(name)
	switch {
	case apierrors.IsNotFound(err):
		// service absence in store means watcher caught the deletion, ensure LB info is cleaned
		err = c.processServiceDeletion(ctx, key)
	case err != nil:
		runtime.HandleError(fmt.Errorf("Unable to retrieve service %v from store: %v", key, err))
	default:
		epsLablelSelector := labels.Set(map[string]string{
			discoveryv1.LabelServiceName: service.Name,
		}).AsSelectorPreValidated()
		epss, err := c.endpointSliceLister.EndpointSlices(service.Namespace).List(epsLablelSelector)
		//klog.V(1).Infof("epss is %v,err:%v", epss, err)
		if err != nil && apierrors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Unable to retrieve eps by namesapce %v, labelSelector %v from store: %v", service.Namespace, epsLablelSelector, err))
			return nil
		}
		if err != nil && !apierrors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Unable to retrieve eps by namesapce %v, labelSelector %v from store: %v", service.Namespace, key, err))
			return err
		}
		// It is not safe to modify an object returned from an informer.
		// As reconcilers may modify the service object we need to copy
		// it first.
		err = c.processServiceCreateOrUpdate(ctx, service.DeepCopy(), key, epss)
	}

	return err
}

func (c *Controller) processServiceDeletion(ctx context.Context, key string) error {
	cachedService, ok := c.cache.get(key)
	if !ok {
		// Cache does not contains the key means:
		// - We didn't create a Load Balancer for the deleted service at all.
		// - We already deleted the Load Balancer that was created for the service.
		// In both cases we have nothing left to do.
		return nil
	}
	klog.V(2).Infof("Service %v has been deleted. Attempting to cleanup load balancer resources", key)
	/*if err := c.processLoadBalancerDelete(ctx, cachedService.state, key); err != nil {
		return err
	}*/
	lbID := getStringFromServiceAnnotation(cachedService.state, ServiceAnnotationLoadBalancerID, "")
	if err := c.processLoadBalancerDelete(ctx, cachedService.state, key, lbID); err != nil {
		return err
	}

	oldLbID := getStringFromServiceAnnotation(cachedService.state, ServiceAnnotationLoadBalancerOldID, "")
	if err := c.processLoadBalancerDelete(ctx, cachedService.state, key, oldLbID); err != nil {
		return err
	}

	c.cache.delete(key)
	return nil
}

func (c *Controller) processLoadBalancerDelete(ctx context.Context, service *v1.Service, key string, lbId string) error {

	//c.eventRecorder.Event(service, v1.EventTypeNormal, "DeletingLoadBalancer", "Deleting load balancer")
	if err := c.balancer.EnsureLoadBalancerDeleted(ctx, c.clusterName, service, lbId); err != nil {
        if strings.Contains(err.Error(), "not found"){
        	// ignore
        	return nil
		}
		c.eventRecorder.Eventf(service, v1.EventTypeWarning, "DeleteLoadBalancerFailed", "Error deleting load balancer: %v", err)
		return err
	}
	//c.eventRecorder.Event(service, v1.EventTypeNormal, "DeletedLoadBalancer", "Deleted load balancer")
	return nil
}

// addFinalizer patches the service to add finalizer.
func (c *Controller) addFinalizer(service *v1.Service) error {
	if servicehelper.HasLBFinalizer(service) {
		return nil
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.ObjectMeta.Finalizers = append(updated.ObjectMeta.Finalizers, servicehelper.LoadBalancerCleanupFinalizer)

	klog.V(1).Infof("Adding finalizer to service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// addFinalizer patches the service to add finalizer.
func (c *Controller) addEndpointSliceFinalizer(endpointslice *discoveryv1.EndpointSlice) error {
	return nil
	if endpointSliceHelper.HasLBFinalizer(endpointslice) {
		return nil
	}
	// Make a copy so we don't mutate the shared informer cache.
	updated := endpointslice.DeepCopy()
	updated.ObjectMeta.Finalizers = append(updated.ObjectMeta.Finalizers, endpointSliceHelper.LoadBalancerCleanupFinalizer)

	klog.V(2).Infof("Adding finalizer to endpointslice %s/%s", updated.Namespace, updated.Name)

	_, err := endpointSliceHelper.PatchEndpointSlice(c.kubeClient.DiscoveryV1(), endpointslice, updated)
	return err
}

// removeFinalizer patches the service to remove finalizer.
func (c *Controller) removeFinalizer(service *v1.Service) error {
	if !servicehelper.HasLBFinalizer(service) {
		return nil
	}
	if !needsCleanup(service) {
		return nil
	}
	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.ObjectMeta.Finalizers = removeString(updated.ObjectMeta.Finalizers, servicehelper.LoadBalancerCleanupFinalizer)

	klog.V(2).Infof("Removing finalizer from service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// removeSvcOldLbId patches the service to remove finalizer.
func (c *Controller) removeAnnotationLbId(service *v1.Service, annotation string) error {
	if !servicehelper.HasLBFinalizer(service) {
		//		return nil
	}

	if service.ObjectMeta.DeletionTimestamp != nil {
		return nil
	}

	// ServiceAnnotationLoadBalancerOldID
	if !servicehelper.HasAnnoation(service, annotation) {
		return nil
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.Annotations = removeAnnotationKey(updated.Annotations, annotation)
	klog.V(2).Infof("Removing old loadbalance annotation from service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// TODO 处理remove endpointslice的finalizer
// removeEndpointSliceFinalizer patches the endpointslice to remove finalizer.
func (c *Controller) removeEndpointSliceFinalizer(endpointslice *discoveryv1.EndpointSlice) error {
	if endpointSliceHelper.HasLBFinalizer(endpointslice) {
		return nil
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := endpointslice.DeepCopy()
	updated.ObjectMeta.Finalizers = removeString(updated.ObjectMeta.Finalizers, endpointSliceHelper.LoadBalancerCleanupFinalizer)

	klog.V(2).Infof("Removing finalizer from endpointslice %s/%s", updated.Namespace, updated.Name)
	_, err := endpointSliceHelper.PatchEndpointSlice(c.kubeClient.DiscoveryV1(), endpointslice, updated)
	return err
}

// removeEndpointSliceFinalizer patches the endpointslice to remove finalizer.
func (c *Controller) removeEndpointSliceFinalizerByService(service *v1.Service) error {
	// 王玉东 remove eps's  finalizer,when eps  belong to service
	epsLablelSelector := labels.Set(map[string]string{
		discoveryv1.LabelServiceName: service.Name,
	}).AsSelectorPreValidated()
	epss, err := c.endpointSliceLister.EndpointSlices(service.Namespace).List(epsLablelSelector)
	if err != nil {
		return fmt.Errorf("syncLoadBalancerIfNeeded: failed to get endpoints : %v", err)
	}
	klog.V(1).Infof("get %d eps by svc %s", len(epss), service.Name)
	for _, eps := range epss {
		if epsNeedsCleanup(eps) {
			if err := c.removeEndpointSliceFinalizer(eps); err != nil {
				return fmt.Errorf("failed to remove endpoints finalizer: %v", err)
			}
		}
	}
	return nil
}

// removeString returns a newly created []string that contains all items from slice that
// are not equal to s.
func removeString(slice []string, s string) []string {
	var newSlice []string
	for _, item := range slice {
		if item != s {
			newSlice = append(newSlice, item)
		}
	}
	return newSlice
}

// removeString returns a newly created []string that contains all items from slice that
// are not equal to s.
func removeAnnotationKey(annotation map[string]string, key string) map[string]string {
	newAnnotation := make(map[string]string)
	for oldAnnotation, value := range annotation {
		if !strings.EqualFold(oldAnnotation, key) {
			newAnnotation[oldAnnotation] = value
		}
	}
	return newAnnotation
}

// patchStatus patches the service with the given LoadBalancerStatus.
func (c *Controller) patchStatus(service *v1.Service, previousStatus, newStatus *v1.LoadBalancerStatus) error {
	if newStatus == nil {
		klog.V(4).Infof("newStatus  %v", newStatus)
		return nil
	}

	if previousStatus != nil && newStatus != nil {
		klog.V(4).Infof("previousStatus  %v,newStatus %v", previousStatus, newStatus)
		if servicehelper.LoadBalancerStatusEqual(previousStatus, newStatus) {
			return nil
		}
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.Status.LoadBalancer = *newStatus

	klog.V(2).Infof("Patching status for service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// NodeConditionPredicate is a function that indicates whether the given node's conditions meet
// some set of criteria defined by the function.
type NodeConditionPredicate func(node *v1.Node) bool

var (
	allNodePredicates []NodeConditionPredicate = []NodeConditionPredicate{
		nodeIncludedPredicate,
		nodeUnTaintedPredicate,
		nodeReadyPredicate,
	}
	etpLocalNodePredicates []NodeConditionPredicate = []NodeConditionPredicate{
		nodeIncludedPredicate,
		nodeUnTaintedPredicate,
		nodeReadyPredicate,
	}
	stableNodeSetPredicates []NodeConditionPredicate = []NodeConditionPredicate{
		nodeNotDeletedPredicate,
		nodeIncludedPredicate,
		// This is not perfect, but probably good enough. We won't update the
		// LBs just because the taint was added (see shouldSyncUpdatedNode) but
		// if any other situation causes an LB sync, tainted nodes will be
		// excluded at that time and cause connections on said node to not
		// connection drain.
		nodeUnTaintedPredicate,
	}
)

func getNodePredicatesForService(service *v1.Service) []NodeConditionPredicate {
	if utilfeature.DefaultFeatureGate.Enabled(features.StableLoadBalancerNodeSet) {
		return stableNodeSetPredicates
	}
	if service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyLocal {
		return etpLocalNodePredicates
	}
	return allNodePredicates
}

// We consider the node for load balancing only when the node is not labelled for exclusion.
func nodeIncludedPredicate(node *v1.Node) bool {
	_, hasExcludeBalancerLabel := node.Labels[v1.LabelNodeExcludeBalancers]
	return !hasExcludeBalancerLabel
}

// We consider the node for load balancing only when its not tainted for deletion by the cluster autoscaler.
func nodeUnTaintedPredicate(node *v1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == ToBeDeletedTaint {
			return false
		}
	}
	return true
}

// We consider the node for load balancing only when its NodeReady condition status is ConditionTrue
func nodeReadyPredicate(node *v1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			return cond.Status == v1.ConditionTrue
		}
	}
	return false
}

func nodeNotDeletedPredicate(node *v1.Node) bool {
	return node.DeletionTimestamp.IsZero()
}

// listWithPredicate gets nodes that matches all predicate functions.
func listWithPredicates(nodeLister corelisters.NodeLister, predicates ...NodeConditionPredicate) ([]*v1.Node, error) {
	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return filterWithPredicates(nodes, predicates...), nil
}

func filterWithPredicates(nodes []*v1.Node, predicates ...NodeConditionPredicate) []*v1.Node {
	var filtered []*v1.Node
	for i := range nodes {
		if respectsPredicates(nodes[i], predicates...) {
			filtered = append(filtered, nodes[i])
		}
	}
	return filtered
}

func respectsPredicates(node *v1.Node, predicates ...NodeConditionPredicate) bool {
	for _, p := range predicates {
		if !p(node) {
			return false
		}
	}
	return true
}

// getStringFromServiceAnnotation searches a given v1.Service for a specific annotationKey and either returns the annotation's value or a specified defaultSetting
func getStringFromServiceAnnotation(service *v1.Service, annotationKey string, defaultSetting string) string {
	klog.V(4).Infof("getStringFromServiceAnnotation(%s/%s, %v, %v)", service.Namespace, service.Name, annotationKey, defaultSetting)
	if annotationValue, ok := service.Annotations[annotationKey]; ok {
		//if there is an annotation for this setting, set the "setting" var to it
		// annotationValue can be empty, it is working as designed
		// it makes possible for instance provisioning loadbalancer without floatingip
		//klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
		return annotationValue
	}
	//if there is no annotation, set "settings" var to the value from cloud config
	if defaultSetting != "" {
		klog.V(4).Infof("Could not find a Service Annotation; falling back on cloud-config setting: %v = %v", annotationKey, defaultSetting)
	}
	return defaultSetting
}
