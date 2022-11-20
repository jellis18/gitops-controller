package utils

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func NewDynamicClient(inCluster bool) (dynamic.Interface, error) {
	var (
		config *rest.Config
		err    error
	)

	if inCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		home := homedir.HomeDir()
		kubeconfig := flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")

		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	// create the clientset
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func reconcile(ctx context.Context, dynamicClient dynamic.Interface, objs []unstructured.Unstructured) error {
	for _, obj := range objs {
		gvk := obj.GroupVersionKind()

		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: fmt.Sprintf("%ss", strings.ToLower(gvk.Kind)),
		}

		targetNamespace := obj.GetNamespace()
		if targetNamespace == "" {
			targetNamespace = "default"
		}

		objName := obj.GetName()
		if objName == "" {
			log.Printf("Cannot apply object %s with no name\n", obj.Object)
			continue
		}

		resource := dynamicClient.Resource(gvr).Namespace(targetNamespace)

		// TODO: add specific annotation here to let us know which objects we control

		// first try to get resource
		_, err := resource.Get(ctx, objName, metav1.GetOptions{})

		// if not found, create it
		if err != nil && errors.IsNotFound(err) {
			log.Printf("Creating %s: %s in namespace %s\n", gvk.Kind, objName, targetNamespace)
			_, err = resource.Create(ctx, &obj, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			return nil

		} else if err != nil {
			return err
		}

		// otherwise update it
		log.Printf("Updating %s: %s in namespace %s\n", gvk.Kind, objName, targetNamespace)
		_, err = resource.Update(ctx, &obj, metav1.UpdateOptions{})
		if err != nil {
			return err
		}

	}
	return nil
}
