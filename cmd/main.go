package cmd

import (
	"context"
	"log"

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

	dynamicClient, err := utils.NewDynamicClient(false)
	if err != nil {
		log.Fatal(err)
	}

	//TODO: make bette use of channels for graceful shutdown
	forever := make(chan bool)
	go utils.SyncGitRepo(ctx, gitApp, dynamicClient)
	<-forever

}
