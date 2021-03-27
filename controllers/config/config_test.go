/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package config

import (
	"fmt"
	"testing"

	gocni "github.com/containerd/go-cni"
	"github.com/ghodss/yaml"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/network"
	"github.com/openshift/cluster-network-operator/pkg/render"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	operatortypes "github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
	"github.com/vmware/antrea-operator-for-kubernetes/version"
)

var mockClusterConfig = configv1.Network{
	Spec: configv1.NetworkSpec{
		ServiceNetwork: []string{"10.96.0.0/12"},
		ClusterNetwork: []configv1.ClusterNetworkEntry{
			{
				CIDR:       "192.168.0.0/16",
				HostPrefix: 24,
			},
		},
		NetworkType: "antrea",
	},
}

var MockDisableMultiNetwork = false

var mockOperatorNetwork = ocoperv1.Network{
	Spec: ocoperv1.NetworkSpec{
		DisableMultiNetwork: &MockDisableMultiNetwork,
	},
}

var mockOperConfig = operatorv1.AntreaInstall{
	Spec: operatorv1.AntreaInstallSpec{
		AntreaAgentConfig: `{
			"serviceCIDR": "10.96.0.0/12"
		}`,
		AntreaCNIConfig: `{
			"cniVersion":"0.3.0",
			"name": "antrea",
			"plugins": [
				{
					"type": "antrea",
					"ipam": {
						"type": "host-local"
					}
				},
				{
					"type": "portmap",
					"capabilities": {"portMappings": true}
				}
			]
		}`,
		AntreaControllerConfig: "apiPort: 10349",
	},
}

var oc = &ConfigOc{}
var k8s = &ConfigK8s{}

func TestFillDefaultsOc(t *testing.T) {
	g := NewGomegaWithT(t)

	clusterConfig := mockClusterConfig.DeepCopy()
	operConfig := mockOperConfig.DeepCopy()
	err := oc.FillConfigs(clusterConfig, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())

	antreaAgentConfig := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(operConfig.Spec.AntreaAgentConfig), &antreaAgentConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(antreaAgentConfig[operatortypes.ServiceCIDROption]).Should(Equal(clusterConfig.Spec.ServiceNetwork[0]))
	g.Expect(int(antreaAgentConfig[operatortypes.DefaultMTUOption].(float64))).Should(Equal(operatortypes.DefaultMTU))
	g.Expect(operConfig.Spec.AntreaImage).Should(Equal(operatortypes.DefaultAntreaImage))
}

func TestFillDefaultsK8s(t *testing.T) {
	g := NewGomegaWithT(t)

	operConfig := mockOperConfig.DeepCopy()
	err := k8s.FillConfigs(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())

	antreaAgentConfig := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(operConfig.Spec.AntreaAgentConfig), &antreaAgentConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(int(antreaAgentConfig[operatortypes.DefaultMTUOption].(float64))).Should(Equal(operatortypes.DefaultMTU))
	g.Expect(operConfig.Spec.AntreaImage).Should(Equal(operatortypes.DefaultAntreaImage))
}

func TestValidateConfigOc(t *testing.T) {
	g := NewGomegaWithT(t)

	clusterConfig := mockClusterConfig.DeepCopy()
	operConfig := mockOperConfig.DeepCopy()
	err := oc.FillConfigs(clusterConfig, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	err = oc.ValidateConfig(clusterConfig, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Validate service CIDR
	clusterConfig = mockClusterConfig.DeepCopy()
	operConfig = mockOperConfig.DeepCopy()
	clusterConfig.Spec.ServiceNetwork = []string{"10.96.0.0.0/12"}
	operConfig.Spec.AntreaAgentConfig = "serviceCIDR: 10.96.0.0.1/12"
	err = oc.ValidateConfig(clusterConfig, operConfig)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("invalid serviceCIDR option"))
	g.Expect(err.Error()).Should(ContainSubstring("available values are"))

	// Validate antrea-agent config
	clusterConfig = mockClusterConfig.DeepCopy()
	operConfig = mockOperConfig.DeepCopy()
	operConfig.Spec.AntreaAgentConfig = `serviceCIDR:---`
	err = oc.ValidateConfig(clusterConfig, operConfig)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("failed to parse AntreaAgentConfig"))
}

func TestValidateConfigK8s(t *testing.T) {
	g := NewGomegaWithT(t)

	operConfig := mockOperConfig.DeepCopy()
	err := k8s.FillConfigs(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	err = k8s.ValidateConfig(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Validate service CIDR and antrea image
	operConfig = mockOperConfig.DeepCopy()
	err = k8s.ValidateConfig(nil, operConfig)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("antreaImage option can not be empty"))

	// Validate antrea-agent config
	operConfig = mockOperConfig.DeepCopy()
	operConfig.Spec.AntreaAgentConfig = `serviceCIDR:---`
	err = k8s.ValidateConfig(nil, operConfig)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("failed to parse AntreaAgentConfig"))
}

