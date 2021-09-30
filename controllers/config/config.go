/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package config

import (
	"errors"
	"fmt"

	gocni "github.com/containerd/go-cni"
	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/network"
	"github.com/openshift/cluster-network-operator/pkg/render"
	"gopkg.in/yaml.v2"
	ctrl "sigs.k8s.io/controller-runtime"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
	"github.com/vmware/antrea-operator-for-kubernetes/version"
)

var log = ctrl.Log.WithName("config")

type Config interface {
	FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error
	ValidateConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error
	GenerateRenderData(operatorNetwork *ocoperv1.Network, operConfig *operatorv1.AntreaInstall) (*render.RenderData, error)
}

type ConfigOc struct{}

type ConfigK8s struct{}

func fillConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	antreaAgentConfig := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(operConfig.Spec.AntreaAgentConfig), &antreaAgentConfig)
	if err != nil {
		return fmt.Errorf("failed to parse AntreaAgentConfig: %v", err)
	}
	antreaControllerConfig := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(operConfig.Spec.AntreaControllerConfig), &antreaControllerConfig)
	if err != nil {
		return fmt.Errorf("failed to parse AntreaControllerConfig: %v", err)
	}
	// Set service CIDR.
	if clusterConfig == nil {
		if _, ok := antreaAgentConfig[types.ServiceCIDROption]; !ok {
			return errors.New("serviceCIDR should be specified on kubernetes.")
		}
		if nodeIPAM, ok := antreaControllerConfig["nodeIPAM"].(map[interface{}]interface{}); ok {
			enableNodeIPAM, ok := nodeIPAM["enableNodeIPAM"].(bool)
			if !ok {
				return errors.New("enableNodeIPAM should be bool.")
			}
			if enableNodeIPAM {
				if _, ok := antreaControllerConfig["clusterCIDRs"]; !ok {
					return errors.New("clusterCIDRs should be specified on kubernetes.")
				}
			}
		} else {
			return errors.New("Invalid nodeIPAM.")
		}
	} else {
		if serviceCIDR, ok := antreaAgentConfig[types.ServiceCIDROption].(string); !ok {
			antreaAgentConfig[types.ServiceCIDROption] = clusterConfig.Spec.ServiceNetwork[0]
		} else if found := inSlice(serviceCIDR, clusterConfig.Spec.ServiceNetwork); !found {
			log.Info("WARNING: option: %s is overwritten by cluster config")
			antreaAgentConfig[types.ServiceCIDROption] = clusterConfig.Spec.ServiceNetwork[0]
		}
		// render antrea-controller config on oc
		if nodeIPAM, ok := antreaControllerConfig["nodeIPAM"].(map[interface{}]interface{}); ok {
			enableNodeIPAM, ok := nodeIPAM["enableNodeIPAM"].(bool)
			if !ok {
				return errors.New("enableNodeIPAM should be bool.")
			}
			if enableNodeIPAM {
				if len(clusterConfig.Spec.ClusterNetwork) == 0 {
					return errors.New("clusterCIDR should be specified.")
				}
				nodeIPAM := make(map[string]interface{})
				clusterCIDRs := make([]string, len(clusterConfig.Spec.ClusterNetwork))
				for index, value := range clusterConfig.Spec.ClusterNetwork {
					clusterCIDRs[index] = value.CIDR
				}
				nodeIPAM["clusterCIDRs"] = clusterCIDRs
				nodeIPAM["serviceCIDR"] = clusterConfig.Spec.ServiceNetwork[0]
				nodeIPAM["nodeCIDRMaskSizeIPv4"] = clusterConfig.Spec.ClusterNetwork[0].HostPrefix
				antreaControllerConfig["nodeIPAM"] = nodeIPAM
			}
		} else {
			return errors.New("Invalid nodeIPAM.")
		}
	}
	// Set default MTU.
	_, ok := antreaAgentConfig[types.DefaultMTUOption]
	if !ok {
		antreaAgentConfig[types.DefaultMTUOption] = types.DefaultMTU
	}
	// Set Antrea image.
	if operConfig.Spec.AntreaImage == "" {
		operConfig.Spec.AntreaImage = types.DefaultAntreaImage
	}
	updatedAntreaAgentConfig, err := yaml.Marshal(antreaAgentConfig)
	if err != nil {
		return fmt.Errorf("failed to fill configurations in AntreaAgentConfig: %v", err)
	}
	updatedAntreaControllerConfig, err := yaml.Marshal(antreaControllerConfig)
	if err != nil {
		return fmt.Errorf("failed to fill configurations in AntreaControllerConfig: %v", err)
	}
	operConfig.Spec.AntreaAgentConfig = string(updatedAntreaAgentConfig)
	operConfig.Spec.AntreaControllerConfig = string(updatedAntreaControllerConfig)
	return nil
}

