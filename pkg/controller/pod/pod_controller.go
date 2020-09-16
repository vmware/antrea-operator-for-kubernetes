/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/cluster-network-operator/pkg/apply"
	"github.com/openshift/cluster-network-operator/pkg/controller/statusmanager"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ruicao93/antrea-operator/pkg/controller/sharedinfo"
)

var log = logf.Log.WithName("controller_pod")

// The periodic resync interval.
// We will re-run the reconciliation logic, even if the NCP configuration
// hasn't changed.
var ResyncPeriod = 2 * time.Minute

// Add creates a new Pod Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, status *statusmanager.StatusManager, sharedInfo *sharedinfo.SharedInfo) error {
	return add(mgr, newReconciler(mgr, status, sharedInfo))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, status *statusmanager.StatusManager, sharedInfo *sharedinfo.SharedInfo) reconcile.Reconciler {
	return &ReconcilePod{client: mgr.GetClient(), scheme: mgr.GetScheme(), status: status, sharedInfo: sharedInfo}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("pod-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Pod
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Pod
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePod implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePod{}

// ReconcilePod reconciles a Pod object
type ReconcilePod struct {
	client     client.Client
	scheme     *runtime.Scheme
	status     *statusmanager.StatusManager
	sharedInfo *sharedinfo.SharedInfo
}

// Reconcile updates the ClusterOperator.Status to match the current state of the watched Deployments/DaemonSets
func (r *ReconcilePod) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling pod update")

	if !r.isAntreaResource(&request) {
		return reconcile.Result{}, nil
	}
	r.status.SetFromPods()

	if err := r.recreateResourceIfNotExist(&request); err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{RequeueAfter: ResyncPeriod}, nil
}

func (r *ReconcilePod) isAntreaResource(request *reconcile.Request) bool {
	if r.sharedInfo.AntreaAgentDaemonSetSpec != nil {
		if request.Name == r.sharedInfo.AntreaAgentDaemonSetSpec.GetName() && request.Namespace == r.sharedInfo.AntreaAgentDaemonSetSpec.GetNamespace() {
			return true
		}
	}
	if r.sharedInfo.AntreaControllerDeploymentSpec != nil {
		if request.Name == r.sharedInfo.AntreaControllerDeploymentSpec.GetName() && request.Namespace == r.sharedInfo.AntreaControllerDeploymentSpec.GetNamespace() {
			return true
		}
	}
	return false
}

func (r *ReconcilePod) recreateResourceIfNotExist(request *reconcile.Request) error {
	r.sharedInfo.Lock()
	defer r.sharedInfo.Unlock()
	var curObject runtime.Object
	var objectSpec *uns.Unstructured
	if request.Name == r.sharedInfo.AntreaAgentDaemonSetSpec.GetName() && request.Namespace == r.sharedInfo.AntreaAgentDaemonSetSpec.GetNamespace() {
		curObject = &appsv1.DaemonSet{}
		objectSpec = r.sharedInfo.AntreaAgentDaemonSetSpec.DeepCopy()
	} else {
		curObject = &appsv1.Deployment{}
		objectSpec = r.sharedInfo.AntreaControllerDeploymentSpec.DeepCopy()
	}
	err := r.client.Get(context.TODO(), request.NamespacedName, curObject)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("K8s resource - '%s' dose not exist", request.Name))
		} else {
			log.Error(err, fmt.Sprintf("Could not retrieve K8s resource - '%s'", request.Name))
			r.status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", fmt.Sprintf("Failed to apply objects: %v", err))
			return err
		}
	} else {
		log.Info(fmt.Sprintf("K8s resource - '%s' already exists", request.Name))
		return nil
	}
	if err = apply.ApplyObject(context.TODO(), r.client, objectSpec); err != nil {
		log.Error(
			err, fmt.Sprintf("could not apply (%s) %s/%s",
				objectSpec.GroupVersionKind(), objectSpec.GetNamespace(), objectSpec.GetName()))
		r.status.SetDegraded(
			statusmanager.OperatorConfig, "ApplyOperatorConfig",
			fmt.Sprintf("Failed to apply operator configuration: %v", err))
		return err
	}
	log.Info(fmt.Sprintf("Recreated K8s resource: %s", request.Name))
	return nil
}
