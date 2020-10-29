// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	akoov1alpha1 "gitlab.eng.vmware.com/fangyuanl/akoo/api/v1alpha1"

	controllerruntime "gitlab.eng.vmware.com/fangyuanl/akoo/pkg/controller-runtime"
	"gitlab.eng.vmware.com/fangyuanl/akoo/pkg/controller-runtime/handlers"
	"gitlab.eng.vmware.com/fangyuanl/akoo/pkg/controller-runtime/patch"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SetupWithManager adds this reconciler to a new controller then to the
// provided manager.
func (r *MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch Cluster API Machine resources.
		For(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handlers.MachinesForCluster(r.Client, r.Log),
			},
		).
		Complete(r)
}

type MachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *MachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	log := r.Log.WithValues("Machine", req.NamespacedName)

	res := ctrl.Result{}
	// Get the resource for this request.
	obj := &clusterv1.Machine{}
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Machine not found, will not reconcile")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Always Patch when exiting this function so changes to the resource are updated on the API server.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to init patch helper for %s %s",
			obj.GroupVersionKind(), req.NamespacedName)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, obj); err != nil {
			if reterr == nil {
				reterr = err
			}
			log.Error(err, "patch failed")
		}
	}()

	// Get the name of the cluster to which the current machine belongs
	clusterName, exist := obj.Labels[clusterv1.ClusterLabelName]
	if !exist {
		log.Info("machine doesn't have cluster name label, skip reconciling")
		return res, nil
	}

	// Get the Cluster object to ensure it has AVI enabled
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: obj.Namespace,
		Name:      clusterName,
	}, cluster); err != nil {
		return res, err
	}

	log = log.WithValues("Cluster", cluster.Namespace+"/"+cluster.Name)

	if _, exist := cluster.Labels[akoov1alpha1.AviClusterLabel]; !exist {
		log.Info("Cluster doesn't have AVI enabled, skip reconciling")
		return res, nil
	}

	// Handle deleted cluster resources.
	if !cluster.GetDeletionTimestamp().IsZero() {
		res, err := r.reconcileClusterDelete(ctx, log, obj, cluster)
		if err != nil {
			log.Error(err, "failed to reconcile Machine deletion")
			return res, err
		}
		return res, nil
	}

	// Handle non-deleted resources.
	if res, err := r.reconcileNormal(ctx, log, obj, cluster); err != nil {
		log.Error(err, "failed to reconcile Machine")
		return res, err
	}
	return res, nil
}

func (r *MachineReconciler) reconcileClusterDelete(
	ctx context.Context,
	log logr.Logger,
	obj *clusterv1.Machine,
	cluster *clusterv1.Cluster,
) (ctrl.Result, error) {
	log.Info("Start reconciling cluster delete")
	return r.reconcileMachineDeletionHook(ctx, log, obj, cluster)
}

// reconcileNormal adds the pre-terminate machine deletion phase hook to the
// Machine
func (r *MachineReconciler) reconcileNormal(
	ctx context.Context,
	log logr.Logger,
	obj *clusterv1.Machine,
	cluster *clusterv1.Cluster,
) (ctrl.Result, error) {
	log.Info("Start reconciling")

	// Add pre-terminate machine deletion phase hook if it doesn't exist
	if _, exist := obj.Annotations[clusterv1.PreTerminateDeleteHookAnnotationPrefix]; !exist {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[preTerminateAnnotation()] = "ako-operator"
	}

	return ctrl.Result{}, nil
}

// reconcileMachineDeletionHook removes the pre-terminate hook when the finalizer on the Cluster
// is absent
func (r *MachineReconciler) reconcileMachineDeletionHook(
	ctx context.Context,
	log logr.Logger,
	obj *clusterv1.Machine,
	cluster *clusterv1.Cluster,
) (ctrl.Result, error) {
	log.Info("Start reconciling machine deletion pre-terminate hook")

	res := ctrl.Result{}

	if controllerruntime.ContainsFinalizer(cluster, akoov1alpha1.ClusterFinalizer) {
		log.Info("Cluster has finalizer set. Clean up has not finished. Will skip reconciling", "finalizer", akoov1alpha1.ClusterFinalizer)
		return res, nil
	}

	if annotations.HasWithPrefix(clusterv1.PreTerminateDeleteHookAnnotationPrefix, obj.ObjectMeta.Annotations) {
		// Removes the pre-terminate hook as the cleanup has finished
		delete(obj.Annotations, preTerminateAnnotation())
		log.Info("Removing pre-terminate hook")
	}

	return res, nil
}

func preTerminateAnnotation() string {
	return clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/avi-cleanup"
}