func (c *ConfigOc) FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return fillConfig(clusterConfig, operConfig)
}

func (c *ConfigK8s) FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return fillConfig(clusterConfig, operConfig)
}

func validateConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	var errs []error

	if operConfig.Spec.AntreaImage == "" {
		errs = append(errs, fmt.Errorf("antreaImage option can not be empty"))
	}

	antreaAgentConfig := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(operConfig.Spec.AntreaAgentConfig), &antreaAgentConfig)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to parse AntreaAgentConfig: %v", err))
		return fmt.Errorf("invalidate configuration: %v", errs)
	}

	if clusterConfig == nil {
		if _, ok := antreaAgentConfig[types.ServiceCIDROption]; !ok {
			errs = append(errs, fmt.Errorf("serviceCIDR option can not be empty"))
		}
	} else {
		if serviceCIDR, ok := antreaAgentConfig[types.ServiceCIDROption].(string); !ok {
			errs = append(errs, fmt.Errorf("serviceCIDR option can not be empty"))
		} else if found := inSlice(serviceCIDR, clusterConfig.Spec.ServiceNetwork); !found {
			errs = append(errs, fmt.Errorf("invalid serviceCIDR option: %s, available values are: %s", serviceCIDR, clusterConfig.Spec.ServiceNetwork))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalidate configuration: %v", errs)
	}
	return nil
}

func (c *ConfigOc) ValidateConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return validateConfig(clusterConfig, operConfig)
}

func (c *ConfigK8s) ValidateConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return validateConfig(clusterConfig, operConfig)
}

func NeedApplyChange(preConfig, curConfig *operatorv1.AntreaInstall) (agentNeedChange, controllerNeedChange, imageChange bool) {
	if preConfig == nil {
		return true, true, false
	}

	if preConfig.Spec.AntreaAgentConfig != curConfig.Spec.AntreaAgentConfig {
		agentNeedChange = true
	}
	if preConfig.Spec.AntreaCNIConfig != curConfig.Spec.AntreaCNIConfig {
		agentNeedChange = true
	}
	if preConfig.Spec.AntreaControllerConfig != curConfig.Spec.AntreaControllerConfig {
		controllerNeedChange = true
	}
	if preConfig.Spec.AntreaImage != curConfig.Spec.AntreaImage {
		agentNeedChange = true
		controllerNeedChange = true
		imageChange = true
	}
	return
}

func HasClusterNetworkConfigChange(preConfig, curConfig *configv1.Network) bool {
	// TODO: We may need to save the applied cluster network config in somewhere else. Thus operator can
	// retrieve the applied config on restart.
	if preConfig == nil {
		return true
	}
	if !stringSliceEqual(preConfig.Spec.ServiceNetwork, curConfig.Spec.ServiceNetwork) {
		return true
	}
	var preCIDRs, curCIDRs []string
	for _, clusterNet := range preConfig.Spec.ClusterNetwork {
		preCIDRs = append(preCIDRs, clusterNet.CIDR)
	}
	for _, clusterNet := range curConfig.Spec.ClusterNetwork {
		curCIDRs = append(curCIDRs, clusterNet.CIDR)
	}
	if !stringSliceEqual(preCIDRs, curCIDRs) {
		return true
	}
	return false
}

