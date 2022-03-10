package semrel

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-semantic-release/semantic-release/v2/pkg/config"
)

func calculateChange(commits []*Commit, latestRelease *Release) *Change {
	change := &Change{}
	for _, commit := range commits {
		if latestRelease.SHA == commit.SHA {
			break
		}
		change.Major = change.Major || commit.Change.Major
		change.Minor = change.Minor || commit.Change.Minor
		change.Patch = change.Patch || commit.Change.Patch
	}
	return change
}

func applyChange(rawVersion string, rawChange *Change, allowInitialDevelopmentVersions bool, forceBumpPatchVersion bool, prerelease string) string {

	logger := log.New(os.Stderr, "[wtf mate]: ", 0)
	version := semver.MustParse(rawVersion)
	change := &Change{
		Major: rawChange.Major,
		Minor: rawChange.Minor,
		Patch: rawChange.Patch,
	}
	if !allowInitialDevelopmentVersions && version.Major() == 0 {
		change.Major = true
	}

	if allowInitialDevelopmentVersions && version.Major() == 0 && version.Minor() == 0 {
		change.Minor = true
	}
	if !change.Major && !change.Minor && !change.Patch {
		if forceBumpPatchVersion {
			change.Patch = true
		} else {
			return ""
		}
	}
	preRel := version.Prerelease()
	preRelVer := strings.Split(preRel, ".")
	preRelLabel := preRelVer[0]

	logger.Println("OKAY")
	logger.Println("wtf " + prerelease + " " + preRelLabel)
	var newVersion semver.Version

	logger.Println("OKAY")
	if preRelLabel == "" {

		logger.Println("IN IF")
		switch {
		case change.Major:
			newVersion = version.IncMajor()
		case change.Minor:
			newVersion = version.IncMinor()
		case change.Patch:
			newVersion = version.IncPatch()
		}
	} else {
		logger.Println("IN ELSE")
		newVersion = *version
	}

	logger.Println("HERE")
	logger.Println("prerelease: " + prerelease + " vs " + preRelVer[0])
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
	newVersion, _ = version.SetPrerelease(preRel)

	logger.Println("RETURNING")
	return newVersion.String()
}

func GetNewVersion(conf *config.Config, commits []*Commit, latestRelease *Release, prerelease string) string {
	return applyChange(latestRelease.Version, calculateChange(commits, latestRelease), conf.AllowInitialDevelopmentVersions, conf.ForceBumpPatchVersion, prerelease)
}
