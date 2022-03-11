package semrel

import (
	"log"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type releases []*Release

func (r releases) Len() int {
	return len(r)
}

func (r releases) Less(i, j int) bool {
	return semver.MustParse(r[j].Version).LessThan(semver.MustParse(r[i].Version))
}

func (r releases) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r releases) GetLatestRelease(vrange string, prerelease string) (*Release, error) {

	logger := log.New(os.Stderr, "[releases]: ", 0)
	if len(r) == 0 {
		return &Release{SHA: "", Version: "0.0.0"}, nil
	}

	sort.Sort(r)

	logger.Println("prerelease: " + prerelease)

	var lastRelease *Release
	for _, r := range r {
		logger.Println("Checking version: ", r.Version)
		if semver.MustParse(r.Version).Prerelease() == "" && lastRelease == nil {
			logger.Println("Setting last release: " + r.Version)
			lastRelease = r
			if prerelease == "" {
				break
			}
		}

		prereleaseParts := strings.Split(semver.MustParse(r.Version).Prerelease(), ".")

		mainVersionParts := strings.Split(r.Version, "-")

		if prereleaseParts[0] == prerelease {

			logger.Println("prereleaseParts[0]: " + prereleaseParts[0] + " : " + prereleaseParts[1] + " : " + r.Version)

			logger.Println("mainReleaseParts[0]: " + mainVersionParts[0] + " : " + r.Version)
			// If it is a beta release and the last production release is newer
			// just stop here and go with the last production release version.
			if lastRelease != nil && semver.MustParse(mainVersionParts[0]).LessThan(semver.MustParse(lastRelease.Version)) {
				break
			}

			if prerelease != "" {
				lastRelease = r
				break
			}
		}
	}

	if vrange == "" {
		if lastRelease != nil {
			return lastRelease, nil
		}
		return &Release{SHA: "", Version: "0.0.0"}, nil
	}

	constraint, err := semver.NewConstraint(vrange)
	if err != nil {
		return nil, err
	}
	for _, r := range r {
		if constraint.Check(semver.MustParse(r.Version)) {
			return r, nil
		}
	}

	nver, err := semver.NewVersion(vrange)
	if err != nil {
		return nil, err
	}

	splitPre := strings.SplitN(vrange, "-", 2)
	if len(splitPre) == 1 {
		return &Release{SHA: lastRelease.SHA, Version: nver.String()}, nil
	}

	npver, err := nver.SetPrerelease(splitPre[1])
	if err != nil {
		return nil, err
	}
	return &Release{SHA: lastRelease.SHA, Version: npver.String()}, nil
}

func GetLatestReleaseFromReleases(rawReleases []*Release, vrange string, prerelease string) (*Release, error) {
	return releases(rawReleases).GetLatestRelease(vrange, prerelease)
}
