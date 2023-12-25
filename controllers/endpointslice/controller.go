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
limitations undere the License.
*/

package endpointslice

import (
	"context"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	discoveryinformers "k8s.io/client-go/informers/discovery/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	discoverylisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	cloudprovider "github.com/inspurDTest/cloud-provider"
	"k8s.io/component-base/featuregate"
)

const (

	minRetryDelay = 5 * time.Second
	maxRetryDelay = 300 * time.Second
	// ToBeDeletedTaint is a taint used by the CLuster Autoscaler before marking a node for deletion. Defined in
	// https://github.com/kubernetes/autoscaler/blob/e80ab518340f88f364fe3ef063f8303755125971/cluster-autoscaler/utils/deletetaint/delete.go#L36
	ToBeDeletedTaint = "ToBeDeletedByClusterAutoscaler"

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
	cache               *serviceCache
	serviceLister       corelisters.ServiceLister
	endpointSliceLister discoverylisters.EndpointSliceLister
	serviceListerSynced cache.InformerSynced
	endpointSliceListerSynced cache.InformerSynced
	eventBroadcaster    record.EventBroadcaster
	eventRecorder       record.EventRecorder
	nodeLister          corelisters.NodeLister
	nodeListerSynced    cache.InformerSynced
	// services and nodes that need to be synced
	serviceQueue workqueue.RateLimitingInterface
	endpointsliceQueue workqueue.RateLimitingInterface
	nodeQueue    workqueue.RateLimitingInterface
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
	endpointSliceInformer discoveryinformers.EndpointSliceInformer,
	clusterName string,
	featureGate featuregate.FeatureGate,
) (*Controller, error) {
    // TODO 暂时只定义，不做处理。数据处理在service完成
	return nil, nil
}

func (c *Controller) processNextNodeItem(ctx context.Context, workers int) bool {
	// TODO
	return true
}

func (c *Controller) processNextEpsItem(ctx context.Context) bool {
	// TODO
	return true
}


func (c *Controller) processEpsCreateOrUpdate(ctx context.Context, service *v1.Service, key string) error {
	// TODO(@MrHohn): Remove the cache once we get rid of the non-finalizer deletion

	return nil
}

