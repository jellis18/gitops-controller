package controllers

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/google/go-github/v48/github"
	gitopsv1 "github.com/jellis18/gitops-controller/api/v1"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type AppStateManager struct {
	client *github.Client
}

func getGithubClient(ctx context.Context, accessToken string) *github.Client {
	if accessToken == "" {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)

}

func NewAppStateManager(accessToken string) *AppStateManager {
	return &AppStateManager{client: getGithubClient(context.Background(), accessToken)}
}

// Gets unstructured objects from git repo
func (a *AppStateManager) getRepoObjs(ctx context.Context, app *gitopsv1.Application) ([]*unstructured.Unstructured, error) {

	// get repo information from app source
	repoOwner, repoName := getRepoOwnerAndNameFromSourceURL(app.Spec.Source.RepoURL)

	var targetObjs []*unstructured.Unstructured

	fileContent, directoryContent, _, err := a.client.Repositories.GetContents(
		ctx,
		repoOwner,
		repoName,
		app.Spec.Source.Path,
		&github.RepositoryContentGetOptions{Ref: app.Spec.Source.TargetRevision})
	if err != nil {
		return nil, err
	}

	if fileContent != nil {

		content, err := fileContent.GetContent()
		if err != nil {
			return nil, err
		}

		objs, err := getResourcesFromYAMLOrJSON(strings.NewReader(content))
		if err != nil {
			return nil, err
		}

		targetObjs = append(targetObjs, objs...)

		return targetObjs, nil
	}

	if directoryContent != nil {
		for _, fileContent := range directoryContent {
			downloadedFile, _, err := a.client.Repositories.DownloadContents(
				ctx,
				repoOwner,
				repoName,
				*fileContent.Path,
				&github.RepositoryContentGetOptions{Ref: app.Spec.Source.TargetRevision})
			if err != nil {
				return nil, err
			}

			objs, err := getResourcesFromYAMLOrJSON(downloadedFile)
			if err != nil {
				return nil, err
			}

			targetObjs = append(targetObjs, objs...)

		}
		return targetObjs, nil
	}
	return nil, errors.New("github path is empty")

}

func getRepoOwnerAndNameFromSourceURL(url string) (repoOwner, repoName string) {
	res := strings.Split(url, "github.com/")[1]
	repoOwner = strings.Split(res, "/")[0]
	repoName = strings.Split(strings.Split(res, ".git")[0], "/")[1]
	return repoOwner, repoName

}

func getResourcesFromYAMLOrJSON(f io.Reader) ([]*unstructured.Unstructured, error) {

	decoder := yaml.NewYAMLOrJSONDecoder(f, 1024)

	var objs []*unstructured.Unstructured
	for {
		var v unstructured.Unstructured
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		objs = append(objs, &v)
	}
	return objs, nil
}
