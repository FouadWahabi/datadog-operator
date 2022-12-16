// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodTemplateSpec(t *testing.T) {
	tests := []struct {
		name            string
		existingManager func() *fake.PodTemplateManagers
		override        v2alpha1.DatadogAgentComponentOverride
		validateManager func(t *testing.T, manager *fake.PodTemplateManagers)
	}{
		{
			name: "override service account name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.ServiceAccountName = "old-service-account"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ServiceAccountName: apiutils.NewStringPointer("new-service-account"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-service-account", manager.PodTemplateSpec().Spec.ServiceAccountName)
			},
		},
		{
			name: "01: given URI, override image name, tag, JMX",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "custom-agent",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/custom-agent:latest-jmx", actualImage(manager, t))
			},
		},
		{
			name: "02: given URI, override image name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "custom-agent",
					Tag:  "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/custom-agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "03: given URI, override image tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Tag: "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "04: given URI, override image with name:tag, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "agent:9.99.9",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "agent:9.99.9", actualImage(manager, t))
			},
		},
		{
			name: "05: given URI, override image name and JMX, retain tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "custom-agent",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/custom-agent:7.38.0-jmx", actualImage(manager, t))
			},
		},
		{
			name: "06: given URI, override image with JMX tag and flag, don't duplicate jmx suffix",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Tag:        "latest-jmx",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/agent:latest-jmx", actualImage(manager, t))
			},
		},
		{
			name: "07: given URI with JMX tag, override image with JMX false",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0-jmx", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					JMXEnabled: false,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/agent:7.38.0", actualImage(manager, t))
			},
		},
		{
			name: "08: given name:tag, override image with full URI name, ignore tag and JMX, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "someregistry.com/datadog/agent:9.99.9",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/agent:9.99.9", actualImage(manager, t))
			},
		},
		{
			name: "09: given name:tag, override image name, tag, JMX, sets default registry",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "agent",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "gcr.io/datadoghq/agent:latest-jmx", actualImage(manager, t))
			},
		},
		{
			name: "10: given name:tag, override image with name:tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "11: given URI, override image with repo name:tag, full name takes precedence",
			// related to 09 Name precedence.
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "repo/agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "repo/agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "12: given image URI, override with short URI, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "someregistry.com/agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "13: given short URI, override with name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "agent",
					Tag:  "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/agent:latest", actualImage(manager, t))
			},
		},
		{
			name: "14: given long URI, override with name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/a/b/c/agent:7.38.0", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "cluster-agent",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/a/b/c/cluster-agent:7.38.0-jmx", actualImage(manager, t))
			},
		},
		{
			name: "15: given long URI, override name with slash, overrides name in current image",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride("someregistry.com/datadog/agent:9.99", t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name: "otherregistry.com/agent",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "someregistry.com/datadog/otherregistry.com/agent:9.99", actualImage(manager, t))
			},
		},
		{
			name: "add envs",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.EnvVar().AddEnvVar(&v1.EnvVar{
					Name:  "existing-env",
					Value: "123",
				})

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Env: []v1.EnvVar{
					{
						Name:  "added-env",
						Value: "456",
					},
					{
						Name: "added-env-valuefrom",
						ValueFrom: &v1.EnvVarSource{
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: common.FieldPathStatusPodIP,
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedEnvs := []*v1.EnvVar{
					{
						Name:  "existing-env",
						Value: "123",
					},
					{
						Name:  "added-env",
						Value: "456",
					},
					{
						Name: "added-env-valuefrom",
						ValueFrom: &v1.EnvVarSource{
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: common.FieldPathStatusPodIP,
							},
						},
					},
				}

				for _, envs := range manager.EnvVarMgr.EnvVarsByC {
					assert.Equal(t, expectedEnvs, envs)
				}
			},
		},
		{
			// Note: this test is for the node agent (hardcoded in t.Run).
			name: "add custom configs",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
					v2alpha1.AgentGeneralConfigFile: {
						ConfigMap: &commonv1.ConfigMapConfig{
							Name: "custom-config",
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == getDefaultConfigMapName("datadog-agent", string(v2alpha1.AgentGeneralConfigFile)) {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override confd with configMap",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraConfd: &v2alpha1.MultiCustomConfig{
					ConfigMap: &commonv1.ConfigMapConfig{
						Name: "extra-confd",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ConfdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override confd with configData",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraConfd: &v2alpha1.MultiCustomConfig{
					ConfigDataMap: map[string]string{
						"path_to_file.yaml": "yaml: data",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ConfdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override checksd with configMap",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraChecksd: &v2alpha1.MultiCustomConfig{
					ConfigMap: &commonv1.ConfigMapConfig{
						Name: "extra-checksd",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ChecksdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override checksd with configData",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraChecksd: &v2alpha1.MultiCustomConfig{
					ConfigDataMap: map[string]string{
						"path_to_file.py": "print('hello')",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ChecksdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			// This test is pretty simple because "container_test.go" already tests overriding containers
			name: "override containers",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.EnvVarMgr.AddEnvVarToContainer(
					commonv1.ClusterAgentContainerName,
					&v1.EnvVar{
						Name:  common.DDLogLevel,
						Value: "info",
					},
				)

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Containers: map[commonv1.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					commonv1.ClusterAgentContainerName: {
						LogLevel: apiutils.NewStringPointer("trace"),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envSet := false

				for _, env := range manager.EnvVarMgr.EnvVarsByC[commonv1.ClusterAgentContainerName] {
					if env.Name == common.DDLogLevel && env.Value == "trace" {
						envSet = true
						break
					}
				}

				assert.True(t, envSet)
			},
		},
		{
			name: "add volumes",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.Volume().AddVolume(&v1.Volume{
					Name: "existing-volume",
				})

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Volumes: []v1.Volume{
					{
						Name: "added-volume",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedVolumes := []*v1.Volume{
					{
						Name: "existing-volume",
					},
					{
						Name: "added-volume",
					},
				}

				assert.Equal(t, expectedVolumes, manager.VolumeMgr.Volumes)
			},
		},
		{
			name: "override security context",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.SecurityContext = &v1.PodSecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(1234),
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				SecurityContext: &v1.PodSecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(5678),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, int64(5678), *manager.PodTemplateSpec().Spec.SecurityContext.RunAsUser)
			},
		},
		{
			name: "override priority class name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.PriorityClassName = "old-name"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				PriorityClassName: apiutils.NewStringPointer("new-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-name", manager.PodTemplateSpec().Spec.PriorityClassName)
			},
		},
		{
			name: "override affinity",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.Affinity = &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 50,
								PodAffinityTerm: v1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"old-label": "123",
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 50,
								PodAffinityTerm: v1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"new-label": "456", // Changed
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t,
					map[string]string{"new-label": "456"},
					manager.PodTemplateSpec().Spec.Affinity.PodAntiAffinity.
						PreferredDuringSchedulingIgnoredDuringExecution[0].
						PodAffinityTerm.LabelSelector.MatchLabels)
			},
		},
		{
			name: "add labels",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Labels = map[string]string{
					"existing-label": "123",
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Labels: map[string]string{
					"existing-label": "456",
					"new-label":      "789",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedLabels := map[string]string{
					"existing-label": "456",
					"new-label":      "789",
				}

				assert.Equal(t, expectedLabels, manager.PodTemplateSpec().Labels)
			},
		},
		{
			name: "override host network",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.HostNetwork = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostNetwork: apiutils.NewBoolPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostNetwork)
			},
		},
		{
			name: "override host PID",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.HostPID = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostPID: apiutils.NewBoolPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostPID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := test.existingManager()

			PodTemplateSpec(manager, &test.override, v2alpha1.NodeAgentComponentName, "datadog-agent")

			test.validateManager(t, manager)
		})
	}
}

// In practice, image string registry will be derived either from global.registry setting or the default.
func fakePodTemplateManagersWithImageOverride(image string, t *testing.T) *fake.PodTemplateManagers {
	manager := fake.NewPodTemplateManagers(t)
	manager.PodTemplateSpec().Spec.InitContainers = []v1.Container{
		{
			Image: image,
		},
	}
	manager.PodTemplateSpec().Spec.Containers = []v1.Container{
		{
			Image: image,
		},
		{
			Image: image,
		},
	}
	return manager
}

// Assert all images are same and return the image
func actualImage(manager *fake.PodTemplateManagers, t *testing.T) string {
	allContainers := append(
		manager.PodTemplateSpec().Spec.Containers, manager.PodTemplateSpec().Spec.InitContainers...,
	)

	image := allContainers[0].Image
	for _, container := range allContainers {
		assert.Equal(t, image, container.Image)
	}
	return image
}
