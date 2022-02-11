/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package statusmanager

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
	"gopkg.in/yaml.v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/sharedinfo"
	operatortypes "github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
	"github.com/vmware/antrea-operator-for-kubernetes/version"
)

var log = logf.Log.WithName("status_manager")

type StatusLevel int

const (
	ClusterConfig StatusLevel = iota
	OperatorConfig
	PodDeployment
	RolloutHung
	ClusterNode
	maxStatusLevel
)

type Adaptor interface {
	getLastPodState(status *StatusManager) (map[types.NamespacedName]daemonsetState, map[types.NamespacedName]deploymentState)
	setLastPodState(status *StatusManager, dss map[types.NamespacedName]daemonsetState, deps map[types.NamespacedName]deploymentState) error
	set(status *StatusManager, reachedAvailableLevel bool, conditions ...configv1.ClusterOperatorStatusCondition)
}

// Status coordinates changes to AntreaInstall.status and ClusterOperator.Status.
type StatusManager struct {
	sync.Mutex

	client client.Client
	mapper meta.RESTMapper

	name    string
	version string

	failing [maxStatusLevel]*configv1.ClusterOperatorStatusCondition

	daemonSets     []types.NamespacedName
	deployments    []types.NamespacedName
	relatedObjects []configv1.ObjectReference

	OperatorNamespace string
	AdaptorName       string
	Adaptor
}

type StatusK8s struct{}

type StatusOc struct{}

func New(client client.Client, mapper meta.RESTMapper, name, operatorNamespace, version string, sharedInfo *sharedinfo.SharedInfo) (*StatusManager, error) {
	status := StatusManager{
		client:            client,
		mapper:            mapper,
		name:              name,
		version:           version,
		OperatorNamespace: operatorNamespace,
		AdaptorName:       sharedInfo.AntreaPlatform,
	}
	switch sharedInfo.AntreaPlatform {
	case "openshift":
		status.Adaptor = &StatusOc{}
	case "kubernetes":
		status.Adaptor = &StatusK8s{}
	default:
		return nil, errors.New("invalid platform: platform should be openshift or kubernetes")
	}
	return &status, nil
}

// deleteRelatedObjects checks for related objects attached to ClusterOperator and deletes
// whatever is not been rendered from manifests. This is a mechanism to cleanup objects
// that are no longer needed and are probably present from a previous version
func (status *StatusManager) deleteRelatedObjectsNotRendered(co *configv1.ClusterOperator) {
	if status.relatedObjects == nil {
		return
	}

	for _, currentObj := range co.Status.RelatedObjects {
		var found bool = false
		for _, renderedObj := range status.relatedObjects {
			found = reflect.DeepEqual(currentObj, renderedObj)

			if found {
				break
			}
		}
		if !found {
			gvr := schema.GroupVersionResource{
				Group:    currentObj.Group,
				Resource: currentObj.Resource,
			}
			gvk, err := status.mapper.KindFor(gvr)
			if err != nil {
				log.Error(err, "Error getting GVK of object for deletion")
				status.relatedObjects = append(status.relatedObjects, currentObj)
				continue
			}
			if gvk.Kind == "Namespace" && gvk.Group == "" {
				// BZ 1820472: During SDN migration, deleting a namespace object may get stuck in 'Terminating' forever if the cluster network doesn't working as expected.
				// We choose to not delete the namespace here but to ask user do it manually after the cluster is back to normal state.
				log.Info("Object Kind is Namespace, skip")
				continue
			}
			objToDelete := &uns.Unstructured{}
			objToDelete.SetName(currentObj.Name)
			objToDelete.SetNamespace(currentObj.Namespace)
			objToDelete.SetGroupVersionKind(gvk)
			err = status.client.Delete(context.TODO(), objToDelete, client.PropagationPolicy("Background"))
			if err != nil {
				log.Error(err, "Error deleting related object")
				if !k8serrors.IsNotFound(err) {
					status.relatedObjects = append(status.relatedObjects, currentObj)
				}
				continue
			}
		}
	}
}

