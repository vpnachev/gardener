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

package awsbotanist

import (
	"fmt"
	"path/filepath"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	"github.com/gardener/gardener/pkg/operation/common"

	awsv1alpha1 "github.com/gardener/gardener-extensions/controllers/provider-aws/pkg/apis/aws/v1alpha1"
)

const cloudProviderConfigTemplate = `
[Global]
VPC=%q
SubnetID=%q
DisableSecurityGroupIngress=true
KubernetesClusterTag=%q
KubernetesClusterID=%q
Zone=%q
`

// GenerateCloudProviderConfig generates the AWS cloud provider config.
// See this for more details:
// https://github.com/kubernetes/kubernetes/blob/master/pkg/cloudprovider/providers/aws/aws.go
func (b *AWSBotanist) GenerateCloudProviderConfig() (string, error) {
	// This code will only exist temporarily until we have introduced the `ControlPlane` extension. Gardener
	// will no longer compute the cloud-provider-config but instead the provider specific controller will be
	// responsible.
	if b.Shoot.InfrastructureStatus == nil {
		return "", fmt.Errorf("no infrastructure status found")
	}
	infrastructureStatus, err := infrastructureStatusFromInfrastructure(b.Shoot.InfrastructureStatus)
	if err != nil {
		return "", err
	}
	publicSubnet, err := findSubnetByPurpose(infrastructureStatus.VPC.Subnets, awsv1alpha1.PurposePublic)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		cloudProviderConfigTemplate,
		infrastructureStatus.VPC.ID,
		publicSubnet.ID,
		b.Shoot.SeedNamespace,
		b.Shoot.SeedNamespace,
		b.Shoot.Info.Spec.Cloud.AWS.Zones[0],
	), nil
}

// RefreshCloudProviderConfig refreshes the cloud provider credentials in the existing cloud
// provider config.
// Not needed on AWS (cloud provider config does not contain the credentials), hence, the
// original is returned back.
func (b *AWSBotanist) RefreshCloudProviderConfig(currentConfig map[string]string) map[string]string {
	return currentConfig
}

// GenerateKubeAPIServerServiceConfig generates the cloud provider specific values which are required to render the
// Service manifest of the kube-apiserver-service properly.
func (b *AWSBotanist) GenerateKubeAPIServerServiceConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"annotations": map[string]interface{}{
			"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout":         "3600",
			"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                "ssl",
			"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":                       "443",
			"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "5",
			"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "30",
			"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
			"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
			"service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy":          "ELBSecurityPolicy-TLS-1-2-2017-01",
		},
	}, nil
}

// GenerateKubeAPIServerExposeConfig defines the cloud provider specific values which configure how the kube-apiserver
// is exposed to the public.
func (b *AWSBotanist) GenerateKubeAPIServerExposeConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"endpointReconcilerType": "none",
	}, nil
}

// GenerateKubeAPIServerConfig generates the cloud provider specific values which are required to render the
// Deployment manifest of the kube-apiserver properly.
func (b *AWSBotanist) GenerateKubeAPIServerConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"environment": getAWSCredentialsEnvironment(),
	}, nil
}

// GenerateCloudControllerManagerConfig generates the cloud provider specific values which are required to
// render the Deployment manifest of the cloud-controller-manager properly.
func (b *AWSBotanist) GenerateCloudControllerManagerConfig() (map[string]interface{}, string, error) {
	return map[string]interface{}{
		"configureRoutes": false,
		"environment":     getAWSCredentialsEnvironment(),
	}, common.CloudControllerManagerDeploymentName, nil
}

// GenerateKubeControllerManagerConfig generates the cloud provider specific values which are required to
// render the Deployment manifest of the kube-controller-manager properly.
func (b *AWSBotanist) GenerateKubeControllerManagerConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"environment": getAWSCredentialsEnvironment(),
	}, nil
}

// GenerateKubeSchedulerConfig generates the cloud provider specific values which are required to render the
// Deployment manifest of the kube-scheduler properly.
func (b *AWSBotanist) GenerateKubeSchedulerConfig() (map[string]interface{}, error) {
	return nil, nil
}

