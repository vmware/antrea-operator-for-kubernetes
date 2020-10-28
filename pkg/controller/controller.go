/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package controller

import (
	"github.com/openshift/cluster-network-operator/pkg/controller/statusmanager"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	operatorversion "github.com/ruicao93/antrea-operator/pkg/version"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, *statusmanager.StatusManager) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager) error {
	s := statusmanager.New(m.GetClient(), m.GetRESTMapper(), "antrea", operatorversion.Version)
	for _, f := range AddToManagerFuncs {
		if err := f(m, s); err != nil {
			return err
		}
	}
	return nil
}