func (status *StatusManager) setConditions(progressing []string, reachedAvailableLevel bool) {
	conditions := make([]configv1.ClusterOperatorStatusCondition, 0, 2)
	if len(progressing) > 0 {
		conditions = append(conditions,
			configv1.ClusterOperatorStatusCondition{
				Type:    configv1.OperatorProgressing,
				Status:  configv1.ConditionTrue,
				Reason:  "Deploying",
				Message: strings.Join(progressing, "\n"),
			},
		)
	} else {
		conditions = append(conditions,
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorProgressing,
				Status: configv1.ConditionFalse,
			},
		)
	}
	if reachedAvailableLevel {
		conditions = append(conditions,
			configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: configv1.ConditionTrue,
			},
		)
	}
	status.set(status, reachedAvailableLevel, conditions...)
}

func (status *StatusManager) setClusterOperatorConditions(co *configv1.ClusterOperator, reachedAvailableLevel bool, conditions *[]configv1.ClusterOperatorStatusCondition) {
	if reachedAvailableLevel {
		co.Status.Versions = []configv1.OperandVersion{
			{Name: "operator", Version: version.Version},
		}
	}
	status.CombineConditions(&co.Status.Conditions, conditions)
	progressingCondition := v1helpers.FindStatusCondition(co.Status.Conditions, configv1.OperatorProgressing)
	availableCondition := v1helpers.FindStatusCondition(co.Status.Conditions, configv1.OperatorAvailable)
	if availableCondition == nil && progressingCondition != nil && progressingCondition.Status == configv1.ConditionTrue {
		v1helpers.SetStatusCondition(&co.Status.Conditions,
			configv1.ClusterOperatorStatusCondition{
				Type:    configv1.OperatorAvailable,
				Status:  configv1.ConditionFalse,
				Reason:  "Startup",
				Message: "The network is starting up",
			},
		)
	}
	v1helpers.SetStatusCondition(&co.Status.Conditions,
		configv1.ClusterOperatorStatusCondition{
			Type:   configv1.OperatorUpgradeable,
			Status: configv1.ConditionTrue,
		},
	)
}

// Set updates the AntreaInstall.Status with the provided conditions for platform kubernetes.
func (adaptor *StatusK8s) set(status *StatusManager, reachedAvailableLevel bool, conditions ...configv1.ClusterOperatorStatusCondition) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		antreaInstall := &operatorv1.AntreaInstall{}
		err := status.client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, antreaInstall)
		if err != nil {
			log.Error(err, "Failed to get antreaInstall")
			return err
		}
		co := &configv1.ClusterOperator{ObjectMeta: metav1.ObjectMeta{Name: status.name}}
		oldStatus := antreaInstall.Status.DeepCopy()
		status.setClusterOperatorConditions(co, reachedAvailableLevel, &conditions)
		if reflect.DeepEqual(oldStatus.Conditions, co.Status.Conditions) {
			return nil
		}
		err = status.setAntreaInstallStatus(&co.Status.Conditions)
		return err
	})
	if err != nil {
		log.Error(err, "Failed to set AntreaInstall")
	}
}

// Set updates the ClusterOperator.Status with the provided conditions for platform openshift.
func (adaptor *StatusOc) set(status *StatusManager, reachedAvailableLevel bool, conditions ...configv1.ClusterOperatorStatusCondition) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		co := &configv1.ClusterOperator{ObjectMeta: metav1.ObjectMeta{Name: status.name}}
		err := status.client.Get(context.TODO(), types.NamespacedName{Name: status.name}, co)
		isNotFound := k8serrors.IsNotFound(err)
		if err != nil && !isNotFound {
			return err
		}
		oldStatus := co.Status.DeepCopy()
		status.deleteRelatedObjectsNotRendered(co)
		if status.relatedObjects != nil {
			co.Status.RelatedObjects = status.relatedObjects
		}
		status.setClusterOperatorConditions(co, reachedAvailableLevel, &conditions)
		if reflect.DeepEqual(*oldStatus, co.Status) {
			return nil
		}
		buf, err := yaml.Marshal(co.Status.Conditions)
		if err != nil {
			buf = []byte(fmt.Sprintf("(failed to convert to YAML: %s)", err))
		}
		if isNotFound {
			if err := status.client.Create(context.TODO(), co); err != nil {
				return err
			}
			log.Info(fmt.Sprintf("Created ClusterOperator with conditions:\n%s", string(buf)))
			return nil
		}
		if err := status.client.Status().Update(context.TODO(), co); err != nil {
			return err
		}
		log.Info(fmt.Sprintf("Updated ClusterOperator with conditions:\n%s", string(buf)))
		err = status.setAntreaInstallStatus(&co.Status.Conditions)
		return err
	})
	if err != nil {
		log.Error(err, "Failed to set ClusterOperator")
	}
}

