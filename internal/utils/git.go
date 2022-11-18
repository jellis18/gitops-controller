package utils

import (
	"context"
	"errors"
	"io"

	"github.com/google/go-github/v48/github"
	"golang.org/x/oauth2"
)

type GitApp struct {
	Client         *github.Client
	Path           string
	RepoOwner      string
	RepoName       string
	repoURL        string
	TargetRevision string
}

func GetGithubClient(ctx context.Context, accessToken string) *github.Client {
	if accessToken == "" {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)

}

// Gets YAML files from github repo
func (g *GitApp) getAppFiles(ctx context.Context) ([][]byte, error) {
	var outputs [][]byte

	fileContent, directoryContent, _, err := g.Client.Repositories.GetContents(
		ctx,
		g.RepoOwner,
		g.RepoName,
		g.Path,
		&github.RepositoryContentGetOptions{Ref: g.TargetRevision})
	if err != nil {
		return nil, err
	}

	if fileContent != nil {

		content, err := fileContent.GetContent()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, []byte(content))
		return outputs, nil
	}

	if directoryContent != nil {
		for _, fileContent := range directoryContent {
			downloadedFile, _, err := g.Client.Repositories.DownloadContents(
				ctx,
				g.RepoOwner,
				g.RepoName,
				*fileContent.Path,
				&github.RepositoryContentGetOptions{Ref: g.TargetRevision})
			if err != nil {
				return nil, err
			}

			body, err := io.ReadAll(downloadedFile)
			if err != nil {
				return nil, err
			}

			outputs = append(outputs, body)

		}
		return outputs, nil
	}
	return nil, errors.New("uh oh")

}
