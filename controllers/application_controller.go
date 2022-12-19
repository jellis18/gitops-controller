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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gitopsv1 "github.com/jellis18/gitops-controller/api/v1"
)

const (
	finalizerName     string = "gitops.jellis18.gitopscontroller.io/finalizer"
	apiTokenSecretKey string = "apiToken"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gitops.jellis18.gitopscontroller.io,resources=applications/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// This is a simple reconciler that will always re-sync the application to the desired state in git on a change
// It will detect orphans (i.e. k8s resources that are live in the cluster but no longer in the target git repo)
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
			if err := r.deleteResources(ctx, app.Status.Resources); err != nil {
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
	stateManager, err := r.getAppStateManager(ctx, &app)
	if err != nil {
		log.Error(err, "Error creating state manager")
		return ctrl.Result{}, err
	}

	targetObjs, err := stateManager.getRepoObjs(ctx, &app)
	if err != nil {
		log.Error(err, "could not fetch k8s resources from git repo")
		// TODO: should retry with some limit but we will just return for now
		return ctrl.Result{}, nil
	}

	// 2. Create or update (for now don't worry about checking status)
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

	// 4. Remove orphans
	orphans := r.findOrphans(&app, resourceList)
	if err := r.deleteResources(ctx, orphans); err != nil {
		log.Error(err, "could not delete orphans")
		return ctrl.Result{}, err
	}

	// should really wait for these to be synced but for now just add to the resource list
	app.Status.SyncedAt = &metav1.Time{Time: time.Now()}
	app.Status.ReconciledAt = &metav1.Time{Time: time.Now()}
	app.Status.Resources = resourceList
	app.Status.Sync = gitopsv1.SyncStatus{SyncStatus: gitopsv1.SyncStatusSynced, Source: app.Spec.Source}
	log.Info("Updating Application status")
	if err := r.Status().Update(ctx, &app); err != nil {
		log.Error(err, fmt.Sprintf("could not update application %s", app.Name))
		return ctrl.Result{}, err
	}

	// determine time for next sync and requeue with delay
	if app.Spec.SyncPeriodMinutes == nil {
		log.Error(errors.NewBadRequest(".spec.syncPeriod must be set"), "No sync period found")
		return ctrl.Result{}, nil
	}

	nextRun := time.Minute * time.Duration(*app.Spec.SyncPeriodMinutes)

	return ctrl.Result{RequeueAfter: nextRun}, nil
}

func (r *ApplicationReconciler) deleteResources(ctx context.Context, resources []gitopsv1.Resource) error {
	log := log.FromContext(ctx)

	for _, resource := range resources {
		gvk := schema.GroupVersionKind{
			Group:   resource.Group,
			Version: resource.Version,
			Kind:    resource.Kind,
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		if err := r.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: resource.Name}, u); client.IgnoreNotFound(err) != nil {
			return err
		}
		log.Info(fmt.Sprintf("deleting %s: %s in namespace %s", resource.Kind, resource.Name, resource.Namespace))
		if err := r.Delete(ctx, u); err != nil {
			return err
		}
	}
	return nil
}

func (r *ApplicationReconciler) findOrphans(app *gitopsv1.Application, targetResourceList []gitopsv1.Resource) []gitopsv1.Resource {

	resources := []gitopsv1.Resource{}

	// get mapping for target resources
	type key struct{ name, namespace, group, version, kind string }
	targetResourceMapping := make(map[key]gitopsv1.Resource)
	for _, resource := range targetResourceList {
		targetResourceMapping[key{
			name:      resource.Name,
			namespace: resource.Namespace,
			group:     resource.Group,
			version:   resource.Version,
			kind:      resource.Kind,
		}] = resource
	}

	// remove orphans by looping over current managed resources and finding those that are not in the target resources
	for _, managedResource := range app.Status.Resources {
		_, ok := targetResourceMapping[key{
			name:      managedResource.Name,
			namespace: managedResource.Namespace,
			group:     managedResource.Group,
			version:   managedResource.Version,
			kind:      managedResource.Kind,
		}]
		// if this resource is not in the target list, it is an orphan, delete it
		if !ok {
			resources = append(resources, managedResource)
		}
	}
	return resources
}

// Find secret, get api token and initialize state manager
func (r *ApplicationReconciler) getAppStateManager(ctx context.Context, app *gitopsv1.Application) (*AppStateManager, error) {
	secretName := app.Spec.Source.RepoSecret
	var stateManager *AppStateManager
	if secretName != "" {
		var repoSecret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: app.Namespace}, &repoSecret); err != nil {
			return nil, fmt.Errorf("could not find secret %s", secretName)
		}
		apiTokenBytes, ok := repoSecret.Data[apiTokenSecretKey]
		if !ok {
			return nil, fmt.Errorf("could not access secret data; %s from secret %s", apiTokenSecretKey, repoSecret.Name)
		}
		stateManager = NewAppStateManager(string(apiTokenBytes))
	} else {
		stateManager = NewAppStateManager("")
	}
	return stateManager, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitopsv1.Application{}).
		Complete(r)
}
