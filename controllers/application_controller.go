/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gitopsv1 "github.com/jellis18/gitops-controller/api/v1"
)

const (
	finalizerName string = "gitops.jellis18.gitopscontroller.io/finalizer"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	StateManager *AppStateManager
}

//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Running reconciler")

	var app gitopsv1.Application
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		log.Error(err, "could not fetch Application")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 0. handle delete case
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		// not being deleted, so lets add finalizer
		if !controllerutil.ContainsFinalizer(&app, finalizerName) {
			controllerutil.AddFinalizer(&app, finalizerName)
			app.Status.ReconciledAt = &metav1.Time{Time: time.Now()}
			if err := r.Update(ctx, &app); err != nil {
				log.Error(err, "could not update application", "app", app)
				return ctrl.Result{}, err
			}
		}
	} else {
		// The Application is being deleted
		if controllerutil.ContainsFinalizer(&app, finalizerName) {
			// delete target managed resources
			log.Info(fmt.Sprintf("Deleting managed resources for app %s", app.Name))
			if err := r.deleteManagedResources(ctx, &app); err != nil {
				log.Error(err, "could not delete managed resources")
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(&app, finalizerName)
			app.Status.ReconciledAt = &metav1.Time{Time: time.Now()}
			if err := r.Update(ctx, &app); err != nil {
				log.Error(err, "could not update application", "app", app)
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation
		log.Info(fmt.Sprintf("Application %s deleted", app.Name))
		return ctrl.Result{}, nil
	}

	// 1. Get target Objects from repo
	targetObjs, err := r.StateManager.getRepoObjs(ctx, &app)
	if err != nil {
		log.Error(err, "could not fetch k8s resources from git repo")
		// TODO: should retry with some limit but we will just return for now
		return ctrl.Result{}, nil
	}

	// 2. Get live state using GVK from target objects
	// var liveObjs []*unstructured.Unstructured
	// for _, target := range targetObjs {
	// 	u := &unstructured.Unstructured{}
	// 	u.SetGroupVersionKind(target.GroupVersionKind())
	// 	if err := r.Get(ctx, client.ObjectKey{Namespace: target.GetNamespace(), Name: target.GetName()}, u); client.IgnoreNotFound(err) != nil {
	// 		log.Error(err, "could not fetch live object corresponding to target object", "target", target)
	// 		return ctrl.Result{}, err
	// 	}
	// 	liveObjs = append(liveObjs, u)
	// }

	// 3. Get tracked object metadata from status (for orphans)

	// 4. Create or update (for now don't worry about checking status)
	var resourceList []gitopsv1.Resource
	for _, target := range targetObjs {
		u := &unstructured.Unstructured{}
		// TODO: this should be handled on the app spec
		namespace := target.GetNamespace()
		if namespace == "" {
			target.SetNamespace("default")
		}

		gvk := target.GroupVersionKind()
		resourceList = append(resourceList, gitopsv1.Resource{
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
			Name:      target.GetName(),
			Namespace: target.GetNamespace(),
			Status:    gitopsv1.SyncStatusSynced,
		})
		u.SetGroupVersionKind(gvk)
		err := r.Get(ctx, client.ObjectKey{Namespace: target.GetNamespace(), Name: target.GetName()}, u)
		if err != nil && errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Creating %s: %s in namespace %s\n", target.GetKind(), target.GetName(), target.GetNamespace()))
			if err := r.Create(ctx, target); err != nil {
				log.Error(err, "could not create object", "target", target)
				return ctrl.Result{}, err
			}
		} else if err != nil {
			log.Error(err, "could not fetch target object", "target", target)
			return ctrl.Result{}, err
		} else {
			log.Info(fmt.Sprintf("Updating %s: %s in namespace %s\n", target.GetKind(), target.GetName(), target.GetNamespace()))
			if err := r.Update(ctx, target); err != nil {
				log.Error(err, "could not update object", "target", target)
				return ctrl.Result{}, err
			}
		}
	}
	// should really wait for these to be synced but for now just add to the resource list
	app.Status.SyncedAt = &metav1.Time{Time: time.Now()}
	app.Status.ReconciledAt = &metav1.Time{Time: time.Now()}
	app.Status.Resources = resourceList
	app.Status.Sync = gitopsv1.SyncStatus{SyncStatus: gitopsv1.SyncStatusSynced, Source: app.Spec.Source}
	if err := r.Update(ctx, &app); err != nil {
		log.Error(err, fmt.Sprintf("could not update application %s", app.Name))
		return ctrl.Result{}, err
	}

	// 5. Remove orphans

	// determine time for next sync and requeue with delay
	if app.Spec.SyncPeriodMinutes == nil {
		log.Error(errors.NewBadRequest(".spec.syncPeriod must be set"), "No sync period found")
		return ctrl.Result{}, nil
	}

	nextRun := time.Minute * time.Duration(*app.Spec.SyncPeriodMinutes)

	return ctrl.Result{RequeueAfter: nextRun}, nil
}

func (r *ApplicationReconciler) deleteManagedResources(ctx context.Context, app *gitopsv1.Application) error {
	log := log.FromContext(ctx)

	for _, managedResource := range app.Status.Resources {
		gvk := schema.GroupVersionKind{
			Group:   managedResource.Group,
			Version: managedResource.Version,
			Kind:    managedResource.Kind,
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		if err := r.Get(ctx, client.ObjectKey{Namespace: managedResource.Namespace, Name: managedResource.Name}, u); client.IgnoreNotFound(err) != nil {
			return err
		}
		log.Info(fmt.Sprintf("deleting %s: %s in namespace %s", managedResource.Kind, managedResource.Name, managedResource.Namespace))
		if err := r.Delete(ctx, u); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitopsv1.Application{}).
		Complete(r)
}
