package semrel

import (
	"os/exec"
	"bytes"
	"strings"
	"log"
	"github.com/Masterminds/semver"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"regexp"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"fmt"
)

var commitPattern = regexp.MustCompile("^(\\w*)(?:\\((.*)\\))?\\: (.*)$")
var breakingPattern = regexp.MustCompile("BREAKING CHANGES?")


type Change struct {
	Major, Minor, Patch bool
	TypeScopeMap map[string]string
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
	LastTagSHA     	string
	LastTagVersion 	*semver.Version
	CurrentBranch  	string
	CurrentSHA	string
}

func GetCmdResult(name string, arg ...string) (string, error) {

	cmd := exec.Command(name, arg...)

	var outbuf, errbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	err := cmd.Start()
	if err != nil {
		log.Println(err)
		return "", err
	}
	err = cmd.Wait()

	if (err != nil) {
		log.Println(errbuf.String())
		log.Println(err)
		return "", err
	}

	return strings.TrimSpace(outbuf.String()), nil
}

func GetCurCommitDetails(repo *git.Repository) (*CurCommitDetails, error) {

	// Setup default values
	lastTagSha := ""
	version := &semver.Version{}

	lastTag, err := GetCmdResult("git", "describe", "--tags", "--abbrev=0", "HEAD")

	if err == nil {
		tagRef, err := repo.Reference(plumbing.ReferenceName("refs/tags/" + lastTag), true)

		if err == nil {
			tag, err := repo.TagObject(tagRef.Hash())

			if err != nil {
				//return nil, err
			}

			lastTagSha = tag.Target.String()

			version, err = semver.NewVersion(lastTag)
			if err != nil {
				return nil, err
			}

		} else {
			//return nil, err
		}

	} else {
		//return nil, err
	}


	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}

	return &CurCommitDetails{
		LastTagSHA:     lastTagSha,
		LastTagVersion: version,
		CurrentBranch:  headRef.Name().Short(),
		CurrentSHA:     headRef.Hash().String(),
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

func parseCommit(commit *object.Commit) *Commit {
	c := new(Commit)
	c.SHA = commit.Hash.String()
	c.Raw = strings.Split(commit.Message, "\n")
	log.Println("Examining:", c.SHA, c.Raw[0])
	found := commitPattern.FindAllStringSubmatch(c.Raw[0], -1)
	if len(found) < 1 {
		return c
	}
	c.Type = strings.ToLower(found[0][1])
	c.Scope = found[0][2]
	c.Message = found[0][3]
	c.Change = Change{
		Major: breakingPattern.MatchString(commit.Message),
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

func ParseCommitsSince(latestRelease *CurCommitDetails) (*Change, error) {

	repo, err := git.PlainOpen(".")

	if (err != nil) {
		return nil, err
	}

	opts := &git.LogOptions{
	}

	cIter, err := repo.Log(opts)

	if (err != nil) {
		return nil, err
	}

	change := Change {
		TypeScopeMap: make(map[string]string),
	}

	err = cIter.ForEach(func(c *object.Commit) error {
		change = CalculateChange(change, parseCommit(c), latestRelease)
		return nil
	})

	return &change, nil
}
