/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package statusmanager

import (
	"context"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-network-operator/pkg/controller/statusmanager"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	operatortypes "github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
)

var log = logf.Log.WithName("status_manager")

func SetAntreaInstallStatus(cl client.Client, conditionType configv1.ClusterStatusConditionType, status configv1.ConditionStatus, t time.Time, reason, message string) {
	antreaInstall := &operatorv1.AntreaInstall{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, antreaInstall)
	if err != nil {
		log.Error(err, "failed to get AntreaInstall")
	}
	antreaInstallPatch := client.MergeFrom(antreaInstall.DeepCopy())
	antreaInstall.Status.Conditions = []operatorv1.InstallCondition{
		{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: metav1.NewTime(t),
		},
	}
	if reason != "" {
		antreaInstall.Status.Conditions[0].Reason = reason
	}
	if message != "" {
		antreaInstall.Status.Conditions[0].Message = message
	}
	if err := cl.Status().Patch(context.TODO(), antreaInstall, antreaInstallPatch); err != nil {
		log.Error(err, "failed to set AntreaInstall")
	}
}

func SetAntreaInstallDegraded(cl client.Client, reason, message string) {
	SetAntreaInstallStatus(cl, configv1.OperatorDegraded, configv1.ConditionTrue, time.Now(), reason, message)
}

func SetAntreaInstallNotDegraded(cl client.Client) {
	SetAntreaInstallStatus(cl, configv1.OperatorDegraded, configv1.ConditionFalse, time.Now(), "", "")
}

func SetDegraded(cl client.Client, status *statusmanager.StatusManager, statusLevel statusmanager.StatusLevel, reason, message string) {
	// Set clusteroperator/antrea status
	status.SetDegraded(statusLevel, reason, message)
	// Set AntreaInstall CR status
	SetAntreaInstallDegraded(cl, reason, message)
}

func SetNotDegraded(cl client.Client, status *statusmanager.StatusManager, statusLevel statusmanager.StatusLevel) {
	// Set clusteroperator/antrea status
	status.SetNotDegraded(statusLevel)
	// Set AntreaInstall CR status
	SetAntreaInstallNotDegraded(cl)
}
