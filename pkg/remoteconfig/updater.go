// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

const (
	defaultSite  = "datadoghq.com"
	pollInterval = 10 * time.Second
)

type RemoteConfigUpdater struct {
	kubeClient  kubeclient.Client
	rcClient    *client.Client
	rcService   *service.Service
	serviceConf RcServiceConfiguration
	logger      logr.Logger
}

type RcServiceConfiguration struct {
	cfg               model.Config
	apiKey            string
	site              string
	baseRawURL        string
	hostname          string
	clusterName       string
	telemetryReporter service.RcTelemetryReporter
	agentVersion      string
	rcDatabaseDir     string
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	Features *FeaturesConfig `json:"features"`
}

type FeaturesConfig struct {
	CWS *FeatureEnabledConfig `json:"CWS"`
}

type FeatureEnabledConfig struct {
	Enabled *bool `json:"enabled"`
}

// TODO replace
type dummyRcTelemetryReporter struct{}

func (d dummyRcTelemetryReporter) IncRateLimit() {}
func (d dummyRcTelemetryReporter) IncTimeout()   {}

func (r *RemoteConfigUpdater) Setup(dda *datadoghqv2alpha1.DatadogAgent) error {
	// Get API Key from DatadogAgent
	apiKey, err := r.getAPIKeyFromDatadogAgent(dda)
	if err != nil {
		return err
	}

	// Extract needed configs from the DatadogAgent
	var site string
	if dda.Spec.Global.Site != nil && *dda.Spec.Global.Site != "" {
		site = *dda.Spec.Global.Site
	}

	var clusterName string
	if dda.Spec.Global.ClusterName != nil && *dda.Spec.Global.ClusterName != "" {
		clusterName = *dda.Spec.Global.ClusterName
	}

	var rcDirectorRoot string
	if dda.Spec.Features.RemoteConfiguration.DirectorRoot != nil && *dda.Spec.Features.RemoteConfiguration.DirectorRoot != "" {
		rcDirectorRoot = *dda.Spec.Features.RemoteConfiguration.DirectorRoot
	}

	var rcConfigRoot string
	if dda.Spec.Features.RemoteConfiguration.ConfigRoot != nil && *dda.Spec.Features.RemoteConfiguration.ConfigRoot != "" {
		rcConfigRoot = *dda.Spec.Features.RemoteConfiguration.ConfigRoot
	}

	if r.rcClient == nil && r.rcService == nil {
		// If rcClient && rcService not setup yet
		err = r.setup(err, apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot)
		if err != nil {
			return err
		}
	} else if apiKey != r.serviceConf.apiKey || site != r.serviceConf.site || clusterName != r.serviceConf.clusterName {
		// If one of configs has been updated
		err := r.Stop()
		if err != nil {
			return err
		}
		err = r.setup(err, apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RemoteConfigUpdater) setup(err error, apiKey string, site string, clusterName string, rcDirectorRoot string, rcConfigRoot string) error {
	// Fill in rc service configuration
	err = r.configureService(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot)
	if err != nil {
		r.logger.Error(err, "Failed to configure Remote Configuration service")
		return err
	}

	rcService, err := service.NewService(
		r.serviceConf.cfg,
		r.serviceConf.apiKey,
		r.serviceConf.baseRawURL,
		r.serviceConf.hostname,
		[]string{fmt.Sprintf("cluster_name:%s", r.serviceConf.clusterName)},
		r.serviceConf.telemetryReporter,
		r.serviceConf.agentVersion,
		service.WithDatabaseFileName(filepath.Join(r.serviceConf.rcDatabaseDir, fmt.Sprintf("remote-config-%s.db", uuid.New()))))

	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration service")
		return err
	}
	r.rcService = rcService

	rcClient, err := client.NewClient(
		rcService,
		client.WithAgent("datadog-operator", "9.9.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithDirectorRootOverride(r.serviceConf.cfg.GetString("remote_configuration.director_root")),
		client.WithPollInterval(10*time.Second))
	if err != nil {
		r.logger.Error(err, "Failed to create Remote Configuration client")
		return err
	}
	r.rcClient = rcClient

	rcService.Start()
	r.logger.Info("rcService started")

	rcClient.Start()
	r.logger.Info("rcClient started")

	rcClient.Subscribe(state.ProductAgentConfig, r.agentConfigUpdateCallback)
	return nil
}

func (r *RemoteConfigUpdater) getAPIKeyFromDatadogAgent(dda *datadoghqv2alpha1.DatadogAgent) (string, error) {
	var err error
	apiKey := ""

	isSet, secretName, secretKeyName := v2alpha1.GetAPIKeySecret(dda.Spec.Global.Credentials, v2alpha1.GetDefaultCredentialsSecretName(dda))
	if isSet {
		return *dda.Spec.Global.Credentials.APIKey, nil
	}
	apiKey, err = r.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
	if err != nil {
		return "", err
	}

	return apiKey, nil
}

func (r *RemoteConfigUpdater) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := r.kubeClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

func (r *RemoteConfigUpdater) Stop() error {
	if r.rcService != nil {
		err := r.rcService.Stop()
		if err != nil {
			return err
		}
	}
	if r.rcClient != nil {
		r.rcClient.Close()
	}
	r.rcService = nil
	r.rcClient = nil
	return nil
}

func (r *RemoteConfigUpdater) agentConfigUpdateCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {

	ctx := context.Background()

	r.logger.Info("agentConfigUpdateCallback is called")
	r.logger.Info("Received", "update", update)

	// ---------- Section to use when mocking config ----------
	// Comment out this section when testing remote config updates

	// mockFeatureConfig := `{"features":{"cws":{"enabled":true}}}` //`{"some":"json"}`

	// mockMetadata := state.Metadata{
	// 	Product:   "testProduct",
	// 	ID:        "testID",
	// 	Name:      "testName",
	// 	Version:   9,
	// 	RawLength: 20,
	// }
	// mockRawConfig := state.RawConfig{
	// 	Config:   []byte(mockFeatureConfig),
	// 	Metadata: mockMetadata,
	// }
	// var mockUpdate = make(map[string]state.RawConfig)
	// mockUpdate["testConfigPath"] = mockRawConfig

	// r.logger.Info(string(mockUpdate["testConfigPath"].Config))

	// update = mockUpdate
	// ---------- End section to use when mocking config ----------

	// TODO
	// For now, only single default config path is present (key of update[key])
	tempstring := ""
	for k := range update {
		tempstring += k
	}

	r.logger.Info(tempstring)

	applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})

	if len(update) == 0 {
		return
	}

	var cfg DatadogAgentRemoteConfig
	for _, update := range update {
		r.logger.Info("Content", "update.Config", string(update.Config))
		if err := json.Unmarshal(update.Config, &cfg); err != nil {
			r.logger.Error(err, "failed to marshal config", "updateMetadata.ID", update.Metadata.ID)
			return
		}
	}

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "failed to get updatable agents")
	}
	serializedSpec, err := json.Marshal(dda.Spec)
	if err != nil {
		r.logger.Error(err, "failed to marshal DatadogAgent spec")
	}
	r.logger.Info("datadog agent spec", "spec", string(serializedSpec))

	if err := r.applyConfig(ctx, dda, cfg); err != nil {
		r.logger.Error(err, "failed to apply config")
		applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	r.logger.Info("successfully applied config!")

	applyStateCallback(tempstring, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})

}

