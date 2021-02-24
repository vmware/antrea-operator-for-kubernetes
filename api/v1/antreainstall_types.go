/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AntreaInstallSpec defines the desired state of AntreaInstall
type AntreaInstallSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// AntreaAgentConfig holds the configurations for antrea-agent.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +required
	AntreaAgentConfig string `json:"antreaAgentConfig"`

	// AntreaCNIConfig holds the configuration of CNI.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +required
	AntreaCNIConfig string `json:"antreaCNIConfig"`

	// AntreaControllerConfig holds the configurations for antrea-controller.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +required
	AntreaControllerConfig string `json:"antreaControllerConfig"`

	// AntreaPlatform is the platform on which antrea will be deployed.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +required
	AntreaPlatform string `json:"antreaPlatform"`

	// AntreaImage is the Docker image name used by antrea-agent and antrea-controller.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	AntreaImage string `json:"antreaImage,omitempty"`
}

// AntreaInstallStatus defines the observed state of AntreaInstall
type AntreaInstallStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions describes the state of Antrea installation.
	// +operator-sdk:csv:customresourcedefinitions:type=status
	// +optional
	Conditions []InstallCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:generate=false
type InstallCondition = configv1.ClusterOperatorStatusCondition

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AntreaInstall is the Schema for the antreainstalls API
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,"A Kubernetes Deployment for the Operator"},{AntreaInstall,v1,"this operator's CR"},{ClusterOperator,v1,"antrea cluster operator"},{Network,v1,"Openshift's cluster network"}}
type AntreaInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AntreaInstallSpec   `json:"spec,omitempty"`
	Status AntreaInstallStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AntreaInstallList contains a list of AntreaInstall
type AntreaInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AntreaInstall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AntreaInstall{}, &AntreaInstallList{})
}
