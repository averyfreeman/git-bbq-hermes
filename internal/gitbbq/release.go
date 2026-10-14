package gitbbq

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var releaseCommitPattern = regexp.MustCompile(`^([^[:space:]():!]+)(\([^)]*\))?(!)?:\s+.+$`)

type semanticVersion struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

// GitReleaseRecommendation is a read-only SemVer suggestion derived from
// conventional commit messages and the project's GitHabits policy.
type GitReleaseRecommendation struct {
	SchemaVersion   string `json:"schema_version"`
	CurrentTag      string `json:"current_tag"`
	Bump            string `json:"bump"`
	NextTag         string `json:"next_tag,omitempty"`
	CommitsAnalyzed int    `json:"commits_analyzed"`
	Reason          string `json:"reason"`
}

// RecommendRelease applies the configured conventional-commit-to-SemVer
// mapping. It never creates or pushes a tag.
func RecommendRelease(config GitHabits, existingTags, commitMessages []string) (GitReleaseRecommendation, error) {
	if err := ValidateGitHabits(config); err != nil {
		return GitReleaseRecommendation{}, err
	}
	if config.Versioning != "semver" {
		return GitReleaseRecommendation{}, fmt.Errorf("release recommendations require semver versioning")
	}
	currentTag, currentVersion, err := latestSemanticVersion(existingTags)
	if err != nil {
		return GitReleaseRecommendation{}, err
	}

	bumps := config.Release.CommitBumps
	recommendation := "none"
	for _, message := range commitMessages {
		if commitHasBreakingChange(message) {
			recommendation = higherReleaseBump(recommendation, bumps.BreakingChange)
			continue
		}
		subject := strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
		match := releaseCommitPattern.FindStringSubmatch(subject)
		if len(match) == 0 {
			recommendation = higherReleaseBump(recommendation, bumps.Other)
			continue
		}
		if match[3] == "!" {
			recommendation = higherReleaseBump(recommendation, bumps.BreakingChange)
			continue
		}
		switch match[1] {
		case "feat":
			recommendation = higherReleaseBump(recommendation, bumps.Feature)
		case "fix":
			recommendation = higherReleaseBump(recommendation, bumps.Fix)
		case "perf":
			recommendation = higherReleaseBump(recommendation, bumps.Performance)
		default:
			recommendation = higherReleaseBump(recommendation, bumps.Other)
		}
	}

	plan := GitReleaseRecommendation{
		SchemaVersion:   SchemaVersion,
		CurrentTag:      currentTag,
		Bump:            recommendation,
		CommitsAnalyzed: len(commitMessages),
	}
	if recommendation == "none" {
		plan.Reason = "commit policy does not require a release increment"
		return plan, nil
	}
	plan.NextTag = bumpSemanticVersion(currentVersion, recommendation)
	plan.Reason = "recommended by the configured conventional-commit release policy"
	return plan, nil
}

// PlanRepositoryRelease reads commit messages since a SemVer tag in the
// selected checkout and returns the policy-derived release recommendation.
func PlanRepositoryRelease(root string, config GitHabits, sinceTag string) (GitReleaseRecommendation, error) {
	if strings.TrimSpace(root) == "" {
		return GitReleaseRecommendation{}, fmt.Errorf("release planning requires a workspace")
	}
	tagOutput, err := exec.Command("git", "-C", root, "tag", "--list").Output()
	if err != nil {
		return GitReleaseRecommendation{}, fmt.Errorf("read Git tags for release planning: %w", err)
	}
	tags := strings.Fields(string(tagOutput))
	if strings.TrimSpace(sinceTag) == "" {
		sinceTag, _, err = latestSemanticVersion(tags)
		if err != nil {
			return GitReleaseRecommendation{}, err
		}
	} else {
		sinceTag = strings.TrimSpace(sinceTag)
		if !safeSemanticVersionTag(sinceTag) {
			return GitReleaseRecommendation{}, fmt.Errorf("release baseline is not a safe SemVer tag")
		}
		found := false
		for _, tag := range tags {
			if tag == sinceTag {
				found = true
				break
			}
		}
		if !found {
			return GitReleaseRecommendation{}, fmt.Errorf("release baseline tag %q does not exist in the selected checkout", sinceTag)
		}
	}
	output, err := exec.Command("git", "-C", root, "log", "--format=%B%x00", sinceTag+"..HEAD").Output()
	if err != nil {
		return GitReleaseRecommendation{}, fmt.Errorf("read commits since release baseline %q: %w", sinceTag, err)
	}
	var messages []string
	for _, message := range strings.Split(string(output), "\x00") {
		if message = strings.TrimSpace(message); message != "" {
			messages = append(messages, message)
		}
	}
	return RecommendRelease(config, []string{sinceTag}, messages)
}

