/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"

	"github.com/openshift/cluster-network-operator/pkg/apply"

	"github.com/vmware/antrea-operator-for-kubernetes/controllers/sharedinfo"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/statusmanager"
)

// The periodic resync interval.
// We will re-run the reconciliation logic, even if the NCP configuration
// hasn't changed.
var ResyncPeriod = 2 * time.Minute

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	Client     client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Status     *statusmanager.StatusManager
	SharedInfo *sharedinfo.SharedInfo
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.DaemonSet{}).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

// Reconcile updates the ClusterOperator.Status to match the current state of the watched Deployments/DaemonSets
func (r *PodReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling pod update")

	if !r.isAntreaResource(&request) {
		return reconcile.Result{}, nil
	}
	r.Status.SetFromPods()

	if err := r.recreateResourceIfNotExist(&request); err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{RequeueAfter: ResyncPeriod}, nil
}

func (r *PodReconciler) isAntreaResource(request *reconcile.Request) bool {
	if r.SharedInfo.AntreaAgentDaemonSetSpec != nil {
		if request.Name == r.SharedInfo.AntreaAgentDaemonSetSpec.GetName() && request.Namespace == r.SharedInfo.AntreaAgentDaemonSetSpec.GetNamespace() {
			return true
		}
	}
	if r.SharedInfo.AntreaControllerDeploymentSpec != nil {
		if request.Name == r.SharedInfo.AntreaControllerDeploymentSpec.GetName() && request.Namespace == r.SharedInfo.AntreaControllerDeploymentSpec.GetNamespace() {
			return true
		}
	}
	return false
}

func (r *PodReconciler) recreateResourceIfNotExist(request *reconcile.Request) error {
	r.SharedInfo.Lock()
	defer r.SharedInfo.Unlock()
	var curObject client.Object
	var objectSpec *uns.Unstructured
	if request.Name == r.SharedInfo.AntreaAgentDaemonSetSpec.GetName() && request.Namespace == r.SharedInfo.AntreaAgentDaemonSetSpec.GetNamespace() {
		curObject = &appsv1.DaemonSet{}
		objectSpec = r.SharedInfo.AntreaAgentDaemonSetSpec.DeepCopy()
	} else {
		curObject = &appsv1.Deployment{}
		objectSpec = r.SharedInfo.AntreaControllerDeploymentSpec.DeepCopy()
	}
	err := r.Client.Get(context.TODO(), request.NamespacedName, curObject)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("K8s resource - '%s' dose not exist", request.Name))
		} else {
			r.Log.Error(err, fmt.Sprintf("Could not retrieve K8s resource - '%s'", request.Name))
			r.Status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", fmt.Sprintf("Failed to apply objects: %v", err))
			return err
		}
	} else {
		r.Log.Info(fmt.Sprintf("K8s resource - '%s' already exists", request.Name))
		return nil
	}
	if err = apply.ApplyObject(context.TODO(), r.Client, objectSpec); err != nil {
		r.Log.Error(
			err, fmt.Sprintf("could not apply (%s) %s/%s",
				objectSpec.GroupVersionKind(), objectSpec.GetNamespace(), objectSpec.GetName()))
		r.Status.SetDegraded(
			statusmanager.OperatorConfig, "ApplyOperatorConfig",
			fmt.Sprintf("Failed to apply operator configuration: %v", err))
		return err
	}
	r.Log.Info(fmt.Sprintf("Recreated K8s resource: %s", request.Name))
	return nil
}