func TestRenderOc(t *testing.T) {
	g := NewGomegaWithT(t)

	clusterConfig := mockClusterConfig.DeepCopy()
	operConfig := mockOperConfig.DeepCopy()
	operatorNetwork := mockOperatorNetwork.DeepCopy()
	err := oc.FillConfigs(clusterConfig, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	renderData, err := oc.GenerateRenderData(operatorNetwork, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	objs, err := render.RenderDir("../../antrea-manifest", renderData)
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, obj := range objs {
		if obj.GetKind() == "ConfigMap" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-config" {
			var data map[string]interface{}
			data = obj.Object["data"].(map[string]interface{})
			g.Expect(data[operatortypes.AntreaAgentConfigOption]).Should(Equal(operConfig.Spec.AntreaAgentConfig))
			g.Expect(data[operatortypes.AntreaCNIConfigOption]).Should(Equal(operConfig.Spec.AntreaCNIConfig))
			g.Expect(data[operatortypes.AntreaControllerConfigOption]).Should(Equal(operConfig.Spec.AntreaControllerConfig))
		} else if obj.GetKind() == "Deployment" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-controller" {
			antreaDeployment := &appsv1.Deployment{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), antreaDeployment)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(antreaDeployment.Spec.Template.Spec.Containers[0].Image).Should(Equal(operatortypes.DefaultAntreaImage))
			g.Expect(antreaDeployment.Annotations["release.openshift.io/version"]).Should(Equal(version.Version))
		} else if obj.GetKind() == "DaemonSet" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-agent" {
			antreaDaemonSet := &appsv1.DaemonSet{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), antreaDaemonSet)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(antreaDaemonSet.Annotations["release.openshift.io/version"]).Should(Equal(version.Version))
		}
	}
}

func TestRenderK8s(t *testing.T) {
	g := NewGomegaWithT(t)

	operConfig := mockOperConfig.DeepCopy()
	err := k8s.FillConfigs(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	renderData, err := k8s.GenerateRenderData(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	objs, err := render.RenderDir("../../antrea-manifest", renderData)
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, obj := range objs {
		if obj.GetKind() == "ConfigMap" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-config" {
			var data map[string]interface{}
			data = obj.Object["data"].(map[string]interface{})
			g.Expect(data[operatortypes.AntreaAgentConfigOption]).Should(Equal(operConfig.Spec.AntreaAgentConfig))
			g.Expect(data[operatortypes.AntreaCNIConfigOption]).Should(Equal(operConfig.Spec.AntreaCNIConfig))
			g.Expect(data[operatortypes.AntreaControllerConfigOption]).Should(Equal(operConfig.Spec.AntreaControllerConfig))
		} else if obj.GetKind() == "Deployment" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-controller" {
			antreaDeployment := &appsv1.Deployment{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), antreaDeployment)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(antreaDeployment.Spec.Template.Spec.Containers[0].Image).Should(Equal(operatortypes.DefaultAntreaImage))
			g.Expect(antreaDeployment.Annotations["release.openshift.io/version"]).Should(Equal(version.Version))
		} else if obj.GetKind() == "DaemonSet" && obj.GetNamespace() == "kube-system" && obj.GetName() == "antrea-agent" {
			antreaDaemonSet := &appsv1.DaemonSet{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), antreaDaemonSet)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(antreaDaemonSet.Annotations["release.openshift.io/version"]).Should(Equal(version.Version))
		}
	}
}

func TestHasClusterNetworkConfigChange(t *testing.T) {
	g := NewGomegaWithT(t)

	preConfig := mockClusterConfig.DeepCopy()
	curConfig := mockClusterConfig.DeepCopy()
	changed := HasClusterNetworkConfigChange(preConfig, curConfig)
	g.Expect(changed).Should(Equal(false))

	// Service network change.
	curConfig.Spec.ServiceNetwork = []string{"10.97.0.0/12"}
	changed = HasClusterNetworkConfigChange(preConfig, curConfig)
	g.Expect(changed).Should(Equal(true))

	curConfig = mockClusterConfig.DeepCopy()
	curConfig.Spec.ServiceNetwork = append(curConfig.Spec.ServiceNetwork, "10.97.0.0/12")
	changed = HasClusterNetworkConfigChange(preConfig, curConfig)
	g.Expect(changed).Should(Equal(true))

	// Cluster network change.
	curConfig = mockClusterConfig.DeepCopy()
	curConfig.Spec.ClusterNetwork = []configv1.ClusterNetworkEntry{
		{
			CIDR:       "192.169.0.0/16",
			HostPrefix: 24,
		},
	}
	changed = HasClusterNetworkConfigChange(preConfig, curConfig)
	g.Expect(changed).Should(Equal(true))

	curConfig = mockClusterConfig.DeepCopy()
	curConfig.Spec.ClusterNetwork = append(curConfig.Spec.ClusterNetwork, configv1.ClusterNetworkEntry{
		CIDR:       "192.169.0.0/16",
		HostPrefix: 24,
	})
	changed = HasClusterNetworkConfigChange(preConfig, curConfig)
	g.Expect(changed).Should(Equal(true))
}

