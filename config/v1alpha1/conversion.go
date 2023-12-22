/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/conversion"
	cpconfig "github.com/inspurDTest/cloud-provider/config"
)

// Important! The public back-and-forth conversion functions for the types in this generic
// package with ComponentConfig types need to be manually exposed like this in order for
// other packages that reference this package to be able to call these conversion functions
// in an autogenerated manner.
// TODO: Fix the bug in conversion-gen so it automatically discovers these Convert_* functions
// in autogenerated code as well.

// Convert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration(in *KubeCloudSharedConfiguration, out *cpconfig.KubeCloudSharedConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration(in, out, s)
}

// Convert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration is an autogenerated conversion function.
func Convert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration(in *cpconfig.KubeCloudSharedConfiguration, out *KubeCloudSharedConfiguration, s conversion.Scope) error {
	return autoConvert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration(in, out, s)
}

// Convert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration(in *CloudProviderConfiguration, out *cpconfig.CloudProviderConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration(in, out, s)
}

// Convert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration is an autogenerated conversion function.
func Convert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration(in *cpconfig.CloudProviderConfiguration, out *CloudProviderConfiguration, s conversion.Scope) error {
	return autoConvert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration(in, out, s)
}
