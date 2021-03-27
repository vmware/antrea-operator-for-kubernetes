/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/apply"
	"github.com/openshift/cluster-network-operator/pkg/render"
	k8sutil "github.com/openshift/cluster-network-operator/pkg/util/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	configutil "github.com/vmware/antrea-operator-for-kubernetes/controllers/config"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/sharedinfo"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/statusmanager"
	operatortypes "github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
)

var log = ctrl.Log.WithName("controllers")

type Adaptor interface {
	SetupWithManager(r *AntreaInstallReconciler, mgr ctrl.Manager) error
	Reconcile(r *AntreaInstallReconciler, request ctrl.Request) (ctrl.Result, error)
	UpdateStatusManagerAndSharedInfo(r *AntreaInstallReconciler, objs []*uns.Unstructured, clusterConfig *configv1.Network) error
}

type AdaptorK8s struct {
	Config configutil.Config
}

type AdaptorOc struct {
	Config configutil.Config
}

func (k8s *AdaptorK8s) SetupWithManager(r *AntreaInstallReconciler, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.AntreaInstall{}).
		Complete(r)
}

func (oc *AdaptorOc) SetupWithManager(r *AntreaInstallReconciler, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.AntreaInstall{}).
		Watches(&source.Kind{Type: &configv1.Network{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func applyConfig(r *AntreaInstallReconciler, config configutil.Config, clusterConfig *configv1.Network, operConfig *operatorv1.AntreaInstall, operatorNetwork *ocoperv1.Network) (reconcile.Result, error) {
	// Fill default configurations.
	if err := config.FillConfigs(clusterConfig, operConfig); err != nil {
		log.Error(err, "failed to fill configurations")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "FillConfigurationsError", fmt.Sprintf("Failed to fill configurations: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Validate configurations.
	if err := config.ValidateConfig(clusterConfig, operConfig); err != nil {
		log.Error(err, "failed to validate configurations")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "InvalidOperatorConfig", fmt.Sprintf("The operator configuration is invalid: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Generate render data.
	renderData, err := config.GenerateRenderData(operatorNetwork, operConfig)
	if err != nil {
		log.Error(err, "failed to generate render data")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "RenderConfigError", fmt.Sprintf("Failed to render operator configurations: %v", err))
		return reconcile.Result{Requeue: true}, err
	}

	// Compare configurations change.
	// TODO why not r.AppliedOperConfig?
	appliedConfig, err := r.getAppliedOperConfig()
	if err != nil {
		log.Error(err, "failed to get applied config")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "InternalError", fmt.Sprintf("Failed to get current configurations: %v", err))
		return reconcile.Result{}, err
	}
	agentNeedChange, controllerNeedChange, imageChange := configutil.NeedApplyChange(appliedConfig, operConfig)
	if !agentNeedChange && !controllerNeedChange {
		log.Info("no configuration change")
	} else {
		// Render configurations.
		objs, err := render.RenderDir(operatortypes.DefaultManifestDir, renderData)
		if err != nil {
			log.Error(err, "failed to render configuration")
			r.Status.SetDegraded(statusmanager.OperatorConfig, "RenderConfigError", fmt.Sprintf("Failed to render operator configurations: %v", err))
			return reconcile.Result{Requeue: true}, err
		}

		// Update status and sharedInfo.
		r.SharedInfo.Lock()
		defer r.SharedInfo.Unlock()
		if err = r.UpdateStatusManagerAndSharedInfo(r, objs, clusterConfig); err != nil {
			return reconcile.Result{Requeue: true}, err
		}

		// Apply configurations.
		for _, obj := range objs {
			if err = apply.ApplyObject(context.TODO(), r.Client, obj); err != nil {
				log.Error(err, "failed to apply resource")
				r.Status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", fmt.Sprintf("Failed to apply operator configurations: %v", err))
				return reconcile.Result{Requeue: true}, err
			}
		}

		// Delete old antrea-agent and antrea-controller pods.
		if r.AppliedOperConfig != nil && agentNeedChange && !imageChange {
			if err = deleteExistingPods(r.Client, operatortypes.AntreaAgentDaemonSetName); err != nil {
				msg := fmt.Sprintf("DaemonSet %s is not using the latest configuration updates because: %v", operatortypes.AntreaAgentDaemonSetName, err)
				r.Status.SetDegraded(statusmanager.OperatorConfig, "DeleteOldPodsError", msg)
				return reconcile.Result{Requeue: true}, err
			}
		}
		if r.AppliedOperConfig != nil && controllerNeedChange && !imageChange {
			if err = deleteExistingPods(r.Client, operatortypes.AntreaControllerDeploymentName); err != nil {
				msg := fmt.Sprintf("Deployment %s is not using the latest configuration updates because: %v", operatortypes.AntreaControllerDeploymentName, err)
				r.Status.SetDegraded(statusmanager.OperatorConfig, "DeleteOldPodsError", msg)
				return reconcile.Result{Requeue: true}, err
			}
		}
	}
	return reconcile.Result{}, nil
}

func (k8s *AdaptorK8s) Reconcile(r *AntreaInstallReconciler, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.NamespacedName", request.NamespacedName)
	if request.Namespace == "" && request.Name == operatortypes.ClusterConfigName {
		reqLogger.Info("Reconciling antrea-operator Cluster Network CR change")
	} else if request.Namespace == operatortypes.OperatorNameSpace && request.Name == operatortypes.OperatorConfigName {
		reqLogger.Info("Reconciling antrea-operator antrea-install CR change")
	} else {
		return reconcile.Result{}, nil
	}

	// Fetch antrea-install CR.
	operConfig := &operatorv1.AntreaInstall{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, operConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s CR not found", operatortypes.OperatorConfigName)
			log.Info(msg)
			r.Status.SetDegraded(statusmanager.ClusterConfig, "NoAntreaInstallCR", msg)
			return reconcile.Result{}, nil
		}
		log.Error(err, "failed to get antrea-install CR")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "InvalidAntreaInstallCR", fmt.Sprintf("Failed to get operator CR: %v", err))
		return reconcile.Result{Requeue: true}, err
	}
	if request.Name == operConfig.Name && r.AppliedOperConfig != nil {
		if reflect.DeepEqual(operConfig.Spec, r.AppliedOperConfig.Spec) {
			log.Info("no configuration change")
			return reconcile.Result{}, nil
		}
	}

	// Apply configuration.
	if result, err := applyConfig(r, k8s.Config, nil, operConfig, nil); err != nil {
		return result, err
	}

	r.Status.SetNotDegraded(statusmanager.ClusterConfig)
	r.Status.SetNotDegraded(statusmanager.OperatorConfig)

	r.AppliedOperConfig = operConfig

	return ctrl.Result{}, nil
}

func (oc *AdaptorOc) Reconcile(r *AntreaInstallReconciler, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.NamespacedName", request.NamespacedName)
	if request.Namespace == "" && request.Name == operatortypes.ClusterConfigName {
		reqLogger.Info("Reconciling antrea-operator Cluster Network CR change")
	} else if request.Namespace == operatortypes.OperatorNameSpace && request.Name == operatortypes.OperatorConfigName {
		reqLogger.Info("Reconciling antrea-operator antrea-install CR change")
	} else {
		return reconcile.Result{}, nil
	}

	// Fetch Cluster Network CR.
	clusterConfig := &configv1.Network{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: operatortypes.ClusterConfigName}, clusterConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := "Cluster Network CR not found"
			log.Info(msg)
			r.Status.SetDegraded(statusmanager.ClusterConfig, "NoClusterConfig", msg)
			return reconcile.Result{}, nil
		}
		r.Status.SetDegraded(statusmanager.ClusterConfig, "InvalidClusterConfig", fmt.Sprintf("Failed to get cluster network CRD: %v", err))
		log.Error(err, "failed to get Cluster Network CR")
		return reconcile.Result{Requeue: true}, err
	}
	if request.Name == clusterConfig.Name && r.AppliedClusterConfig != nil {
		if reflect.DeepEqual(clusterConfig.Spec, r.AppliedClusterConfig.Spec) {
			log.Info("no configuration change")
			return reconcile.Result{}, nil
		}
	}

	// Fetch the Network.operator.openshift.io instance
	operatorNetwork := &ocoperv1.Network{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: operatortypes.ClusterOperatorNetworkName}, operatorNetwork)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Status.SetDegraded(statusmanager.OperatorConfig, "NoClusterNetworkOperatorConfig", fmt.Sprintf("Cluster network operator configuration not found"))
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Unable to retrieve Network.operator.openshift.io object")
		return reconcile.Result{Requeue: true}, err
	}

	// Fetch antrea-install CR.
	operConfig := &operatorv1.AntreaInstall{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, operConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("%s CR not found", operatortypes.OperatorConfigName)
			log.Info(msg)
			r.Status.SetDegraded(statusmanager.ClusterConfig, "NoAntreaInstallCR", msg)
			return reconcile.Result{}, nil
		}
		log.Error(err, "failed to get antrea-install CR")
		r.Status.SetDegraded(statusmanager.OperatorConfig, "InvalidAntreaInstallCR", fmt.Sprintf("Failed to get operator CR: %v", err))
		return reconcile.Result{Requeue: true}, err
	}
	if request.Name == operConfig.Name && r.AppliedOperConfig != nil {
		if reflect.DeepEqual(operConfig.Spec, r.AppliedOperConfig.Spec) {
			log.Info("no configuration change")
			return reconcile.Result{}, nil
		}
	}

	// Apply configuration.
	if result, err := applyConfig(r, oc.Config, clusterConfig, operConfig, operatorNetwork); err != nil {
		return result, err
	}

	// Update cluster network CR status.
	clusterNetworkConfigChanged := configutil.HasClusterNetworkConfigChange(r.AppliedClusterConfig, clusterConfig)
	defaultMTUChanged, curDefaultMTU, err := configutil.HasDefaultMTUChange(r.AppliedOperConfig, operConfig)
	if err != nil {
		r.Status.SetDegraded(statusmanager.OperatorConfig, "UpdateNetworkStatusError", fmt.Sprintf("failed to check default MTU configuration: %v", err))
		return reconcile.Result{Requeue: true}, err
	}
	if clusterNetworkConfigChanged || defaultMTUChanged {
		if err = updateNetworkStatus(r.Client, clusterConfig, curDefaultMTU); err != nil {
			r.Status.SetDegraded(statusmanager.ClusterConfig, "UpdateNetworkStatusError", fmt.Sprintf("Failed to update network status: %v", err))
			return reconcile.Result{Requeue: true}, err
		}
	}

	r.Status.SetNotDegraded(statusmanager.ClusterConfig)
	r.Status.SetNotDegraded(statusmanager.OperatorConfig)

	r.AppliedClusterConfig = clusterConfig
	r.AppliedOperConfig = operConfig

	return ctrl.Result{}, nil
}

