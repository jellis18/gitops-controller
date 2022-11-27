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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	// Reference to the location of the applications manifests
	Source ApplicationSource `json:"source"`

	//+kubebuilder:validation:Minimum=1

	// Time in between sync attempts in minutes. Defaults to 3.
	// +optional
	SyncPeriodMinutes *int32 `json:"syncPeriod,omitempty"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	// List of k8s resources managed by this application
	Resources []Resource `json:"resources,omitempty"`

	// Time indicating last time application state was reconciled
	ReconciledAt *metav1.Time `json:"reconciledAt,omitempty"`

	// Time indicating lst time application was synced
	SyncedAt *metav1.Time `json:"syncedAt,omitempty"`

	// Information about sync
	Sync SyncStatus `json:"sync"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=.status.sync.syncStatus,name=status,type=string

// Application is the Schema for the applications API
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// ApplicationSource contains all required information about the (git) source of the application
type ApplicationSource struct {
	// URL to the git repository that contains the application manifests
	RepoURL string `json:"repoURL"`

	// Path is the directory within the Git repository where your manifest(s) live(s)
	Path string `json:"path"`

	// Defines the revision of the source to the sync the application to.
	// This can be a git commit, tag or branch.
	// If empty will default to HEAD
	// +optional
	TargetRevision string `json:"targetRevision,omitempty"`
}

// SyncStatusCode is a type representing possible comparison/sync states
type SyncStatusCode string

const (
	// Status could not be determined
	SyncStatusUnknown SyncStatusCode = "Unknown"

	// Currently synced with state in git repository
	SyncStatusSynced SyncStatusCode = "Synced"

	// Currently out of sync with state in git repository
	SyncStatusOutOfSync SyncStatusCode = "OutOfSync"
)

// Resource holds the current information about a k8s resource
type Resource struct {
	// +optional
	Group string `json:"group,omitempty"`

	// +optional
	Version string `json:"version,omitempty"`

	// +optional
	Kind string `json:"kind,omitempty"`

	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Current status compared to git repository
	// Valid values are:
	// - "Unknown";
	// - "Synced";
	// - "OutOfSync"
	// +optional
	Status SyncStatusCode `json:"status,omitempty"`
}

type SyncStatus struct {
	SyncStatus SyncStatusCode    `json:"syncStatus"`
	Source     ApplicationSource `json:"source"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
