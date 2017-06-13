package semrel

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"gopkg.in/src-d/go-git.v4"
)

var commitPattern = regexp.MustCompile("^(\\w*)(?:\\((.*)\\))?\\: (.*)$")
var breakingPattern = regexp.MustCompile("BREAKING CHANGES?")

type Change struct {
	Major, Minor, Patch bool
	TypeScopeMap        map[string]string
}

type Commit struct {
	SHA     string
	Raw     []string
	Type    string
	Scope   string
	Message string
	Change  Change
}

type CurCommitDetails struct {
	CurrentBranch string
	CurrentSHA    string
}

type Tag struct {
	SHA     string
	Version *semver.Version
}

func GetCurCommitDetails(repo *git.Repository) (*CurCommitDetails, error) {

	headRef, err := repo.Head()

	if err != nil {
		return nil, err
	}

	return &CurCommitDetails{
		CurrentBranch: headRef.Name().Short(),
		CurrentSHA:    headRef.Hash().String(),
	}, nil
}

func trimSHA(sha string) string {
	if len(sha) < 9 {
		return sha
	}
	return sha[:8]
}

func formatCommit(c *Commit) string {
	ret := "* "
	if c.Scope != "" {
		ret += fmt.Sprintf("**%s:** ", c.Scope)
	}
	ret += fmt.Sprintf("%s (%s)\n", c.Message, trimSHA(c.SHA))
	return ret
}

func parseCommit(commit *github.RepositoryCommit) *Commit {
	c := new(Commit)
	c.SHA = *commit.SHA
	c.Raw = strings.Split(*commit.Commit.Message, "\n")
	log.Println("Examining:", c.SHA, c.Raw[0])
	found := commitPattern.FindAllStringSubmatch(c.Raw[0], -1)
	if len(found) < 1 {
		return c
	}
	c.Type = strings.ToLower(found[0][1])
	c.Scope = found[0][2]
	c.Message = found[0][3]
	c.Change = Change{
		Major: breakingPattern.MatchString(*commit.Commit.Message),
		Minor: c.Type == "feat",
		Patch: c.Type == "fix",
	}

	return c
}

func CalculateChange(change Change, commit *Commit, latestRelease *CurCommitDetails) Change {
	change.Major = change.Major || commit.Change.Major
	change.Minor = change.Minor || commit.Change.Minor
	change.Patch = change.Patch || commit.Change.Patch

	if commit.Change.Major {
		change.TypeScopeMap["%%bc%%"] += fmt.Sprintf("%s\n```%s\n```\n", formatCommit(commit), strings.Join(commit.Raw[1:], "\n"))
	} else if commit.Type == "" {
	} else {
		change.TypeScopeMap[commit.Type] += formatCommit(commit)
	}

	return change
}

func ParseHistory(ghRepo *Repository, latestRelease *CurCommitDetails, tags []*Tag) (*Change, *Tag, error) {

	//version := &semver.Version{}
	//version, err = semver.NewVersion(lastTagName)

	opts := &github.CommitsListOptions{
		SHA: latestRelease.CurrentSHA,
	}

	var lastTag *Tag

	var allCommits []*github.RepositoryCommit

	for {
		commits, resp, err :=
			ghRepo.Client.Repositories.ListCommits(ghRepo.Ctx, ghRepo.Owner, ghRepo.Repo, opts)

		if err != nil {
			return nil, nil, err
		}

		stopIdx := len(commits)

		for i, commit := range commits {

			for _, tag := range tags {
				if *commit.SHA == tag.SHA {
					log.Println("Found last tag: " + tag.Version.String())
					lastTag = tag
					stopIdx = i + 1
					break
				}
			}

			if lastTag != nil {
				break
			}
		}

		if lastTag == nil {
			log.Println("No last tag found, need to make one up")
		}

		allCommits = append(allCommits, commits[:stopIdx]...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	change := Change{
		TypeScopeMap: make(map[string]string),
	}

	for _, c := range allCommits {
		change = CalculateChange(change, parseCommit(c), latestRelease)
	}

	return &change, lastTag, nil
}

func GetTags(ghRepo *Repository) ([]*Tag, error) {

	opts := &github.ReferenceListOptions{
		Type: "tags",
	}

	var allRefs []*github.Reference

	for {
		refs, resp, err :=
			ghRepo.Client.Git.ListRefs(ghRepo.Ctx, ghRepo.Owner, ghRepo.Repo, opts)

		if err != nil {
			return nil, err
		}

		allRefs = append(allRefs, refs...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	var tags []*Tag

	for _, ref := range allRefs {

		versionStr := strings.TrimPrefix(*ref.Ref, "refs/tags/")

		version, err := semver.NewVersion(versionStr)

		if err == nil {
			tags = append(tags, &Tag{
				SHA:     *ref.Object.SHA,
				Version: version,
			})
		}

	}

	return tags, nil
}
