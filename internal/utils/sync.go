package utils

import (
	"context"
	"log"
	"time"

	"k8s.io/client-go/kubernetes"
)

func SyncGitRepo(ctx context.Context, gitApp *GitApp, clientset *kubernetes.Clientset) {
	for {
		log.Println("Checking git repo for updates...")
		files, err := gitApp.getAppFiles(ctx)
		if err != nil {
			log.Printf("error pulling files from git\n%s", err)
			break
		}

		for _, file := range files {
			_, err := deploy(ctx, clientset, file)
			if err != nil {
				log.Printf("error deploying to kubernetes\n%s", err)
				break
			}
		}
		time.Sleep(30 * time.Second)
	}
}