func (status *StatusManager) setAntreaInstallStatus(conditions *[]configv1.ClusterOperatorStatusCondition) error {
	antreaInstall := &operatorv1.AntreaInstall{}
	err := status.client.Get(context.TODO(), types.NamespacedName{Namespace: operatortypes.OperatorNameSpace, Name: operatortypes.OperatorConfigName}, antreaInstall)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("AntreaInstall not found, skipping set AntreaInstall status")
			return nil
		}
		log.Error(err, "failed to get AntreaInstall")
		return err
	}
	antreaInstallPatch := client.MergeFrom(antreaInstall.DeepCopy())
	antreaInstall.Status.Conditions = *conditions
	if err := status.client.Status().Patch(context.TODO(), antreaInstall, antreaInstallPatch); err != nil {
		log.Error(err, "failed to set AntreaInstall")
		return err
	}
	return err
}

func (status *StatusManager) CombineConditions(conditions *[]configv1.ClusterOperatorStatusCondition,
	newConditions *[]configv1.ClusterOperatorStatusCondition) (bool, string) {
	messages := ""
	changed := false
	for _, newCondition := range *newConditions {
		existingCondition := v1helpers.FindStatusCondition(*conditions, newCondition.Type)
		if existingCondition == nil {
			v1helpers.SetStatusCondition(conditions, newCondition)
			messages += fmt.Sprintf("%v. ", newCondition)
			changed = true
		} else if existingCondition.Status != newCondition.Status ||
			existingCondition.Reason != newCondition.Reason ||
			existingCondition.Message != newCondition.Message {
			v1helpers.SetStatusCondition(conditions, newCondition)
			messages += fmt.Sprintf("%v. ", newCondition)
			changed = true
		}
	}
	return changed, messages
}

func (status *StatusManager) syncDegraded() {
	for _, c := range status.failing {
		if c != nil {
			status.set(status, false, *c)
			return
		}
	}
	status.set(
		status,
		false,
		configv1.ClusterOperatorStatusCondition{
			Type:   configv1.OperatorDegraded,
			Status: configv1.ConditionFalse,
		},
	)
}

func (status *StatusManager) setDegraded(statusLevel StatusLevel, reason, message string) {
	status.failing[statusLevel] = &configv1.ClusterOperatorStatusCondition{
		Type:    configv1.OperatorDegraded,
		Status:  configv1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
	status.syncDegraded()
}

func (status *StatusManager) setNotDegraded(statusLevel StatusLevel) {
	if status.failing[statusLevel] != nil {
		status.failing[statusLevel] = nil
	}
	status.syncDegraded()
}

func (status *StatusManager) SetDegraded(statusLevel StatusLevel, reason, message string) {
	status.Lock()
	defer status.Unlock()
	status.setDegraded(statusLevel, reason, message)
}

func (status *StatusManager) SetNotDegraded(statusLevel StatusLevel) {
	status.Lock()
	defer status.Unlock()
	status.setNotDegraded(statusLevel)
}

func (status *StatusManager) SetDaemonSets(daemonSets []types.NamespacedName) {
	status.Lock()
	defer status.Unlock()
	status.daemonSets = daemonSets
}

func (status *StatusManager) SetDeployments(deployments []types.NamespacedName) {
	status.Lock()
	defer status.Unlock()
	status.deployments = deployments
}

func (status *StatusManager) SetRelatedObjects(relatedObjects []configv1.ObjectReference) {
	status.Lock()
	defer status.Unlock()
	status.relatedObjects = relatedObjects
}
