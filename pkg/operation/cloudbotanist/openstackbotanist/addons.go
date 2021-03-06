// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openstackbotanist

import (
	"github.com/gardener/gardener/pkg/operation/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployKube2IAMResources - Not needed on OpenStack
func (b *OpenStackBotanist) DeployKube2IAMResources() error {
	return nil
}

// DestroyKube2IAMResources - Not needed on OpenStack.
func (b *OpenStackBotanist) DestroyKube2IAMResources() error {
	return nil
}

// GenerateKube2IAMConfig - Not needed on OpenStack.
func (b *OpenStackBotanist) GenerateKube2IAMConfig() (map[string]interface{}, error) {
	return common.GenerateAddonConfig(nil, false), nil
}

// GenerateStorageClassesConfig generates values which are required to render the chart shoot-storageclasses properly.
func (b *OpenStackBotanist) GenerateStorageClassesConfig() (map[string]interface{}, error) {
	// Delete legacy storage class (named "default") as we can't update the parameters (this legacy class
	// did set `.parameters.type=default`).
	if err := b.K8sShootClient.Kubernetes().StorageV1().StorageClasses().Delete("default", &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	return map[string]interface{}{
		"StorageClasses": []map[string]interface{}{
			{
				"Name":           "default-class",
				"IsDefaultClass": true,
				"Provisioner":    "kubernetes.io/cinder",
				"Parameters": map[string]interface{}{
					"availability": b.Shoot.Info.Spec.Cloud.OpenStack.Zones[0],
				},
			},
		},
	}, nil
}

// GenerateNginxIngressConfig generates values which are required to render the chart nginx-ingress properly.
func (b *OpenStackBotanist) GenerateNginxIngressConfig() (map[string]interface{}, error) {
	return common.GenerateAddonConfig(nil, b.Shoot.NginxIngressEnabled()), nil
}

// GenerateVPNShootConfig generate cloud-specific vpn override - nothing unique for openstack
func (b *OpenStackBotanist) GenerateVPNShootConfig() (map[string]interface{}, error) {
	return nil, nil
}
