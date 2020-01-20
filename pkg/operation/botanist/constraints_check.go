// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package botanist

import (
	"context"
	"fmt"
	"strings"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/webhook"
	genericwebhook "k8s.io/apiserver/pkg/admission/plugin/webhook/generic"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/mutating"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/validating"
)

func shootHibernatedConstraint(condition gardencorev1beta1.Condition) gardencorev1beta1.Condition {
	return gardencorev1beta1helper.UpdatedCondition(condition, gardencorev1beta1.ConditionTrue, "ConstraintNotChecked", "Shoot cluster has been hibernated.")
}

// ConstraintsChecks conducts the constraints checks on all the given constraints.
func (b *Botanist) ConstraintsChecks(ctx context.Context, initializeShootClients func() error, hibernation gardencorev1beta1.Condition) gardencorev1beta1.Condition {
	hibernationPossible := b.constraintsChecks(ctx, initializeShootClients, hibernation)
	return b.pardonCondition(hibernationPossible)
}

func (b *Botanist) constraintsChecks(ctx context.Context, initializeShootClients func() error, hibernationConstraint gardencorev1beta1.Condition) gardencorev1beta1.Condition {
	if b.Shoot.HibernationEnabled || b.Shoot.Info.Status.IsHibernated {
		return shootHibernatedConstraint(hibernationConstraint)
	}

	if err := initializeShootClients(); err != nil {
		message := fmt.Sprintf("Could not initialize Shoot client for constraints check: %+v", err)
		b.Logger.Error(message)
		hibernationConstraint = gardencorev1beta1helper.UpdatedConditionUnknownErrorMessage(hibernationConstraint, message)

		return hibernationConstraint
	}

	newHibernationConstraint, err := b.CheckHibernationPossible(ctx, hibernationConstraint)
	hibernationConstraint = newConditionOrError(hibernationConstraint, newHibernationConstraint, err)

	return hibernationConstraint
}

// CheckHibernationPossible checks the Shoot for problematic webhooks which could prevent wakeup after hibernation
func (b *Botanist) CheckHibernationPossible(ctx context.Context, constraint gardencorev1beta1.Condition) (*gardencorev1beta1.Condition, error) {
	validatingWebhookConfigs := &admissionregistrationv1beta1.ValidatingWebhookConfigurationList{}
	if err := b.K8sShootClient.Client().List(ctx, validatingWebhookConfigs); err != nil {
		return nil, fmt.Errorf("could not get ValidatingWebhookConfigurations of Shoot cluster to check if Shoot can be hibernated")
	}

	for _, webhookConfig := range validatingWebhookConfigs.Items {
		for _, w := range webhookConfig.Webhooks {
			validatingPlugin, err := validating.NewValidatingAdmissionWebhook(strings.NewReader(w.ClientConfig.String()))
			if err != nil {
				return nil, fmt.Errorf("Cannot initializer validating webhook err: %v", err)
			}
			uid := fmt.Sprintf("%s/%s", webhookConfig.Name, w.Name)
			accessor := webhook.NewValidatingWebhookAccessor(uid, webhookConfig.Name, &w)
			if IsProblematicWebhook(accessor, validatingPlugin.Webhook) {
				c := gardencorev1beta1helper.UpdatedCondition(
					constraint,
					gardencorev1beta1.ConditionFalse,
					"ProblematicWebhooks",
					fmt.Sprintf(
						"Shoot cannot be hibernated because of ValidatingWebhookConfiguration %q: webhook %q will probably prevent the Shoot from being woken up again",
						webhookConfig.Name,
						w.Name))
				return &c, nil
			}
		}
	}

	mutatingWebhookConfigs := &admissionregistrationv1beta1.MutatingWebhookConfigurationList{}
	if err := b.K8sShootClient.Client().List(ctx, mutatingWebhookConfigs); err != nil {
		return nil, fmt.Errorf("could not get MutatingWebhookConfigurations of Shoot cluster to check if Shoot can be hibernated")
	}

	for _, webhookConfig := range mutatingWebhookConfigs.Items {
		for _, w := range webhookConfig.Webhooks {
			mutatingPlugin, err := mutating.NewMutatingWebhook(strings.NewReader(w.ClientConfig.String()))
			if err != nil {
				return nil, fmt.Errorf("Cannot initializer mutating webhook err: %v", err)
			}
			uid := fmt.Sprintf("%s/%s", webhookConfig.Name, w.Name)
			accessor := webhook.NewMutatingWebhookAccessor(uid, webhookConfig.Name, &w)
			if IsProblematicWebhook(accessor, mutatingPlugin.Webhook) {
				c := gardencorev1beta1helper.UpdatedCondition(
					constraint,
					gardencorev1beta1.ConditionFalse,
					"ProblematicWebhooks",
					fmt.Sprintf(
						"Shoot cannot be hibernated because of MutatingWebhookConfiguration %q: webhook %q  will probably prevent the Shoot from being woken up again",
						webhookConfig.Name,
						w.Name))
				return &c, nil
			}
		}
	}

	c := gardencorev1beta1helper.UpdatedCondition(constraint, gardencorev1beta1.ConditionTrue, "NoProblematicWebhooks", "Shoot can be hibernated.")
	return &c, nil
}

