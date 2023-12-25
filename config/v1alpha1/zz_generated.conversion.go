//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1alpha1

import (
	unsafe "unsafe"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	config "github.com/inspurDTest/cloud-provider/config"
	nodeconfigv1alpha1 "github.com/inspurDTest/cloud-provider/controllers/node/config/v1alpha1"
	serviceconfigv1alpha1 "github.com/inspurDTest/cloud-provider/controllers/service/config/v1alpha1"
	configv1alpha1 "k8s.io/controller-manager/config/v1alpha1"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*CloudControllerManagerConfiguration)(nil), (*config.CloudControllerManagerConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha1_CloudControllerManagerConfiguration_To_config_CloudControllerManagerConfiguration(a.(*CloudControllerManagerConfiguration), b.(*config.CloudControllerManagerConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.CloudControllerManagerConfiguration)(nil), (*CloudControllerManagerConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_CloudControllerManagerConfiguration_To_v1alpha1_CloudControllerManagerConfiguration(a.(*config.CloudControllerManagerConfiguration), b.(*CloudControllerManagerConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*WebhookConfiguration)(nil), (*config.WebhookConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration(a.(*WebhookConfiguration), b.(*config.WebhookConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.WebhookConfiguration)(nil), (*WebhookConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration(a.(*config.WebhookConfiguration), b.(*WebhookConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*config.CloudProviderConfiguration)(nil), (*CloudProviderConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration(a.(*config.CloudProviderConfiguration), b.(*CloudProviderConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*config.KubeCloudSharedConfiguration)(nil), (*KubeCloudSharedConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration(a.(*config.KubeCloudSharedConfiguration), b.(*KubeCloudSharedConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*CloudProviderConfiguration)(nil), (*config.CloudProviderConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration(a.(*CloudProviderConfiguration), b.(*config.CloudProviderConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*KubeCloudSharedConfiguration)(nil), (*config.KubeCloudSharedConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration(a.(*KubeCloudSharedConfiguration), b.(*config.KubeCloudSharedConfiguration), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha1_CloudControllerManagerConfiguration_To_config_CloudControllerManagerConfiguration(in *CloudControllerManagerConfiguration, out *config.CloudControllerManagerConfiguration, s conversion.Scope) error {
	if err := configv1alpha1.Convert_v1alpha1_GenericControllerManagerConfiguration_To_config_GenericControllerManagerConfiguration(&in.Generic, &out.Generic, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration(&in.KubeCloudShared, &out.KubeCloudShared, s); err != nil {
		return err
	}
	if err := nodeconfigv1alpha1.Convert_v1alpha1_NodeControllerConfiguration_To_config_NodeControllerConfiguration(&in.NodeController, &out.NodeController, s); err != nil {
		return err
	}
	if err := serviceconfigv1alpha1.Convert_v1alpha1_ServiceControllerConfiguration_To_config_ServiceControllerConfiguration(&in.ServiceController, &out.ServiceController, s); err != nil {
		return err
	}
	out.NodeStatusUpdateFrequency = in.NodeStatusUpdateFrequency
	if err := Convert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration(&in.Webhook, &out.Webhook, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha1_CloudControllerManagerConfiguration_To_config_CloudControllerManagerConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_CloudControllerManagerConfiguration_To_config_CloudControllerManagerConfiguration(in *CloudControllerManagerConfiguration, out *config.CloudControllerManagerConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_CloudControllerManagerConfiguration_To_config_CloudControllerManagerConfiguration(in, out, s)
}

func autoConvert_config_CloudControllerManagerConfiguration_To_v1alpha1_CloudControllerManagerConfiguration(in *config.CloudControllerManagerConfiguration, out *CloudControllerManagerConfiguration, s conversion.Scope) error {
	if err := configv1alpha1.Convert_config_GenericControllerManagerConfiguration_To_v1alpha1_GenericControllerManagerConfiguration(&in.Generic, &out.Generic, s); err != nil {
		return err
	}
	if err := Convert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration(&in.KubeCloudShared, &out.KubeCloudShared, s); err != nil {
		return err
	}
	if err := nodeconfigv1alpha1.Convert_config_NodeControllerConfiguration_To_v1alpha1_NodeControllerConfiguration(&in.NodeController, &out.NodeController, s); err != nil {
		return err
	}
	if err := serviceconfigv1alpha1.Convert_config_ServiceControllerConfiguration_To_v1alpha1_ServiceControllerConfiguration(&in.ServiceController, &out.ServiceController, s); err != nil {
		return err
	}
	out.NodeStatusUpdateFrequency = in.NodeStatusUpdateFrequency
	if err := Convert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration(&in.Webhook, &out.Webhook, s); err != nil {
		return err
	}
	return nil
}

// Convert_config_CloudControllerManagerConfiguration_To_v1alpha1_CloudControllerManagerConfiguration is an autogenerated conversion function.
func Convert_config_CloudControllerManagerConfiguration_To_v1alpha1_CloudControllerManagerConfiguration(in *config.CloudControllerManagerConfiguration, out *CloudControllerManagerConfiguration, s conversion.Scope) error {
	return autoConvert_config_CloudControllerManagerConfiguration_To_v1alpha1_CloudControllerManagerConfiguration(in, out, s)
}

func autoConvert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration(in *CloudProviderConfiguration, out *config.CloudProviderConfiguration, s conversion.Scope) error {
	out.Name = in.Name
	out.CloudConfigFile = in.CloudConfigFile
	return nil
}

func autoConvert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration(in *config.CloudProviderConfiguration, out *CloudProviderConfiguration, s conversion.Scope) error {
	out.Name = in.Name
	out.CloudConfigFile = in.CloudConfigFile
	return nil
}

func autoConvert_v1alpha1_KubeCloudSharedConfiguration_To_config_KubeCloudSharedConfiguration(in *KubeCloudSharedConfiguration, out *config.KubeCloudSharedConfiguration, s conversion.Scope) error {
	if err := Convert_v1alpha1_CloudProviderConfiguration_To_config_CloudProviderConfiguration(&in.CloudProvider, &out.CloudProvider, s); err != nil {
		return err
	}
	out.ExternalCloudVolumePlugin = in.ExternalCloudVolumePlugin
	out.UseServiceAccountCredentials = in.UseServiceAccountCredentials
	out.AllowUntaggedCloud = in.AllowUntaggedCloud
	out.RouteReconciliationPeriod = in.RouteReconciliationPeriod
	out.NodeMonitorPeriod = in.NodeMonitorPeriod
	out.ClusterName = in.ClusterName
	out.ClusterCIDR = in.ClusterCIDR
	out.AllocateNodeCIDRs = in.AllocateNodeCIDRs
	out.CIDRAllocatorType = in.CIDRAllocatorType
	if err := v1.Convert_Pointer_bool_To_bool(&in.ConfigureCloudRoutes, &out.ConfigureCloudRoutes, s); err != nil {
		return err
	}
	out.NodeSyncPeriod = in.NodeSyncPeriod
	return nil
}

func autoConvert_config_KubeCloudSharedConfiguration_To_v1alpha1_KubeCloudSharedConfiguration(in *config.KubeCloudSharedConfiguration, out *KubeCloudSharedConfiguration, s conversion.Scope) error {
	if err := Convert_config_CloudProviderConfiguration_To_v1alpha1_CloudProviderConfiguration(&in.CloudProvider, &out.CloudProvider, s); err != nil {
		return err
	}
	out.ExternalCloudVolumePlugin = in.ExternalCloudVolumePlugin
	out.UseServiceAccountCredentials = in.UseServiceAccountCredentials
	out.AllowUntaggedCloud = in.AllowUntaggedCloud
	out.RouteReconciliationPeriod = in.RouteReconciliationPeriod
	out.NodeMonitorPeriod = in.NodeMonitorPeriod
	out.ClusterName = in.ClusterName
	out.ClusterCIDR = in.ClusterCIDR
	out.AllocateNodeCIDRs = in.AllocateNodeCIDRs
	out.CIDRAllocatorType = in.CIDRAllocatorType
	if err := v1.Convert_bool_To_Pointer_bool(&in.ConfigureCloudRoutes, &out.ConfigureCloudRoutes, s); err != nil {
		return err
	}
	out.NodeSyncPeriod = in.NodeSyncPeriod
	return nil
}

func autoConvert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration(in *WebhookConfiguration, out *config.WebhookConfiguration, s conversion.Scope) error {
	out.Webhooks = *(*[]string)(unsafe.Pointer(&in.Webhooks))
	return nil
}

// Convert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration(in *WebhookConfiguration, out *config.WebhookConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_WebhookConfiguration_To_config_WebhookConfiguration(in, out, s)
}

func autoConvert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration(in *config.WebhookConfiguration, out *WebhookConfiguration, s conversion.Scope) error {
	out.Webhooks = *(*[]string)(unsafe.Pointer(&in.Webhooks))
	return nil
}

// Convert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration is an autogenerated conversion function.
func Convert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration(in *config.WebhookConfiguration, out *WebhookConfiguration, s conversion.Scope) error {
	return autoConvert_config_WebhookConfiguration_To_v1alpha1_WebhookConfiguration(in, out, s)
}
