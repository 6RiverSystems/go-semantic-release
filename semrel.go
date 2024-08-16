package semrel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/v48/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

var (
	commitPattern   = regexp.MustCompile(`^(\w*)(?:\((.*)\))?\: (.*)$`)
	breakingPattern = regexp.MustCompile("BREAKING CHANGES?")
)

type Change struct {
	Major, Minor, Patch, NoChange bool
}

type Commit struct {
	SHA     string
	Raw     []string
	Type    string
	Scope   string
	Message string
	Change  Change
}
type Commits []*Commit

type Release struct {
	SHA     string
	Version *semver.Version
}

type Releases []*Release

func (r Releases) Len() int {
	return len(r)
}

func (r Releases) Less(i, j int) bool {
	return r[j].Version.LessThan(r[i].Version)
}

func (r Releases) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

type Repository struct {
	Owner      string
	Repo       string
	Ctx        context.Context
	GQLClient  *githubv4.Client
	RESTClient *github.Client
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
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))
	repo.GQLClient = githubv4.NewClient(oauthClient)
	repo.RESTClient = github.NewClient(oauthClient)
	return repo, nil
}

type repoInfoQuery struct {
	Repository struct {
		DefaultBranchRef struct {
			Name githubv4.String
		}
		IsPrivate githubv4.Boolean
	} `graphql:"repository(owner: $owner, name: $name)"`
}

func (repoInfoQuery) vars(repo *Repository) map[string]any {
	return map[string]any{
		"owner": githubv4.String(repo.Owner),
		"name":  githubv4.String(repo.Repo),
	}
}

func (repo *Repository) GetInfo() (string, bool, error) {
	var q repoInfoQuery
	if err := repo.GQLClient.Query(repo.Ctx, &q, q.vars(repo)); err != nil {
		return "", false, err
	}
	return string(q.Repository.DefaultBranchRef.Name), bool(q.Repository.IsPrivate), nil
}

func parseCommit(commit commitOidAndMessage) *Commit {
	c := new(Commit)
	c.SHA = string(commit.Oid)
	c.Raw = strings.Split(string(commit.Message), "\n")
	found := commitPattern.FindAllStringSubmatch(c.Raw[0], -1)
	if len(found) < 1 {
		c.Change = Change{
			Patch: true,
		}
		return c
	}
	c.Type = strings.ToLower(found[0][1])
	c.Scope = found[0][2]
	c.Message = found[0][3]
	c.Change = Change{
		Major: breakingPattern.MatchString(string(commit.Message)),
		Minor: c.Type == "feat",
		Patch: c.Type == "fix",
	}
	return c
}

type commitOidAndMessage struct {
	Oid     githubv4.String
	Message githubv4.String
}