// GenerateCSIConfig generates the configuration for CSI charts
func (b *AWSBotanist) GenerateCSIConfig() (map[string]interface{}, error) {
	return nil, nil
}

// maps are mutable, so it's safer to create a new instance
func getAWSCredentialsEnvironment() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name": "AWS_ACCESS_KEY_ID",
			"valueFrom": map[string]interface{}{
				"secretKeyRef": map[string]interface{}{
					"key":  AccessKeyID,
					"name": gardencorev1alpha1.SecretNameCloudProvider,
				},
			},
		},
		{
			"name": "AWS_SECRET_ACCESS_KEY",
			"valueFrom": map[string]interface{}{
				"secretKeyRef": map[string]interface{}{
					"key":  SecretAccessKey,
					"name": gardencorev1alpha1.SecretNameCloudProvider,
				},
			},
		},
	}
}

// GenerateETCDStorageClassConfig generates values which are required to create etcd volume storageclass properly.
func (b *AWSBotanist) GenerateETCDStorageClassConfig() map[string]interface{} {
	return map[string]interface{}{
		"name":        "gardener.cloud-fast",
		"capacity":    b.Seed.GetValidVolumeSize("80Gi"),
		"provisioner": "kubernetes.io/aws-ebs",
		"parameters": map[string]interface{}{
			"type": "gp2",
		},
	}
}

// GenerateEtcdBackupConfig returns the etcd backup configuration for the etcd Helm chart.
func (b *AWSBotanist) GenerateEtcdBackupConfig() (map[string][]byte, map[string]interface{}, error) {
	bucketName := "bucketName"

	tf, err := b.NewBackupInfrastructureTerraformer()
	if err != nil {
		return nil, nil, err
	}
	stateVariables, err := tf.GetStateOutputVariables(bucketName)
	if err != nil {
		return nil, nil, err
	}

	secretData := map[string][]byte{
		common.BackupBucketName: []byte(stateVariables[bucketName]),
		Region:                  []byte(b.Seed.Info.Spec.Cloud.Region),
		AccessKeyID:             b.Seed.Secret.Data[AccessKeyID],
		SecretAccessKey:         b.Seed.Secret.Data[SecretAccessKey],
	}

	backupConfigData := map[string]interface{}{
		"schedule":         b.Operation.ShootBackup.Schedule,
		"storageProvider":  "S3",
		"storageContainer": stateVariables[bucketName],
		"backupSecret":     common.BackupSecretName,
		"env": []map[string]interface{}{
			{
				"name": "AWS_REGION",
				"valueFrom": map[string]interface{}{
					"secretKeyRef": map[string]interface{}{
						"name": common.BackupSecretName,
						"key":  Region,
					},
				},
			},
			{
				"name": "AWS_SECRET_ACCESS_KEY",
				"valueFrom": map[string]interface{}{
					"secretKeyRef": map[string]interface{}{
						"name": common.BackupSecretName,
						"key":  SecretAccessKey,
					},
				},
			},
			{
				"name": "AWS_ACCESS_KEY_ID",
				"valueFrom": map[string]interface{}{
					"secretKeyRef": map[string]interface{}{
						"name": common.BackupSecretName,
						"key":  AccessKeyID,
					},
				},
			},
		},
		"volumeMount": []map[string]interface{}{},
	}

	return secretData, backupConfigData, nil
}

// DeployCloudSpecificControlPlane updates the AWS ELB health check to SSL and deploys the aws-lb-readvertiser.
// https://github.com/gardener/aws-lb-readvertiser
func (b *AWSBotanist) DeployCloudSpecificControlPlane() error {
	var (
		name          = "aws-lb-readvertiser"
		defaultValues = map[string]interface{}{
			"domain":   b.APIServerAddress,
			"replicas": b.Shoot.GetReplicas(1),
			"podAnnotations": map[string]interface{}{
				"checksum/secret-aws-lb-readvertiser": b.CheckSums[name],
			},
		}
	)

	values, err := b.InjectSeedShootImages(defaultValues, common.AWSLBReadvertiserImageName)
	if err != nil {
		return err
	}

	return b.ApplyChartSeed(filepath.Join(common.ChartPath, "seed-controlplane", "charts", name), b.Shoot.SeedNamespace, name, nil, values)
}
