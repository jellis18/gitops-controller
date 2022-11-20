package utils

import (
	"bytes"
	"context"
	"io"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

func SyncGitRepo(ctx context.Context, gitApp *GitApp, dynamicClient dynamic.Interface) {
	for {
		log.Println("Checking git repo for updates...")
		files, err := gitApp.getAppFiles(ctx)
		if err != nil {
			log.Printf("error pulling files from git\n%s", err)
			continue
		}

		for _, file := range files {
			objs, err := getResourcesFromYAMLOrJSON(bytes.NewReader(file))
			if err != nil {
				log.Printf("error unmarshalling file %s\n%s", string(file), err)
				continue
			}
			err = reconcile(ctx, dynamicClient, objs)
			if err != nil {
				log.Printf("error deploying to kubernetes\n%s", err)
				continue
			}
		}
		time.Sleep(30 * time.Second)
	}
}

func getResourcesFromYAMLOrJSON(f io.Reader) ([]unstructured.Unstructured, error) {

	decoder := yaml.NewYAMLOrJSONDecoder(f, 1024)

	var objs []unstructured.Unstructured
	for {
		var v unstructured.Unstructured
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		objs = append(objs, v)
	}
	return objs, nil
}
