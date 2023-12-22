/*
Copyright 2019 The Kubernetes Authors.

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

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	discoveryv1client "k8s.io/client-go/kubernetes/typed/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	utilnet "k8s.io/utils/net"
)

/*
This file is duplicated from "k8s.io/kubernetes/pkg/api/v1/service/util.go"
in order for in-tree cloud providers to not depend on internal packages.
*/

const (
	defaultLoadBalancerSourceRanges = "0.0.0.0/0"

	// LoadBalancerCleanupFinalizer is the finalizer added to load balancer
	// services to ensure the Service resource is not fully deleted until
	// the correlating load balancer resources are deleted.
	LoadBalancerCleanupFinalizer = "endpointslice.kubernetes.io/load-balancer-cleanup"
)

// IsAllowAll checks whether the utilnet.IPNet allows traffic from 0.0.0.0/0
func IsAllowAll(ipnets utilnet.IPNetSet) bool {
	for _, s := range ipnets.StringSlice() {
		if s == "0.0.0.0/0" {
			return true
		}
	}
	return false
}

// GetLoadBalancerSourceRanges first try to parse and verify LoadBalancerSourceRanges field from a service.
// If the field is not specified, turn to parse and verify the AnnotationLoadBalancerSourceRangesKey annotation from a service,
// extracting the source ranges to allow, and if not present returns a default (allow-all) value.
func GetLoadBalancerSourceRanges(service *v1.Service) (utilnet.IPNetSet, error) {
	var ipnets utilnet.IPNetSet
	var err error
	// if SourceRange field is specified, ignore sourceRange annotation
	if len(service.Spec.LoadBalancerSourceRanges) > 0 {
		specs := service.Spec.LoadBalancerSourceRanges
		ipnets, err = utilnet.ParseIPNets(specs...)

		if err != nil {
			return nil, fmt.Errorf("service.Spec.LoadBalancerSourceRanges: %v is not valid. Expecting a list of IP ranges. For example, 10.0.0.0/24. Error msg: %v", specs, err)
		}
	} else {
		val := service.Annotations[v1.AnnotationLoadBalancerSourceRangesKey]
		val = strings.TrimSpace(val)
		if val == "" {
			val = defaultLoadBalancerSourceRanges
		}
		specs := strings.Split(val, ",")
		ipnets, err = utilnet.ParseIPNets(specs...)
		if err != nil {
			return nil, fmt.Errorf("%s: %s is not valid. Expecting a comma-separated list of source IP ranges. For example, 10.0.0.0/24,192.168.2.0/24", v1.AnnotationLoadBalancerSourceRangesKey, val)
		}
	}
	return ipnets, nil
}

// GetServiceHealthCheckPathPort returns the path and nodePort programmed into the Cloud LB Health Check
func GetServiceHealthCheckPathPort(service *v1.Service) (string, int32) {
	if !NeedsHealthCheck(service) {
		return "", 0
	}
	port := service.Spec.HealthCheckNodePort
	if port == 0 {
		return "", 0
	}
	return "/healthz", port
}

// RequestsOnlyLocalTraffic checks if service requests OnlyLocal traffic.
func RequestsOnlyLocalTraffic(service *v1.Service) bool {
	if service.Spec.Type != v1.ServiceTypeLoadBalancer &&
		service.Spec.Type != v1.ServiceTypeNodePort {
		return false
	}
	return service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyLocal
}

// NeedsHealthCheck checks if service needs health check.
func NeedsHealthCheck(service *v1.Service) bool {
	if service.Spec.Type != v1.ServiceTypeLoadBalancer {
		return false
	}
	return RequestsOnlyLocalTraffic(service)
}

// HasLBFinalizer checks if endpointSlice contains LoadBalancerCleanupFinalizer.
func HasLBFinalizer(eps *discoveryv1.EndpointSlice) bool {
	for _, finalizer := range eps.ObjectMeta.Finalizers {
		if finalizer == LoadBalancerCleanupFinalizer {
			return true
		}
	}
	return false
}


// HasLBFinalizer checks if service contains LoadBalancerCleanupFinalizer.
func EPSHasLBFinalizer(service *v1.Service) bool {
	for _, finalizer := range service.ObjectMeta.Finalizers {
		if finalizer == LoadBalancerCleanupFinalizer {
			return true
		}
	}
	return false
}

// LoadBalancerStatusEqual checks if load balancer status are equal
func LoadBalancerStatusEqual(l, r *v1.LoadBalancerStatus) bool {
	return ingressSliceEqual(l.Ingress, r.Ingress)
}

// PatchEndpointSlice patches the given endpointSlice's Status or ObjectMeta based on the original and
// updated ones. Change to spec will be ignored.
func PatchEndpointSlice(c discoveryv1client.DiscoveryV1Interface, oldEps, newEps *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	// Reset spec to make sure only patch for Status or ObjectMeta.
	patchBytes, err := epsGetPatchBytes(oldEps, newEps)
	if err != nil {
		return nil, err
	}

	return c.EndpointSlices(oldEps.Namespace).Patch(context.TODO(), oldEps.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}, "")
}

func getPatchBytes(oldSvc, newSvc *v1.Service) ([]byte, error) {
	oldData, err := json.Marshal(oldSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal oldData for svc %s/%s: %v", oldSvc.Namespace, oldSvc.Name, err)
	}

	newData, err := json.Marshal(newSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal newData for svc %s/%s: %v", newSvc.Namespace, newSvc.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Service{})
	if err != nil {
		return nil, fmt.Errorf("failed to CreateTwoWayMergePatch for svc %s/%s: %v", oldSvc.Namespace, oldSvc.Name, err)
	}
	return patchBytes, nil

}

func epsGetPatchBytes(oldEps, newEps *discoveryv1.EndpointSlice) ([]byte, error) {
	oldData, err := json.Marshal(oldEps)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal oldData for svc %s/%s: %v", oldEps.Namespace, oldEps.Name, err)
	}

	newData, err := json.Marshal(newEps)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal newData for svc %s/%s: %v", newEps.Namespace, newEps.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, discoveryv1.EndpointSlice{})
	if err != nil {
		return nil, fmt.Errorf("failed to CreateTwoWayMergePatch for eps %s/%s: %v", oldEps.Namespace, newEps.Name, err)
	}
	return patchBytes, nil

}

func ingressSliceEqual(lhs, rhs []v1.LoadBalancerIngress) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for i := range lhs {
		if !ingressEqual(&lhs[i], &rhs[i]) {
			return false
		}
	}
	return true
}

func ingressEqual(lhs, rhs *v1.LoadBalancerIngress) bool {
	if lhs.IP != rhs.IP {
		return false
	}
	if lhs.Hostname != rhs.Hostname {
		return false
	}
	if lhs.IPMode != rhs.IPMode {
		return false
	}
	return true
}