// AntreaInstallReconciler reconciles a AntreaInstall object
type AntreaInstallReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Status *statusmanager.StatusManager
	Mapper meta.RESTMapper

	Adaptor

	SharedInfo           *sharedinfo.SharedInfo
	AppliedClusterConfig *configv1.Network
	AppliedOperConfig    *operatorv1.AntreaInstall
}

func New(mgr ctrl.Manager, statusManager *statusmanager.StatusManager, info *sharedinfo.SharedInfo) (*AntreaInstallReconciler, error) {
	r := AntreaInstallReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AntreaInstall"),
		Scheme:     mgr.GetScheme(),
		Status:     statusManager,
		Mapper:     mgr.GetRESTMapper(),
		SharedInfo: info,
	}
	switch info.AntreaPlatform {
	case "openshift":
		r.Adaptor = &AdaptorOc{
			Config: &configutil.ConfigOc{},
		}
	case "kubernetes":
		r.Adaptor = &AdaptorK8s{
			Config: &configutil.ConfigK8s{},
		}
	default:
		return nil, errors.New("invalid platform: platform should be openshift or kubernetes")
	}
	return &r, nil
}

func (r *AntreaInstallReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.Adaptor.SetupWithManager(r, mgr)
}

// +kubebuilder:rbac:groups=operator.antrea.vmware.com,resources=antreainstalls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.antrea.vmware.com,resources=antreainstalls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=networks,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=config.openshift.io,resources=networks/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=operator.openshift.io,resources=networks,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;watch;list
// +kubebuilder:rbac:groups="",resources=pods;endpoints,verbs=get;watch;list;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews;subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=get;create;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;watch;list
// +kubebuilder:rbac:groups=ops.antrea.tanzu.vmware.com,resources=traceflows;traceflows/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clusterinformation.antrea.tanzu.vmware.com,resources=antreaagentinfos;antreacontrollerinfos,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=networking.antrea.tanzu.vmware.com,resources=networkpolicies;appliedtogroups;addressgroups,verbs=get;watch;list;delete
// +kubebuilder:rbac:groups=security.antrea.tanzu.vmware.com,resources=clusternetworkpolicies,verbs=get;watch;list;delete
// +kubebuilder:rbac:groups=system.antrea.tanzu.vmware.com,resources=controllerinfos;agentinfos;supportbundles;supportbundles/download,verbs=get;watch;list;post;delete
// +kubebuilder:rbac:urls=/agentinfo;/addressgroups;/appliedtogroups;/networkpolicies;/ovsflows;/ovstracing;/podinterfaces,verbs=get

