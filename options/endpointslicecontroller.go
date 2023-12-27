/*
Copyright 2018 The Kubernetes Authors.

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

package options

import (
	"github.com/spf13/pflag"
	 endpointSliceConfig "github.com/inspurDTest/cloud-provider/controllers/endpointslice/config"
)

// EndpointSliceOptions holds the EndpointSlice options.
type EndpointSliceOptions struct {
	*endpointSliceConfig.EndpointSliceControllerConfiguration
}

// AddFlags adds flags related to EndpointSlice for controller manager to the specified FlagSet.
func (o *EndpointSliceOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.Int32Var(&o.ConcurrentEndpointSliceSyncs, "concurrent-service-syncs", o.ConcurrentEndpointSliceSyncs, "The number of services that are allowed to sync concurrently. Larger number = more responsive service management, but more CPU (and network) load")
}

// ApplyTo fills up EndpointSlice config with options.
func (o *EndpointSliceOptions) ApplyTo(cfg *endpointSliceConfig.EndpointSliceControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentEndpointSliceSyncs = o.ConcurrentEndpointSliceSyncs

	return nil
}

// Validate checks validation of EndpointSliceOptions.
func (o *EndpointSliceOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	return errs
}