func HasDefaultMTUChange(preConfig, curConfig *operatorv1.AntreaInstall) (bool, int, error) {

	curAntreaAgentConfig := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(curConfig.Spec.AntreaAgentConfig), &curAntreaAgentConfig)
	if err != nil {
		return false, types.DefaultMTU, err
	}
	curDefaultMTU, ok := curAntreaAgentConfig[types.DefaultMTUOption]
	if !ok {
		return false, types.DefaultMTU, fmt.Errorf("%s option can not be empty", types.DefaultMTUOption)
	}

	if preConfig == nil {
		return true, curDefaultMTU.(int), nil
	}

	preAntreaAgentConfig := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(preConfig.Spec.AntreaAgentConfig), &preAntreaAgentConfig)
	if err != nil {
		return false, types.DefaultMTU, err
	}
	preDefaultMTU, ok := preAntreaAgentConfig[types.DefaultMTUOption]
	if !ok {
		return false, types.DefaultMTU, fmt.Errorf("%s option can not be empty", types.DefaultMTUOption)
	}

	return preDefaultMTU != curDefaultMTU, curDefaultMTU.(int), nil
}

func BuildNetworkStatus(clusterConfig *configv1.Network, defaultMTU int) *configv1.NetworkStatus {
	// Values extracted from spec are serviceNetwork and clusterNetworkCIDR.
	status := configv1.NetworkStatus{}
	for _, snet := range clusterConfig.Spec.ServiceNetwork {
		status.ServiceNetwork = append(status.ServiceNetwork, snet)
	}

	for _, cnet := range clusterConfig.Spec.ClusterNetwork {
		status.ClusterNetwork = append(status.ClusterNetwork,
			configv1.ClusterNetworkEntry{
				CIDR:       cnet.CIDR,
				HostPrefix: cnet.HostPrefix,
			})
	}
	status.NetworkType = clusterConfig.Spec.NetworkType
	status.ClusterNetworkMTU = defaultMTU
	return &status
}

func inSlice(str string, s []string) bool {
	for _, v := range s {
		if str == v {
			return true
		}
	}
	return false
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if !inSlice(v, b) {
			return false
		}
	}
	return true
}

// pluginCNIDir is the directory where plugins should install their CNI
// configuration file. By default, it is where multus looks, unless multus
// is deactivated
func pluginCNIConfDir(conf *ocoperv1.NetworkSpec) string {
	if conf.DisableMultiNetwork == nil || !*conf.DisableMultiNetwork {
		return network.MultusCNIConfDir
	}
	return network.SystemCNIConfDir
}

func generateRenderData(operatorNetwork *ocoperv1.Network, operConfig *operatorv1.AntreaInstall) *render.RenderData {
	renderData := render.MakeRenderData()
	renderData.Data[types.ReleaseVersion] = version.Version
	renderData.Data[types.AntreaAgentConfigRenderKey] = operConfig.Spec.AntreaAgentConfig
	renderData.Data[types.AntreaCNIConfigRenderKey] = operConfig.Spec.AntreaCNIConfig
	renderData.Data[types.AntreaControllerConfigRenderKey] = operConfig.Spec.AntreaControllerConfig
	renderData.Data[types.AntreaImageRenderKey] = operConfig.Spec.AntreaImage
	if operatorNetwork == nil {
		renderData.Data[types.CNIConfDirRenderKey] = gocni.DefaultNetDir
		renderData.Data[types.CNIBinDirRenderKey] = gocni.DefaultCNIDir
	} else {
		renderData.Data[types.CNIConfDirRenderKey] = pluginCNIConfDir(&operatorNetwork.Spec)
		renderData.Data[types.CNIBinDirRenderKey] = network.CNIBinDir
	}
	return &renderData
}

func (c *ConfigK8s) GenerateRenderData(operatorNetwork *ocoperv1.Network, operConfig *operatorv1.AntreaInstall) (*render.RenderData, error) {
	renderData := generateRenderData(operatorNetwork, operConfig)
	return renderData, nil
}

func (c *ConfigOc) GenerateRenderData(operatorNetwork *ocoperv1.Network, operConfig *operatorv1.AntreaInstall) (*render.RenderData, error) {
	renderData := generateRenderData(operatorNetwork, operConfig)
	return renderData, nil
}