func latestSemanticVersion(tags []string) (string, semanticVersion, error) {
	var latestTag string
	var latest semanticVersion
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !safeSemanticVersionTag(tag) {
			continue
		}
		version, err := parseSemanticVersion(tag)
		if err != nil {
			continue
		}
		if latestTag == "" || compareSemanticVersions(version, latest) > 0 {
			latestTag = tag
			latest = version
		}
	}
	if latestTag == "" {
		return "", semanticVersion{}, fmt.Errorf("release planning requires an existing SemVer baseline tag")
	}
	return latestTag, latest, nil
}

func commitHasBreakingChange(message string) bool {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(line, "BREAKING CHANGE:") || strings.HasPrefix(line, "BREAKING-CHANGE:") {
			return true
		}
	}
	return false
}

func higherReleaseBump(current, candidate string) string {
	rank := map[string]int{"none": 0, "patch": 1, "minor": 2, "major": 3}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func bumpSemanticVersion(current semanticVersion, bump string) string {
	if len(current.prerelease) > 0 && bump == "patch" {
		return fmt.Sprintf("v%d.%d.%d", current.major, current.minor, current.patch)
	}
	switch bump {
	case "major":
		current.major++
		current.minor = 0
		current.patch = 0
	case "minor":
		current.minor++
		current.patch = 0
	case "patch":
		current.patch++
	default:
		return ""
	}
	return fmt.Sprintf("v%d.%d.%d", current.major, current.minor, current.patch)
}

// ValidateTagProgression rejects a candidate that does not advance observed SemVer tags.
// Non-SemVer tags are ignored because repositories may contain non-release tags.
func ValidateTagProgression(existingTags []string, candidate string) error {
	candidateVersion, err := parseSemanticVersion(candidate)
	if err != nil {
		return err
	}
	for _, existing := range existingTags {
		existing = strings.TrimSpace(existing)
		if existing == "" || !safeSemanticVersionTag(existing) {
			continue
		}
		existingVersion, err := parseSemanticVersion(existing)
		if err != nil {
			continue
		}
		if compareSemanticVersions(candidateVersion, existingVersion) <= 0 {
			return fmt.Errorf("tag %q is not newer than existing tag %q", candidate, existing)
		}
	}
	return nil
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	value = strings.TrimSpace(value)
	if !safeSemanticVersionTag(value) {
		return semanticVersion{}, fmt.Errorf("tag is not a valid semver value: %q", value)
	}
	value = strings.TrimPrefix(value, "v")
	if buildIndex := strings.IndexByte(value, '+'); buildIndex >= 0 {
		value = value[:buildIndex]
	}
	prerelease := []string(nil)
	if prereleaseIndex := strings.IndexByte(value, '-'); prereleaseIndex >= 0 {
		prerelease = strings.Split(value[prereleaseIndex+1:], ".")
		value = value[:prereleaseIndex]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("tag is not a valid semver value: %q", value)
	}
	major, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return semanticVersion{}, fmt.Errorf("tag is not a valid semver value: %q", value)
	}
	minor, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return semanticVersion{}, fmt.Errorf("tag is not a valid semver value: %q", value)
	}
	patch, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return semanticVersion{}, fmt.Errorf("tag is not a valid semver value: %q", value)
	}
	return semanticVersion{major: major, minor: minor, patch: patch, prerelease: prerelease}, nil
}

func compareSemanticVersions(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) > 0 {
		return 1
	}
	if len(left.prerelease) > 0 && len(right.prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(left.prerelease) && index < len(right.prerelease); index++ {
		leftIdentifier := left.prerelease[index]
		rightIdentifier := right.prerelease[index]
		leftNumber, leftIsNumber := parsePrereleaseNumber(leftIdentifier)
		rightNumber, rightIsNumber := parsePrereleaseNumber(rightIdentifier)
		if leftIsNumber && rightIsNumber {
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
			continue
		}
		if leftIsNumber != rightIsNumber {
			if leftIsNumber {
				return -1
			}
			return 1
		}
		if leftIdentifier < rightIdentifier {
			return -1
		}
		if leftIdentifier > rightIdentifier {
			return 1
		}
	}
	if len(left.prerelease) < len(right.prerelease) {
		return -1
	}
	if len(left.prerelease) > len(right.prerelease) {
		return 1
	}
	return 0
}

func parsePrereleaseNumber(value string) (uint64, bool) {
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, false
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return number, err == nil
}
