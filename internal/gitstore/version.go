package gitstore

import (
	"fmt"
	"regexp"
	"strconv"
)

type GitVersion struct {
	Major int
	Minor int
	Patch int
	Extra string
}

var MinimumGitVersion = GitVersion{Major: 2, Minor: 43, Patch: 0}

var gitVersionPattern = regexp.MustCompile(`^git version ([0-9]+)\.([0-9]+)\.([0-9]+)([^\n]*)?\n?$`)

func ParseGitVersion(output string) (GitVersion, error) {
	m := gitVersionPattern.FindStringSubmatch(output)
	if m == nil {
		return GitVersion{}, gitErr(CodeMalformedGitOutput, "version", "git version", "unexpected git version output", nil)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	v := GitVersion{Major: major, Minor: minor, Patch: patch, Extra: m[4]}
	if !v.AtLeast(MinimumGitVersion) {
		return GitVersion{}, gitErr(CodeUnsupportedGitVersion, "version", "git version", fmt.Sprintf("git %s is below supported minimum %s", v.String(), MinimumGitVersion.String()), nil)
	}
	return v, nil
}

func (v GitVersion) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

func (v GitVersion) AtLeast(min GitVersion) bool {
	if v.Major != min.Major {
		return v.Major > min.Major
	}
	if v.Minor != min.Minor {
		return v.Minor > min.Minor
	}
	return v.Patch >= min.Patch
}