func (r *AntreaInstallReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	return r.Adaptor.Reconcile(r, request)
}

func (r *AntreaInstallReconciler) getAppliedOperConfig() (*operatorv1.AntreaInstall, error) {
	if r.AppliedOperConfig != nil {
		return r.AppliedOperConfig, nil
	}
	operConfig := &operatorv1.AntreaInstall{}
	antreaConfig := corev1.ConfigMap{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.AntreaNamespace, Name: operatortypes.AntreaConfigMapName}, &antreaConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	antreaControllerDeployment := appsv1.Deployment{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.AntreaNamespace, Name: operatortypes.AntreaControllerDeploymentName}, &antreaControllerDeployment); err != nil {
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
	status := configutil.BuildNetworkStatus(clusterConfig, defaultMTU)
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

func updateStatusManagerAndSharedInfo(r *AntreaInstallReconciler, objs []*uns.Unstructured, clusterConfig *configv1.Network) error {
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
		restMapping, err := r.Mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
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
		if clusterConfig != nil {
			if err := controllerutil.SetControllerReference(clusterConfig, obj, r.Scheme); err != nil {
				log.Error(err, "failed to set owner reference", "resource", obj.GetName())
				r.Status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", fmt.Sprintf("Failed to set owner reference: %v", err))
				return err
			}
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
		r.Status.SetDegraded(statusmanager.OperatorConfig, "ApplyObjectsError", err.Error())
		return err
	}
	r.Status.SetDaemonSets(daemonSets)
	r.Status.SetDeployments(deployments)
	r.Status.SetRelatedObjects(relatedObjects)
	r.SharedInfo.AntreaAgentDaemonSetSpec = daemonSetObject.DeepCopy()
	r.SharedInfo.AntreaControllerDeploymentSpec = deploymentObject.DeepCopy()
	return nil
}

func (a *AdaptorK8s) UpdateStatusManagerAndSharedInfo(r *AntreaInstallReconciler, objs []*uns.Unstructured, clusterConfig *configv1.Network) error {
	return updateStatusManagerAndSharedInfo(r, objs, clusterConfig)
}

func (a *AdaptorOc) UpdateStatusManagerAndSharedInfo(r *AntreaInstallReconciler, objs []*uns.Unstructured, clusterConfig *configv1.Network) error {
	return updateStatusManagerAndSharedInfo(r, objs, clusterConfig)
}
