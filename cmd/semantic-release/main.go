package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/semantic-release/go-semantic-release"
	"github.com/semantic-release/go-semantic-release/condition"
	"github.com/semantic-release/go-semantic-release/update"
	"gopkg.in/src-d/go-git.v4"
)

var SRVERSION string

func errorHandler(logger *log.Logger) func(error) {
	return func(err error) {
		if err != nil {
			logger.Println(err)
			os.Exit(1)
		}
	}
}

func main() {
	token := flag.String("token", os.Getenv("GITHUB_TOKEN"), "github token")
	slug := flag.String("slug", os.Getenv("TRAVIS_REPO_SLUG"), "slug of the repository")
	ghr := flag.Bool("ghr", false, "create a .ghr file with the parameters for ghr")
	noci := flag.Bool("noci", false, "run semantic-release locally")
	dry := flag.Bool("dry", false, "do not create release")
	vFile := flag.Bool("vf", false, "create a .version file")
	showVersion := flag.Bool("version", false, "outputs the semantic-release version")
	updateFile := flag.String("update", "", "updates the version of a certain file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("semantic-release v%s", SRVERSION)
		return
	}

	logger := log.New(os.Stderr, "[semantic-release]: ", 0)
	exitIfError := errorHandler(logger)

	if val, ok := os.LookupEnv("GH_TOKEN"); *token == "" && ok {
		*token = val
	}

	if *token == "" {
		exitIfError(errors.New("github token missing"))
	}
	if *slug == "" {
		exitIfError(errors.New("slug missing"))
	}

	ghRepo, err := semrel.NewRepository(context.TODO(), *slug, *token)
	exitIfError(err)

	localRepo, err := git.PlainOpen(".")
	exitIfError(err)

	logger.Println("getting default branch...")
	defaultBranch, isPrivate, err := ghRepo.GetInfo()
	exitIfError(err)
	logger.Println("found default branch: " + defaultBranch)

	if !*noci {
		logger.Println("running CI condition...")
		exitIfError(condition.Travis(*token, defaultBranch, isPrivate))
	}

	logger.Println("getting latest release...")
	curCommitDetails, err := semrel.GetCurCommitDetails(localRepo)
	exitIfError(err)

	logger.Println("getting tags...")
	tags, err := semrel.GetTags(ghRepo)
	exitIfError(err)

	logger.Println("getting commits...")
	change, lastTag, err := semrel.ParseHistory(ghRepo, curCommitDetails, tags)
	logger.Println("found version: " + lastTag.Version.String())
	exitIfError(err)

	logger.Println("calculating new version...")
	newVer := semrel.GetNewVersion(curCommitDetails, lastTag, change)
	if newVer == nil {
		exitIfError(errors.New("no change"))
	}
	logger.Println("new version: " + newVer.String())

	if *dry {
		exitIfError(errors.New("DRY RUN: no release was created"))
	}

	logger.Println("creating release...")
	exitIfError(ghRepo.CreateRelease(change, curCommitDetails, newVer))

	if *ghr {
		exitIfError(ioutil.WriteFile(".ghr", []byte(fmt.Sprintf("-u %s -r %s v%s", ghRepo.Owner, ghRepo.Repo, newVer.String())), 0644))
	}

	if *vFile {
		exitIfError(ioutil.WriteFile(".version", []byte(newVer.String()), 0644))
	}

	if *updateFile != "" {
		exitIfError(update.Apply(*updateFile, newVer.String()))
	}

	logger.Println("done.")
}
