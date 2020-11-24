/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AntreaInstallSpec defines the desired state of AntreaInstall
type AntreaInstallSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// AntreaAgentConfig holds the configurations for antrea-agent.
	// +required
	AntreaAgentConfig string `json:"antreaAgentConfig"`

	// AntreaCNIConfig holds the configuration of CNI.
	// +required
	AntreaCNIConfig string `json:"antreaCNIConfig"`

	// AntreaControllerConfig holds the configurations for antrea-controller.
	// +required
	AntreaControllerConfig string `json:"antreaControllerConfig"`

	// AntreaImage is the Docker image name used by antrea-agent and antrea-controller.
	// +optional
	AntreaImage string `json:"antreaImage,omitempty"`
}

// AntreaInstallStatus defines the observed state of AntreaInstall
type AntreaInstallStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Conditions describes the state of Antrea installation.
	// +optional
	Conditions []InstallCondition `json:"conditions,omitempty"`
}

type InstallCondition = configv1.ClusterOperatorStatusCondition

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AntreaInstall is the Schema for the antreainstalls API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=antreainstalls,scope=Namespaced
type AntreaInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AntreaInstallSpec   `json:"spec,omitempty"`
	Status AntreaInstallStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AntreaInstallList contains a list of AntreaInstall
type AntreaInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AntreaInstall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AntreaInstall{}, &AntreaInstallList{})
}
