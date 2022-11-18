package utils

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func deploy(ctx context.Context, clientset *kubernetes.Clientset, stream []byte) (*v1.Deployment, error) {
	var deployment *v1.Deployment

	obj, gKV, _ := scheme.Codecs.UniversalDeserializer().Decode(stream, nil, nil)
	if gKV.Kind == "Deployment" {
		deployment = obj.(*v1.Deployment)
	} else {
		return nil, fmt.Errorf("unrecognized type %s", gKV.Kind)
	}

	_, err := clientset.AppsV1().Deployments("default").Get(ctx, deployment.Name, metav1.GetOptions{})

	// if the deployment doesn't exist, create it
	if err != nil && errors.IsNotFound(err) {
		log.Printf("Creating deploment %s\n", deployment.Name)
		depl, err := clientset.AppsV1().Deployments("default").Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		return depl, nil

	} else if err != nil {
		return nil, err
	}

	// otherwise update it
	log.Printf("Updating deploment %s\n", deployment.Name)
	depl, err := clientset.AppsV1().Deployments("default").Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return depl, nil
}

func waitForPods(ctx context.Context, clientset *kubernetes.Clientset, deploymentLabels map[string]string, replicaCount int) error {
	for {
		validatedLabels, err := labels.ValidatedSelectorFromSet(deploymentLabels)
		if err != nil {
			return err
		}

		pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: validatedLabels.String(),
		})
		if err != nil {
			return err
		}

		podsRunning := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				podsRunning++
			}
		}
		fmt.Printf("Waiting for pods. (%d/%d) running\n", podsRunning, replicaCount)
		if podsRunning == replicaCount {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return nil

}

func GetK8sClient(inCluster bool) (*kubernetes.Clientset, error) {
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
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