// IsProblematicWebhook checks if a single webhook of the Shoot Cluster is problematic and the Shoot should therefore
// not be hibernated. Problematic webhooks are webhooks with rules affecting CREATE|UPDATE if pods|deployments|daemonsts|nodes and
// failurePolicy=Fail. If the Shoot contains such a webhook, we can never wake up this shoot cluster again
// as new nodes cannot get created/ready, or our system component pods cannot get created/ready
// (because the webhook's backing pod is not yet running).
func IsProblematicWebhook(accessor webhook.WebhookAccessor, webhook *genericwebhook.Webhook) bool {
	failurePolicy := accessor.GetFailurePolicy()
	if failurePolicy == nil || *failurePolicy == admissionregistrationv1beta1.Ignore {
		return false
	}

	objectMeta := metav1.ObjectMeta{
		Labels: map[string]string{
			"origin": "gardener",
		},
	}
	po := corev1.Pod{
		ObjectMeta: objectMeta,
	}
	deploy := appsv1.Deployment{
		ObjectMeta: objectMeta,
	}
	ds := appsv1.DaemonSet{
		ObjectMeta: objectMeta,
	}
	nodes := corev1.Node{}
	interfaces := &admission.RuntimeObjectInterfaces{EquivalentResourceMapper: nil}

	createPods := admission.NewAttributesRecord(&po, nil, schema.GroupVersionKind{"", "v1", "Pod"}, "kube-system", "name", schema.GroupVersionResource{"", "v1", "pods"}, "", admission.Create, &metav1.CreateOptions{}, false, nil)
	updatePods := admission.NewAttributesRecord(&po, nil, schema.GroupVersionKind{"", "v1", "Pod"}, "kube-system", "name", schema.GroupVersionResource{"", "v1", "pods"}, "", admission.Update, &metav1.UpdateOptions{}, false, nil)

	createDeploys := admission.NewAttributesRecord(&deploy, nil, schema.GroupVersionKind{"apps", "v1", "Deployment"}, "kube-system", "name", schema.GroupVersionResource{"apps", "v1", "deployments"}, "", admission.Create, &metav1.CreateOptions{}, false, nil)
	updateDeploys := admission.NewAttributesRecord(&deploy, nil, schema.GroupVersionKind{"apps", "v1", "Deployment"}, "kube-system", "name", schema.GroupVersionResource{"apps", "v1", "deployments"}, "", admission.Update, &metav1.UpdateOptions{}, false, nil)

	createDSs := admission.NewAttributesRecord(&ds, nil, schema.GroupVersionKind{"apps", "v1", "DaemonSet"}, "kube-system", "name", schema.GroupVersionResource{"apps", "v1", "daemonsets"}, "", admission.Create, &metav1.CreateOptions{}, false, nil)
	updateDSs := admission.NewAttributesRecord(&ds, nil, schema.GroupVersionKind{"apps", "v1", "DaemonSet"}, "kube-system", "name", schema.GroupVersionResource{"apps", "v1", "daemonsets"}, "", admission.Update, &metav1.UpdateOptions{}, false, nil)

	createNodes := admission.NewAttributesRecord(&nodes, nil, schema.GroupVersionKind{"", "v1", "Node"}, "kube-system", "name", schema.GroupVersionResource{"", "v1", "nodes"}, "", admission.Create, &metav1.CreateOptions{}, false, nil)
	updateNodes := admission.NewAttributesRecord(&nodes, nil, schema.GroupVersionKind{"", "v1", "Node"}, "kube-system", "name", schema.GroupVersionResource{"", "v1", "nodes"}, "", admission.Update, &metav1.UpdateOptions{}, false, nil)

	invocation, apiErrors := webhook.ShouldCallHook(accessor, createPods, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}
	invocation, apiErrors = webhook.ShouldCallHook(accessor, updatePods, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}

	invocation, apiErrors = webhook.ShouldCallHook(accessor, createDeploys, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}
	invocation, apiErrors = webhook.ShouldCallHook(accessor, updateDeploys, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}

	invocation, apiErrors = webhook.ShouldCallHook(accessor, createDSs, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}
	invocation, apiErrors = webhook.ShouldCallHook(accessor, updateDSs, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}

	invocation, apiErrors = webhook.ShouldCallHook(accessor, createNodes, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}
	invocation, apiErrors = webhook.ShouldCallHook(accessor, updateNodes, interfaces)
	if apiErrors != nil || invocation != nil {
		return true
	}

	return false
}
