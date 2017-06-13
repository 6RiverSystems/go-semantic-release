package semrel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type Repository struct {
	Owner  string
	Repo   string
	Ctx    context.Context
	Client *github.Client
}

func NewRepository(ctx context.Context, slug, token string) (*Repository, error) {
	if !strings.Contains(slug, "/") {
		return nil, errors.New("invalid slug")
	}
	repo := new(Repository)
	splited := strings.Split(slug, "/")
	repo.Owner = splited[0]
	repo.Repo = splited[1]
	repo.Ctx = ctx
	repo.Client = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))
	return repo, nil
}

func (repo *Repository) GetInfo() (string, bool, error) {
	r, _, err := repo.Client.Repositories.Get(repo.Ctx, repo.Owner, repo.Repo)
	if err != nil {
		return "", false, err
	}
	return r.GetDefaultBranch(), r.GetPrivate(), nil
}

func (repo *Repository) CreateRelease(change *Change, details *CurCommitDetails, newVersion *semver.Version) error {
	tag := fmt.Sprintf("v%s", newVersion.String())
	changelog := GetChangelog(change, details, newVersion)
	opts := &github.RepositoryRelease{
		TargetCommitish: &details.CurrentSHA,
		TagName:         &tag,
		Body:            &changelog,
	}
	_, _, err := repo.Client.Repositories.CreateRelease(repo.Ctx, repo.Owner, repo.Repo, opts)
	if err != nil {
		return err
	}
	return nil
}

func CalculatePrerelease(latestRelease *CurCommitDetails, lastVersion semver.Version) string {

	prerelease := ""

	switch latestRelease.CurrentBranch {
	// If branch is master -> no pre-release version
	case "master":
		return ""
	// If branch is develop -> beta release
	case "develop":
		prerelease = "beta"
	default:
		prerelease = latestRelease.CurrentBranch
	}

	// if else, prerelease = branch
	oldPrereleaseParts := strings.Split(lastVersion.Prerelease(), ".")

	if oldPrereleaseParts[0] != prerelease {
		return prerelease + ".1"
	} else {
		subver, err := strconv.Atoi(oldPrereleaseParts[1])

		if err != nil {
			// What now?
		}

		return prerelease + "." + strconv.Itoa(subver+1)
	}
}

func GetNewVersion(latestRelease *CurCommitDetails, lastTag *Tag, change *Change) *semver.Version {
	lastVersion := *lastTag.Version

	if lastVersion.Major() == 0 {
		change.Major = true
	}

	var newVersion semver.Version
	switch {
	case change.Major:
		newVersion = lastVersion.IncMajor()
		break
	case change.Minor:
		newVersion = lastVersion.IncMinor()
		break
	case change.Patch:
		newVersion = lastVersion.IncPatch()
		break
	default:
		return nil
	}

	prerelease := CalculatePrerelease(latestRelease, lastVersion)
	newVersion, err := newVersion.SetPrerelease(prerelease)

	if err != nil {
		// TODO: handle it
	}

	return &newVersion
}

var typeToText = map[string]string{
	"feat":     "Feature",
	"fix":      "Bug Fixes",
	"perf":     "Performance Improvements",
	"revert":   "Reverts",
	"docs":     "Documentation",
	"style":    "Styles",
	"refactor": "Code Refactoring",
	"test":     "Tests",
	"chore":    "Chores",
	"%%bc%%":   "Breaking Changes",
}

func getSortedKeys(m *map[string]string) []string {
	keys := make([]string, len(*m))
	i := 0
	for k := range *m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

func GetChangelog(change *Change, latestRelease *CurCommitDetails, newVersion *semver.Version) string {
	ret := fmt.Sprintf("## %s (%s)\n\n", newVersion.String(), time.Now().UTC().Format("2006-01-02"))

	for _, t := range getSortedKeys(&change.TypeScopeMap) {
		msg := change.TypeScopeMap[t]
		typeName, found := typeToText[t]
		if !found {
			typeName = t
		}
		ret += fmt.Sprintf("#### %s\n\n%s\n", typeName, msg)
	}
	return ret
}