func (r *RemoteConfigUpdater) getDatadogAgentInstance(ctx context.Context) (v2alpha1.DatadogAgent, error) {
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.kubeClient.List(context.TODO(), ddaList); err != nil {
		return v2alpha1.DatadogAgent{}, fmt.Errorf("unable to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return v2alpha1.DatadogAgent{}, errors.New("cannot find any DatadogAgent")
	}

	// Return first DatadogAgent as only one is supported
	return ddaList.Items[0], nil
}

func (r *RemoteConfigUpdater) applyConfig(ctx context.Context, dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {
	if err := r.updateInstance(dda, cfg); err != nil {
		return err
	}

	return nil
}

func (r *RemoteConfigUpdater) updateInstance(dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {

	if cfg.Features == nil {
		return nil
	}

	if cfg.Features.CWS == nil {
		return nil
	}

	if cfg.Features.CWS.Enabled == nil {
		return nil
	}

	newdda := dda.DeepCopy()

	if newdda.Spec.Features == nil {
		newdda.Spec.Features = &v2alpha1.DatadogFeatures{}
	}

	if newdda.Spec.Features.CWS == nil {
		newdda.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}

	if newdda.Spec.Features.CWS.Enabled == nil {
		newdda.Spec.Features.CWS.Enabled = new(bool)
	}

	newdda.Spec.Features.CWS.Enabled = cfg.Features.CWS.Enabled

	if !apiutils.IsEqualStruct(dda.Spec, newdda.Spec) {
		return r.kubeClient.Update(context.TODO(), newdda)
	}

	return nil
}

// configureService fills the configuration needed to start the rc service
func (r *RemoteConfigUpdater) configureService(apiKey, site, clusterName, rcDirectorRoot, rcConfigRoot string) error {
	// Create config required for RC service
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.SetWithoutSource("api_key", apiKey)
	// TODO For the moment still binding the config root and director root from the operator env variable
	cfg.SetWithoutSource("remote_configuration.config_root", rcConfigRoot)
	cfg.SetWithoutSource("remote_configuration.director_root", rcDirectorRoot)

	hostname, _ := os.Hostname()
	baseRawURL := getRemoteConfigEndpoint(site, "https://config.")

	// TODO change to a different dir
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	// TODO decide what to put for version, since NewService is expecting agentVersion (even "1.50.0" for operator doesn't work)
	serviceConf := RcServiceConfiguration{
		cfg:               cfg,
		apiKey:            apiKey,
		site:              site,
		baseRawURL:        baseRawURL,
		hostname:          hostname,
		clusterName:       clusterName,
		telemetryReporter: dummyRcTelemetryReporter{},
		agentVersion:      "7.50.0",
		// agentVersion:  version.Version,
		rcDatabaseDir: baseDir,
	}
	r.serviceConf = serviceConf
	return nil
}

// getRemoteConfigEndpoint returns the main DD URL defined in the config, based on `site` and the prefix
func getRemoteConfigEndpoint(site, prefix string) string {
	if site != "" {
		return prefix + strings.TrimSpace(site)
	}
	return prefix + defaultSite
}

func NewRemoteConfigUpdater(client kubeclient.Client, logger logr.Logger) *RemoteConfigUpdater {
	return &RemoteConfigUpdater{
		kubeClient: client,
		logger:     logger.WithName("remote-config"),
	}
}
