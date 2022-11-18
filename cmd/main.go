package cmd

import (
	"context"
	"log"
	"time"

	"github.com/jellis18/gitops-controller/internal/utils"
)

func Run() {

	ctx := context.Background()

	//githubToken := os.Getenv("GITHUB_TOKEN")
	githubToken := "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"
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment varialbe is required")
	}

	githubClient := utils.GetGithubClient(ctx, githubToken)

	// TODO: don't hard code these values
	gitApp := &utils.GitApp{
		Client:         githubClient,
		RepoOwner:      "jellis18",
		RepoName:       "go-kubernetest-deploy",
		Path:           "app",
		TargetRevision: "HEAD",
	}

	clientset, err := utils.GetK8sClient(false)
	if err != nil {
		log.Fatal(err)
	}

	go utils.SyncGitRepo(ctx, gitApp, clientset)

	log.Println("On to other things...")
	time.Sleep(120 * time.Second)
}
