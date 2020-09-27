/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package config

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/apply"
	"github.com/openshift/cluster-network-operator/pkg/controller/statusmanager"
	"github.com/openshift/cluster-network-operator/pkg/render"
	k8sutil "github.com/openshift/cluster-network-operator/pkg/util/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1 "github.com/ruicao93/antrea-operator/pkg/apis/operator/v1"
	"github.com/ruicao93/antrea-operator/pkg/controller/sharedinfo"
	operatortypes "github.com/ruicao93/antrea-operator/pkg/types"
)

var log = logf.Log.WithName("controller_config")

// Add creates a new ConfigMap Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, status *statusmanager.StatusManager, sharedInfo *sharedinfo.SharedInfo) error {
	return add(mgr, newReconciler(mgr, status, sharedInfo))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, status *statusmanager.StatusManager, sharedInfo *sharedinfo.SharedInfo) reconcile.Reconciler {
	return &ReconcileConfig{client: mgr.GetClient(), scheme: mgr.GetScheme(), status: status, sharedInfo: sharedInfo, mapper: mgr.GetRESTMapper()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("config-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AntreaInstall CRD
	err = c.Watch(&source.Kind{Type: &operatorv1.AntreaInstall{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Network CRD
	err = c.Watch(&source.Kind{Type: &configv1.Network{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileConfig{}

// ReconcileConfig reconciles cluster network configuration changes.
type ReconcileConfig struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	status     *statusmanager.StatusManager
	mapper     meta.RESTMapper
	sharedInfo *sharedinfo.SharedInfo

	appliedClusterConfig *configv1.Network
	appliedOperConfig    *operatorv1.AntreaInstall
}

// Reconcile propagates changes from the cluster config and operater config to
// antrea config. And then update antrea resources if antrea config changes.
func (r *ReconcileConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	if request.Namespace == "" && request.Name == operatortypes.ClusterConfigName {
		reqLogger.Info("Reconciling antrea-operator Cluster Network CR change")
	} else if request.Namespace == operatortypes.OperatorNameSpace && request.Name == operatortypes.OperatorConfigName {
		reqLogger.Info("Reconciling antrea-operator antrea-install CR change")
	} else {
		return reconcile.Result{}, nil
	}

	// Fetch Cluster Network CR.
	clusterConfig := &configv1.Network{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: operatortypes.ClusterConfigName}, clusterConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := "Cluster Network CR not found"
			log.Info(msg)
			r.status.SetDegraded(statusmanager.ClusterConfig, "NoClusterConfig", msg)
			return reconcile.Result{}, nil
		}
		r.status.SetDegraded(statusmanager.ClusterConfig, "InvalidClusterConfig",
			fmt.Sprintf("Failed to get cluster network CRD: %v", err))
		log.Error(err, "failed to get Cluster Network CR")
		return reconcile.Result{Requeue: true}, err
	}

	// Fetch the Network.operator.openshift.io instance
	operatorNetwork := &ocoperv1.Network{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: operatortypes.ClusterOperatorNetworkName}, operatorNetwork)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.status.SetDegraded(statusmanager.OperatorConfig, "NoClusterNetworkOperatorConfig",
				fmt.Sprintf("Cluster network operator configuration not found"))
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Unable to retrieve Network.operator.openshift.io object")
		return reconcile.Result{Requeue: true}, err
	}

	// Fetch antrea-install CR.
	operConfig := &operatorv1.AntreaInstall{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, operConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s CR not found", operatortypes.OperatorConfigName)
			log.Info(msg)
			r.status.SetDegraded(statusmanager.ClusterConfig, "NoAntreaInstallCR", msg)
			return reconcile.Result{}, nil
		}
		log.Error(err, "failed to get antrea-install CR")
		r.status.SetDegraded(statusmanager.OperatorConfig, "InvalidAntreaInstallCR",
			fmt.Sprintf("Failed to get operator CR: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Fill default configurations.
	if err = FillConfigs(clusterConfig, operConfig); err != nil {
		log.Error(err, "failed to fill configurations")
		r.status.SetDegraded(statusmanager.OperatorConfig, "FillConfigurationsError",
			fmt.Sprintf("Failed to fill configurations: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Validate configurations.
	if err = ValidateConfig(clusterConfig, operConfig); err != nil {
		log.Error(err, "failed to validate configurations")
		r.status.SetDegraded(statusmanager.OperatorConfig, "InvalidOperatorConfig",
			fmt.Sprintf("The operator configuration is invalid: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Generate render data.
	renderData, err := GenerateRenderData(operatorNetwork, operConfig)
	if err != nil {
		log.Error(err, "failed to generate render data")
		r.status.SetDegraded(statusmanager.OperatorConfig, "RenderConfigError",
			fmt.Sprintf("Failed to render operator configurations: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Compare configurations change.
	appliedConfig, err := r.getAppliedOperConfig()
	if err != nil {
		log.Error(err, "failed to get applied config")
		r.status.SetDegraded(statusmanager.OperatorConfig, "InternalError",
			fmt.Sprintf("Failed to get current configurations: %v", err))
		return reconcile.Result{}, err
	}
	agentNeedChange, controllerNeedChange, imageChange := NeedApplyChange(appliedConfig, operConfig)
	if !agentNeedChange && !controllerNeedChange {
		log.Info("no configuration change")
	} else {
		// Render configurations.
		objs, err := render.RenderDir(operatortypes.DefaultManifestDir, renderData)
		if err != nil {
			log.Error(err, "failed to render configuration")
			r.status.SetDegraded(statusmanager.OperatorConfig, "RenderConfigError",
				fmt.Sprintf("Failed to render operator configurations: %v", err))
			return reconcile.Result{Requeue: true}, err
		}

		// Update status and sharedInfo.
		r.sharedInfo.Lock()
		defer r.sharedInfo.Unlock()
		if err = r.updateStatusManagerAndSharedInfo(objs, clusterConfig); err != nil {
			return reconcile.Result{Requeue: true}, err
		}

		// Apply configurations.
		for _, obj := range objs {
			if err = apply.ApplyObject(context.TODO(), r.client, obj); err != nil {
				log.Error(err, "failed to apply resource")
				r.status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError",
					fmt.Sprintf("Failed to apply operator configurations: %v", err))
				return reconcile.Result{Requeue: true}, err
			}
		}

		// Delete old antrea-agent and antrea-controller pods.
		if r.appliedOperConfig != nil && agentNeedChange && !imageChange {
			if err = deleteExistingPods(r.client, operatortypes.AntreaAgentDaemonSetName); err != nil {
				r.status.SetDegraded(statusmanager.OperatorConfig, "DeleteOldPodsError",
					fmt.Sprintf("DaemonSet %s is not using the latest configuration updates because: %v",
						operatortypes.AntreaAgentDaemonSetName, err))
				return reconcile.Result{Requeue: true}, err
			}
		}
		if r.appliedOperConfig != nil && controllerNeedChange && !imageChange {
			if err = deleteExistingPods(r.client, operatortypes.AntreaControllerDeploymentName); err != nil {
				r.status.SetDegraded(statusmanager.OperatorConfig, "DeleteOldPodsError",
					fmt.Sprintf("Deployment %s is not using the latest configuration updates because: %v",
						operatortypes.AntreaControllerDeploymentName, err))
				return reconcile.Result{Requeue: true}, err
			}
		}
	}

	// Update cluster network CR status.
	clusterNetworkConfigChanged := HasClusterNetworkConfigChange(r.appliedClusterConfig, clusterConfig)
	defaultMTUChanged, curDefaultMTU, err := HasDefaultMTUChange(r.appliedOperConfig, operConfig)
	if err != nil {
		r.status.SetDegraded(statusmanager.OperatorConfig, "UpdateNetworkStatusError",
			fmt.Sprintf("failed to check default MTU configuration: %v", err))
		return reconcile.Result{Requeue: true}, err
	}
	if clusterNetworkConfigChanged || defaultMTUChanged {
		if err = updateNetworkStatus(r.client, clusterConfig, curDefaultMTU); err != nil {
			r.status.SetDegraded(statusmanager.ClusterConfig, "UpdateNetworkStatusError",
				fmt.Sprintf("Failed to update network status: %v", err))
			return reconcile.Result{Requeue: true}, err
		}
	}

	r.status.SetNotDegraded(statusmanager.ClusterConfig)
	r.status.SetNotDegraded(statusmanager.OperatorConfig)

	r.appliedClusterConfig = clusterConfig
	r.appliedOperConfig = operConfig
	return reconcile.Result{}, nil
}

func (r *ReconcileConfig) updateStatusManagerAndSharedInfo(objs []*uns.Unstructured, clusterConfig *configv1.Network) error {
	var daemonSets, deployments []types.NamespacedName
	var relatedObjects []configv1.ObjectReference
	var daemonSetObject, deploymentObject *uns.Unstructured
	for _, obj := range objs {
		if obj.GetAPIVersion() == "apps/v1" && obj.GetKind() == "DaemonSet" {
			daemonSets = append(daemonSets, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()})
			daemonSetObject = obj
		} else if obj.GetAPIVersion() == "apps/v1" && obj.GetKind() == "Deployment" {
			deployments = append(deployments, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()})
			deploymentObject = obj
		}
		restMapping, err := r.mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
		if err != nil {
			log.Error(err, "failed to get REST mapping for storing related object")
			continue
		}
		relatedObjects = append(relatedObjects, configv1.ObjectReference{
			Group:     obj.GetObjectKind().GroupVersionKind().Group,
			Resource:  restMapping.Resource.Resource,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		})
		if err = controllerutil.SetControllerReference(clusterConfig, obj, r.scheme); err != nil {
			log.Error(err, "failed to set owner reference", "resource", obj.GetName())
			r.status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError",
				fmt.Sprintf("Failed to set owner reference: %v", err))
			return err
		}
	}
	if daemonSetObject == nil || deploymentObject == nil {
		var missedResources []string
		if daemonSetObject == nil {
			missedResources = append(missedResources, fmt.Sprintf("DaemonSet: %s", operatortypes.AntreaAgentDaemonSetName))
		}
		if deploymentObject == nil {
			missedResources = append(missedResources, fmt.Sprintf("Deployment: %s", operatortypes.AntreaControllerDeploymentName))
		}
		err := fmt.Errorf("configuration of resources %v is missing", missedResources)
		log.Error(nil, err.Error())
		r.status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", err.Error())
		return err
	}
	r.status.SetDaemonSets(daemonSets)
	r.status.SetDeployments(deployments)
	r.status.SetRelatedObjects(relatedObjects)
	r.sharedInfo.AntreaAgentDaemonSetSpec = daemonSetObject.DeepCopy()
	r.sharedInfo.AntreaControllerDeploymentSpec = deploymentObject.DeepCopy()
	return nil
}

func (r *ReconcileConfig) getAppliedOperConfig() (*operatorv1.AntreaInstall, error) {
	if r.appliedOperConfig != nil {
		return r.appliedOperConfig, nil
	}
	operConfig := &operatorv1.AntreaInstall{}
	antreaConfig := corev1.ConfigMap{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.AntreaNamespace, Name: operatortypes.AntreaConfigMapName}, &antreaConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	antreaControllerDeployment := appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.AntreaNamespace, Name: operatortypes.AntreaControllerDeploymentName}, &antreaControllerDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	image := antreaControllerDeployment.Spec.Template.Spec.Containers[0].Image
	operConfigSpec := operatorv1.AntreaInstallSpec{
		AntreaAgentConfig:      antreaConfig.Data[operatortypes.AntreaAgentConfigOption],
		AntreaCNIConfig:        antreaConfig.Data[operatortypes.AntreaCNIConfigOption],
		AntreaControllerConfig: antreaConfig.Data[operatortypes.AntreaControllerConfigOption],
		AntreaImage:            image,
	}
	operConfig.Spec = operConfigSpec
	return operConfig, nil
}

func deleteExistingPods(c client.Client, component string) error {
	var period int64 = 0
	policy := metav1.DeletePropagationBackground
	label := map[string]string{"component": component}
	err := c.DeleteAllOf(context.TODO(), &corev1.Pod{}, client.InNamespace(operatortypes.AntreaNamespace), client.MatchingLabels(label), client.PropagationPolicy(policy), client.GracePeriodSeconds(period))
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to delete pods for component: %s", component))
	}
	return err
}

func updateNetworkStatus(c client.Client, clusterConfig *configv1.Network, defaultMTU int) error {
	status := BuildNetworkStatus(clusterConfig, defaultMTU)
	clusterConfig.Status = *status
	data, err := k8sutil.ToUnstructured(clusterConfig)
	if err != nil {
		log.Error(err, "Failed to render configurations")
		return err
	}

	if data != nil {
		if err := apply.ApplyObject(context.TODO(), c, data); err != nil {
			log.Error(err, fmt.Sprintf("Could not apply (%s) %s/%s", data.GroupVersionKind(),
				data.GetNamespace(), data.GetName()))
			return err
		}
	} else {
		log.Error(err, "Retrieved data for updating network status is empty.")
		return err
	}
	log.Info("Successfully updated Network Status")
	return nil
}
