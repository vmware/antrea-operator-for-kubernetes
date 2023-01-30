/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package main

import (
	"flag"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	ocoperv1 "github.com/openshift/api/operator/v1"
	cnoclient "github.com/openshift/cluster-network-operator/pkg/client"
	"github.com/openshift/cluster-network-operator/pkg/names"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1 "github.com/vmware/antrea-operator-for-kubernetes/api/v1"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/sharedinfo"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/statusmanager"
	"github.com/vmware/antrea-operator-for-kubernetes/controllers/types"
	"github.com/vmware/antrea-operator-for-kubernetes/version"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(ocoperv1.Install(scheme))

	utilruntime.Must(operatorv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", "0", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "antrea-operator.antrea.vmware.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	sharedInfo, err := sharedinfo.New(mgr)
	if err != nil {
		setupLog.Error(err, "unable to get shareinfo")
		os.Exit(1)
	}
	statusManager, err := statusmanager.New(mgr.GetClient(), mgr.GetRESTMapper(), types.AntreaClusterOperatorName, types.OperatorNameSpace, version.Version, sharedInfo)
	if err != nil {
		setupLog.Error(err, "unable to get status manager")
		os.Exit(1)
	}
	cnoClient, err := cnoclient.NewClient(cfg, cfg, names.DefaultClusterName, nil)
	if err != nil {
		setupLog.Error(err, "fail to create client")
		os.Exit(1)
	}
	controller, err := controllers.New(mgr, statusManager, sharedInfo, cnoClient)
	if err != nil {
		setupLog.Error(err, "unable to get controller")
		os.Exit(1)
	}
	if err = controller.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AntreaInstall")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder
	if err = (&controllers.PodReconciler{
		Client:     cnoClient,
		Log:        ctrl.Log.WithName("controllers").WithName("Pod"),
		Scheme:     mgr.GetScheme(),
		Status:     statusManager,
		SharedInfo: sharedInfo,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AntreaInstall")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