func TestHasDefaultMTUChange(t *testing.T) {
	g := NewGomegaWithT(t)

	testPreMtu := 1500
	preConfig := mockOperConfig.DeepCopy()
	preConfig.Spec.AntreaAgentConfig = fmt.Sprintf("%s: %d", operatortypes.DefaultMTUOption, testPreMtu)
	curConfig := preConfig.DeepCopy()
	defaultMTUChanged, curDefaultMTU, err := HasDefaultMTUChange(preConfig, curConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(defaultMTUChanged).Should(Equal(false))
	g.Expect(curDefaultMTU).Should(Equal(testPreMtu))

	testCurMtu := 1600
	curConfig.Spec.AntreaAgentConfig = fmt.Sprintf("%s: %d", operatortypes.DefaultMTUOption, testCurMtu)
	defaultMTUChanged, curDefaultMTU, err = HasDefaultMTUChange(preConfig, curConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(defaultMTUChanged).Should(Equal(true))
	g.Expect(curDefaultMTU).Should(Equal(testCurMtu))

	preConfig = mockOperConfig.DeepCopy()
	curConfig = mockOperConfig.DeepCopy()
	_, _, err = HasDefaultMTUChange(preConfig, curConfig)
	g.Expect(err).Should(HaveOccurred())
}

func TestBuildNetworkStatus(t *testing.T) {
	g := NewGomegaWithT(t)

	defaultMtu := 1500
	clusterConfig := mockClusterConfig.DeepCopy()
	updatedClusterConfig := BuildNetworkStatus(clusterConfig, defaultMtu)

	comparedClusterConfig := mockClusterConfig.DeepCopy()
	comparedClusterConfig.Spec.ServiceNetwork = updatedClusterConfig.ServiceNetwork
	comparedClusterConfig.Spec.ClusterNetwork = updatedClusterConfig.ClusterNetwork
	g.Expect(HasClusterNetworkConfigChange(clusterConfig, comparedClusterConfig)).Should(Equal(false))
	g.Expect(updatedClusterConfig.NetworkType).Should(Equal(clusterConfig.Spec.NetworkType))
	g.Expect(updatedClusterConfig.ClusterNetworkMTU).Should(Equal(defaultMtu))
}

func TestGenerateRenderDataOc(t *testing.T) {
	g := NewGomegaWithT(t)

	operConfig := mockOperConfig.DeepCopy()
	operConfig.Spec.AntreaImage = operatortypes.DefaultAntreaImage
	operatorNetwork := mockOperatorNetwork.DeepCopy()
	renderData, err := oc.GenerateRenderData(operatorNetwork, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(renderData.Data[operatortypes.AntreaAgentConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaAgentConfig))
	g.Expect(renderData.Data[operatortypes.AntreaCNIConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaCNIConfig))
	g.Expect(renderData.Data[operatortypes.AntreaControllerConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaControllerConfig))
	g.Expect(renderData.Data[operatortypes.AntreaImageRenderKey]).Should(Equal(operConfig.Spec.AntreaImage))
	g.Expect(renderData.Data[operatortypes.CNIConfDirRenderKey]).Should(Equal(network.MultusCNIConfDir))
	g.Expect(renderData.Data[operatortypes.CNIBinDirRenderKey]).Should(Equal(network.CNIBinDir))

	operatorNetwork.Spec.DisableMultiNetwork = nil
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(renderData.Data[operatortypes.CNIConfDirRenderKey]).Should(Equal(network.MultusCNIConfDir))

	disableMultiNetwork := true
	operatorNetwork.Spec.DisableMultiNetwork = &disableMultiNetwork
	renderData, err = oc.GenerateRenderData(operatorNetwork, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(renderData.Data[operatortypes.CNIConfDirRenderKey]).Should(Equal(network.SystemCNIConfDir))
}

func TestGenerateRenderDataK8s(t *testing.T) {
	g := NewGomegaWithT(t)

	operConfig := mockOperConfig.DeepCopy()
	operConfig.Spec.AntreaImage = operatortypes.DefaultAntreaImage
	renderData, err := k8s.GenerateRenderData(nil, operConfig)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(renderData.Data[operatortypes.AntreaAgentConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaAgentConfig))
	g.Expect(renderData.Data[operatortypes.AntreaCNIConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaCNIConfig))
	g.Expect(renderData.Data[operatortypes.AntreaControllerConfigRenderKey]).Should(Equal(operConfig.Spec.AntreaControllerConfig))
	g.Expect(renderData.Data[operatortypes.AntreaImageRenderKey]).Should(Equal(operConfig.Spec.AntreaImage))
	g.Expect(renderData.Data[operatortypes.CNIConfDirRenderKey]).Should(Equal(gocni.DefaultNetDir))
	g.Expect(renderData.Data[operatortypes.CNIBinDirRenderKey]).Should(Equal(gocni.DefaultCNIDir))
}
