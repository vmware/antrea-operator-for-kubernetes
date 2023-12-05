/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package config

import (
	"errors"
	"fmt"

	"k8s.io/utils/net"

	ctlconfig "antrea.io/antrea/pkg/config/controller"
	"antrea.io/antrea/pkg/features"
	gocni "github.com/containerd/go-cni"
	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/network"
	"github.com/openshift/cluster-network-operator/pkg/render"
	"gopkg.in/yaml.v2"
	ctrl "sigs.k8s.io/controller-runtime"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
	"github.com/vmware/antrea-operator-for-kubernetes/internal/version"
)

var log = ctrl.Log.WithName("config")

type Config interface {
	FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error
	ValidateConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error
	GenerateRenderData(operatorNetwork *ocoperv1.Network, operConfig *operatorv1.AntreaInstall) (*render.RenderData, error)
}

type ConfigOc struct{}

type ConfigK8s struct{}

func fillAgentConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	antreaAgentConfig := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(operConfig.Spec.AntreaAgentConfig), &antreaAgentConfig)
	if err != nil {
		return fmt.Errorf("failed to parse AntreaAgentConfig: %v", err)
	}
	// Set service CIDR.
	if clusterConfig == nil {
		if _, ok := antreaAgentConfig[types.ServiceCIDROption]; !ok {
			return errors.New("serviceCIDR should be specified on kubernetes.")
		}
	} else {
		if serviceCIDR, ok := antreaAgentConfig[types.ServiceCIDROption].(string); !ok {
			antreaAgentConfig[types.ServiceCIDROption] = clusterConfig.Spec.ServiceNetwork[0]
		} else if found := inSlice(serviceCIDR, clusterConfig.Spec.ServiceNetwork); !found {
			log.Info("WARNING: ServiceCIDROption is overwritten by cluster config")
			antreaAgentConfig[types.ServiceCIDROption] = clusterConfig.Spec.ServiceNetwork[0]
		}
	}
	// Set default MTU.
	_, ok := antreaAgentConfig[types.DefaultMTUOption]
	if !ok {
		antreaAgentConfig[types.DefaultMTUOption] = types.DefaultMTU
	}
	updatedAntreaAgentConfig, err := yaml.Marshal(antreaAgentConfig)
	if err != nil {
		return fmt.Errorf("failed to fill configurations in AntreaAgentConfig: %v", err)
	}
	operConfig.Spec.AntreaAgentConfig = string(updatedAntreaAgentConfig)
	return nil
}

func fillControllerConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	var controllerConfig ctlconfig.ControllerConfig
	err := yaml.Unmarshal([]byte(operConfig.Spec.AntreaControllerConfig), &controllerConfig)
	if err != nil {
		return fmt.Errorf("failed to parse AntreaControllerConfig: %v", err)
	}

	//Turn on NodeIPAM featureGate.
	if controllerConfig.FeatureGates == nil {
		controllerConfig.FeatureGates = make(map[string]bool)
	}
	controllerConfig.FeatureGates[string(features.NodeIPAM)] = true
	controllerConfig.NodeIPAM.EnableNodeIPAM = true

	if clusterConfig != nil {
		ip4found := false
		ip6found := false

		for _, cidr := range clusterConfig.Spec.ClusterNetwork {
			if net.IsIPv4CIDRString(cidr.CIDR) {
				if !ip4found {
					ip4found = true
					controllerConfig.NodeIPAM.ClusterCIDRs = append(controllerConfig.NodeIPAM.ClusterCIDRs, cidr.CIDR)
					controllerConfig.NodeIPAM.NodeCIDRMaskSizeIPv4 = int(cidr.HostPrefix)
				}
			} else {
				if !ip6found {
					ip6found = true
					controllerConfig.NodeIPAM.ClusterCIDRs = append(controllerConfig.NodeIPAM.ClusterCIDRs, cidr.CIDR)
					controllerConfig.NodeIPAM.NodeCIDRMaskSizeIPv6 = int(cidr.HostPrefix)
				}
			}
		}

		// Set service CIDR
		ip4found = false
		ip6found = false

		for _, svcCIDR := range clusterConfig.Spec.ServiceNetwork {
			if net.IsIPv4CIDRString(svcCIDR) {
				if !ip4found {
					ip4found = true
					controllerConfig.NodeIPAM.ServiceCIDR = svcCIDR
				}
			} else {
				if !ip6found {
					ip6found = true
					controllerConfig.NodeIPAM.ServiceCIDRv6 = svcCIDR
				}
			}
		}
	}

	updatedAntreaControllerConfig, err := yaml.Marshal(controllerConfig)
	if err != nil {
		return fmt.Errorf("failed to fill configurations in AntreaControllerConfig: %v", err)
	}
	operConfig.Spec.AntreaControllerConfig = string(updatedAntreaControllerConfig)
	return nil
}

func fillConfig(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall, isOpenShift bool) error {
	err := fillAgentConfig(clusterConfig, operConfig)
	if err != nil {
		return err
	}

	if isOpenShift {
		err = fillControllerConfig(clusterConfig, operConfig)
		if err != nil {
			return err
		}
	}

	// Set Antrea image.
	if operConfig.Spec.AntreaImage == "" {
		operConfig.Spec.AntreaImage = types.DefaultAntreaImage
	}

	return nil
}

func (c *ConfigOc) FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return fillConfig(clusterConfig, operConfig, true)
}

func (c *ConfigK8s) FillConfigs(clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall) error {
	return fillConfig(clusterConfig, operConfig, false)
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
	renderData.Data[types.ReleaseVersion] = version.GetVersion()
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