type getBranchHistoryQuery struct {
	Repository struct {
		Ref struct {
			Name   githubv4.String
			Target struct {
				Commit struct {
					History struct {
						Nodes    []commitOidAndMessage
						PageInfo forwardPageInfo
					} `graphql:"history(first: $perPage, after: $cursor)"`
				} `graphql:"... on Commit"`
			}
		} `graphql:"ref(qualifiedName: $branch)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

func (getBranchHistoryQuery) vars(repo *Repository, branch string, perPage int, cursor string) map[string]any {
	ret := map[string]any{
		"owner":   githubv4.String(repo.Owner),
		"name":    githubv4.String(repo.Repo),
		"branch":  githubv4.String(branch),
		"perPage": githubv4.Int(perPage),
	}
	if cursor == "" {
		ret["cursor"] = (*githubv4.String)(nil)
	} else {
		ret["cursor"] = githubv4.String(cursor)
	}
	return ret
}

func (repo *Repository) GetCommits(branch string) ([]*Commit, error) {
	var q getBranchHistoryQuery

	err := repo.GQLClient.Query(repo.Ctx, &q, q.vars(repo, branch, 100, ""))
	if err != nil {
		return nil, err
	}

	ret := make([]*Commit, len(q.Repository.Ref.Target.Commit.History.Nodes))

	for i, commit := range q.Repository.Ref.Target.Commit.History.Nodes {
		ret[i] = parseCommit(commit)
	}

	// TODO: should this retrieve more pages?

	return ret, nil
}

type forwardPageInfo struct {
	EndCursor   githubv4.String
	HasNextPage githubv4.Boolean
}
type listRefsQuery struct {
	Repository struct {
		Refs struct {
			Nodes []struct {
				Name   githubv4.String
				Target struct {
					Oid githubv4.String
				}
			}
			PageInfo forwardPageInfo
		} `graphql:"refs(refPrefix: \"refs/tags/\", first: $perPage, after: $cursor, orderBy:{field:TAG_COMMIT_DATE, direction:DESC})"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

func (listRefsQuery) vars(repo *Repository, perPage int, cursor string) map[string]any {
	return map[string]any{
		"owner":   githubv4.String(repo.Owner),
		"name":    githubv4.String(repo.Repo),
		"perPage": githubv4.Int(perPage),
		"cursor":  githubv4.String(cursor),
	}
}

// GetLatestRelease returns the latest release that matches the given version
// range or prerelease. If neither is set, it returns the latest non-prerelease.
// If verRange is unset but prerelease is set, it returns the newer of either
// the latest non-prerelease or the latest matching prerelease. If verRange is
// set but prerelease is unset, it returns the latest matching version for
// verRange. If both are set, strange things may happen. It will take the latest
// non-prerelease or matching pre-release, and use its SHA with the verRange
// version, resulting in a return value that may not actually exist in the repo.
func (repo *Repository) GetLatestRelease(verRange string, prerelease string) (*Release, error) {
	var lastRelease *Release
	var cursor string
	var verRangeConstraint *semver.Constraints
	if verRange != "" {
		var err error
		if verRangeConstraint, err = semver.NewConstraint(verRange); err != nil {
			return nil, err
		}
	}

REFS:
	for {
		// this will do a reverse chronological sort, which SHOULD be in semrel
		// order for at least the "main" and pre-release sequences we're hunting
		// for.
		var q listRefsQuery
		err := repo.GQLClient.Query(repo.Ctx, &q, q.vars(repo, 100, cursor))
		if err != nil {
			return nil, fmt.Errorf("failed github tags query: %w", err)
		}
		for _, r := range q.Repository.Refs.Nodes {
			version, err := semver.NewVersion(strings.TrimPrefix(string(r.Name), "refs/tags/"))
			if err != nil {
				continue
			}
			r := &Release{string(r.Target.Oid), version}
			log.Println("Checking version: ", r.Version.String())
			if r.Version.Prerelease() == "" {
				// this must be newer than anything with a matching pre-release, if one
				// was requested. As long as we aren't doing a version range special
				// case, we're done.
				if verRangeConstraint == nil {
					return r, nil
				}
				lastRelease = r
				// assume any pre-release that we might find later is older
				break REFS
			} else if verRangeConstraint != nil && verRangeConstraint.Check(r.Version) {
				// if verRange is set and matches this release, we are done
				return r, nil
			} else if rPreRel, _, _ := strings.Cut(r.Version.Prerelease(), "."); rPreRel == prerelease {
				// If the last production release was newer, we already would have
				// stopped, so we know this is newer.
				if prerelease != "" {
					if verRangeConstraint == nil {
						return r, nil
					}
					lastRelease = r
					break REFS
				}
			}
		}

		if !q.Repository.Refs.PageInfo.HasNextPage {
			break
		}
		cursor = string(q.Repository.Refs.PageInfo.EndCursor)
	}

	// pretend the latest release is from verRange, applying any pre-release tag it has
	if verRangeVer, err := semver.NewVersion(verRange); err != nil {
		return nil, fmt.Errorf("unable to parse maintained version %q as semver: %w", verRange, err)
	} else if _, verRangePre, vrHasPre := strings.Cut(verRange, "-"); !vrHasPre {
		// verRange is just a version, no pre-release segment. Attach its version to
		// the latest release's commit
		return &Release{lastRelease.SHA, verRangeVer}, nil
	} else if verWithPreRel, err := verRangeVer.SetPrerelease(verRangePre); err != nil {
		return nil, fmt.Errorf("failed to initialize pre-release for mainted version %q: %w", verRange, err)
	} else {
		// verRange is a version and a pre-release tag. Attach this combo to the
		// latest release's commit
		return &Release{lastRelease.SHA, &verWithPreRel}, nil
	}
}

func (repo *Repository) CreateRelease(commits []*Commit, latestRelease *Release, newVersion *semver.Version, branch string) error {
	tag := fmt.Sprintf("v%s", newVersion.String())
	changelog := GetChangelog(commits, latestRelease, newVersion)
	hasPrerelease := newVersion.Prerelease() != ""
	opts := &github.RepositoryRelease{
		TagName:         &tag,
		TargetCommitish: &branch,
		Body:            &changelog,
		Prerelease:      &hasPrerelease,
	}
	_, _, err := repo.RESTClient.Repositories.CreateRelease(repo.Ctx, repo.Owner, repo.Repo, opts)
	if err != nil {
		return err
	}
	return nil
}

func CalculateChange(commits []*Commit, latestRelease *Release) Change {
	var change Change
	numChanges := 0
	for _, commit := range commits {
		log.Printf("Examining commit %s: %#v\n", commit.SHA, commit.Change)

		if latestRelease.SHA == commit.SHA {
			change.NoChange = true
			break
		}
		numChanges++
		change.Major = change.Major || commit.Change.Major
		change.Minor = change.Minor || commit.Change.Minor
		change.Patch = change.Patch || commit.Change.Patch
	}
	// always apply at least a patch change if there was at least one new commit
	if numChanges > 0 {
		change.Patch = true
	}
	return change
}

func ApplyChange(latestVersion *semver.Version, prerelease string, change Change) *semver.Version {
	if !change.Major && !change.Minor && !change.Patch {
		if change.NoChange {
			return latestVersion
		}
		return nil
	}

	preRel := latestVersion.Prerelease()
	preRelVer := strings.Split(preRel, ".")
	preRelLabel := preRelVer[0]

	newVersion := *latestVersion
	if preRelLabel == "" {
		switch {
		case change.Major:
			newVersion = latestVersion.IncMajor()
		case change.Minor:
			newVersion = latestVersion.IncMinor()
		case change.Patch:
			newVersion = latestVersion.IncPatch()
		}
	}

	if prerelease != "" && preRelVer[0] != prerelease {
		preRel = prerelease + ".1"
	} else {
		if len(preRelVer) > 1 {
			idx, err := strconv.ParseInt(preRelVer[1], 10, 32)
			if err != nil {
				idx = 0
			}
			preRel = fmt.Sprintf("%s.%d", preRelVer[0], idx+1)
		} else {
			preRel += ".1"
		}
	}

	newVersion, _ = newVersion.SetPrerelease(preRel)
	return &newVersion
}

func GetNewVersion(commits []*Commit, latestRelease *Release, prerelease string) *semver.Version {
	newVersion := ApplyChange(latestRelease.Version, prerelease, CalculateChange(commits, latestRelease))

	return newVersion
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

func GetChangelog(commits []*Commit, latestRelease *Release, newVersion *semver.Version) string {
	ret := fmt.Sprintf("## %s (%s)\n\n", newVersion.String(), time.Now().UTC().Format("2006-01-02"))
	typeScopeMap := make(map[string]string)
	for _, commit := range commits {
		if latestRelease.SHA == commit.SHA {
			break
		}
		if commit.Change.Major {
			typeScopeMap["%%bc%%"] += fmt.Sprintf("%s\n```%s\n```\n", formatCommit(commit), strings.Join(commit.Raw[1:], "\n"))
			continue
		}
		if commit.Type == "" {
			continue
		}
		typeScopeMap[commit.Type] += formatCommit(commit)
	}
	for _, t := range getSortedKeys(&typeScopeMap) {
		msg := typeScopeMap[t]
		typeName, found := typeToText[t]
		if !found {
			typeName = t
		}
		ret += fmt.Sprintf("#### %s\n\n%s\n", typeName, msg)
	}
	return ret
}
