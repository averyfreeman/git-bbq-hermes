package gitbbq

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// LegacyScaffoldFilename is the configuration filename recognized by the
	// AI Software Architect migration reader.
	LegacyScaffoldFilename = ".adr-scaffold.yaml"
	// LegacyArchitectDir is the directory recognized for legacy architect data.
	LegacyArchitectDir     = ".ai-architect"
	legacyDecisionLimit    = 200
	legacyDecisionMaxBytes = 500_000
	legacyInventoryLimit   = 10_000
)

var (
	legacyDecisionIDPattern       = regexp.MustCompile(`^ADR-[0-9]{3}$`)
	legacyDecisionFilenamePattern = regexp.MustCompile(`^(ADR-[0-9]{3})(?:-[a-z0-9]+(?:-[a-z0-9]+)*)?\.md$`)
	legacyOptionIDPattern         = regexp.MustCompile(`^OPT-[0-9]{3}$`)
	legacyDatePattern             = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// MigrationAssessment describes the read-only compatibility check and proposed
// changes for a legacy project migration.
type MigrationAssessment struct {
	Mode              string   `json:"mode"`
	Root              string   `json:"root"`
	Detected          bool     `json:"detected"`
	Compatible        bool     `json:"compatible"`
	Legacy            []string `json:"legacy"`
	Proposed          []string `json:"proposed"`
	Conflicts         []string `json:"conflicts"`
	Blockers          []string `json:"blockers"`
	RequiredApprovals []string `json:"required_approvals,omitempty"`
	MigratedADRs      int      `json:"migrated_adrs"`
	Warnings          []string `json:"warnings,omitempty"`
	ReadOnly          bool     `json:"read_only"`
}

// MigrationResult reports files created, skipped, or archived by an approved
// legacy migration.
type MigrationResult struct {
	Root         string   `json:"root"`
	Created      []string `json:"created"`
	Skipped      []string `json:"skipped"`
	Archived     []string `json:"archived"`
	MigratedADRs int      `json:"migrated_adrs"`
	Warnings     []string `json:"warnings,omitempty"`
}

type legacyScaffoldMetadata struct {
	Version            int                  `yaml:"version"`
	Language           string               `yaml:"language"`
	Features           legacyFeatureToggles `yaml:"features"`
	Paths              legacyScaffoldPaths  `yaml:"paths"`
	PersistentDefaults map[string]bool      `yaml:"persistent_defaults,omitempty"`
}

type legacyFeatureToggles struct {
	Agents             bool `yaml:"agents"`
	Skills             bool `yaml:"skills"`
	Claude             bool `yaml:"claude"`
	ADR                bool `yaml:"adr"`
	Context            bool `yaml:"context"`
	Contract           bool `yaml:"contract"`
	ImplementationPlan bool `yaml:"implementation_plan"`
	Pages              bool `yaml:"pages"`
	Memory             bool `yaml:"memory"`
}

type legacyScaffoldPaths struct {
	Architect string `yaml:"architect"`
	Decisions string `yaml:"decisions"`
	DocsSite  string `yaml:"docs_site"`
}

type legacyContextMetadata struct {
	SchemaVersion string `yaml:"schema_version"`
	Language      string `yaml:"language"`
	Motivation    struct {
		WhyBuilding     string `yaml:"why_building"`
		Problem         string `yaml:"problem"`
		Audience        string `yaml:"audience"`
		Alternatives    string `yaml:"alternatives"`
		WhyThisSolution string `yaml:"why_this_solution"`
		Source          string `yaml:"source,omitempty"`
	} `yaml:"motivation"`
	Warnings []string `yaml:"warnings,omitempty"`
}

type legacyDecisionArtifact struct {
	SchemaVersion string         `yaml:"schema_version"`
	Revision      int            `yaml:"revision"`
	Decision      legacyDecision `yaml:"decision"`
}

type legacyDecision struct {
	ID                   string                 `yaml:"id"`
	Title                string                 `yaml:"title"`
	Date                 string                 `yaml:"date"`
	Status               string                 `yaml:"status"`
	Motivation           legacyMotivation       `yaml:"motivation"`
	Context              string                 `yaml:"context"`
	Drivers              []string               `yaml:"drivers"`
	ConsideredOptionIDs  []string               `yaml:"considered_option_ids"`
	SelectedOptionID     *string                `yaml:"selected_option_id"`
	Decision             string                 `yaml:"decision"`
	PositiveConsequences []string               `yaml:"positive_consequences"`
	NegativeConsequences []string               `yaml:"negative_consequences"`
	Assumptions          []string               `yaml:"assumptions"`
	ValidationCriteria   []string               `yaml:"validation_criteria"`
	Supersedes           []string               `yaml:"supersedes"`
	DecisionMatrix       []legacyDecisionOption `yaml:"decision_matrix"`
	ActionContract       legacyActionContract   `yaml:"action_contract"`
}

type legacyMotivation struct {
	WhyBuilding     string `yaml:"why_building"`
	Problem         string `yaml:"problem"`
	Audience        string `yaml:"audience"`
	Alternatives    string `yaml:"alternatives"`
	WhyThisSolution string `yaml:"why_this_solution"`
	Source          string `yaml:"source,omitempty"`
}

type legacyDecisionOption struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	FitScore  int      `yaml:"fit_score"`
	Benefits  []string `yaml:"benefits"`
	Drawbacks []string `yaml:"drawbacks"`
	Risks     []string `yaml:"risks"`
	Evidence  []string `yaml:"evidence"`
	Outcome   string   `yaml:"outcome"`
}

type legacyActionContract struct {
	GoodOutcomes     []string `yaml:"good_outcomes"`
	BadOutcomes      []string `yaml:"bad_outcomes"`
	CorrectExample   string   `yaml:"correct_example"`
	IncorrectExample string   `yaml:"incorrect_example"`
}

type plannedMigrationADR struct {
	Relative string
	Content  []byte
}

type legacyMigrationPlan struct {
	assessment       MigrationAssessment
	language         string
	problem          string
	adrs             []plannedMigrationADR
	legacyGitHabits  []byte
	archiveGitHabits bool
}

// AssessMigration inventories and validates a legacy project without writing.
// The assessment separates preservable existing files from blockers that must
// be resolved before an approved migration can safely proceed.
func AssessMigration(root string) (MigrationAssessment, error) {
	plan, err := buildMigrationPlan(root)
	if err != nil {
		return MigrationAssessment{}, err
	}
	return plan.assessment, nil
}

// Migrate applies a reviewed migration while retaining every legacy source.
// Replacing an incompatible legacy Git habits file requires the additional
// archiveLegacyGitHabits approval.
func Migrate(root string, archiveLegacyGitHabits bool) (MigrationResult, error) {
	plan, err := buildMigrationPlan(root)
	if err != nil {
		return MigrationResult{}, err
	}
	assessment := plan.assessment
	if !assessment.Detected {
		return MigrationResult{}, fmt.Errorf("no legacy AI Software Architect scaffold detected")
	}
	if !assessment.Compatible {
		return MigrationResult{}, fmt.Errorf("migration is blocked: %s", strings.Join(assessment.Blockers, "; "))
	}
	if plan.archiveGitHabits && !archiveLegacyGitHabits {
		return MigrationResult{}, fmt.Errorf("%s is incompatible; review the assessment and pass --archive-legacy-githabits to preserve and replace it", GitHabitsFilename)
	}

	result := MigrationResult{
		Root:     assessment.Root,
		Created:  []string{},
		Skipped:  []string{},
		Archived: []string{},
		Warnings: []string{"legacy .adr-scaffold.yaml and .ai-architect files were preserved; review migrated ADRs before accepting them"},
	}
	archiveRelative := filepath.ToSlash(filepath.Join(".gitbbq", "migration", "legacy", GitHabitsFilename))
	if plan.archiveGitHabits {
		if _, err := os.Lstat(filepath.Join(assessment.Root, filepath.FromSlash(archiveRelative))); err == nil {
			return MigrationResult{}, fmt.Errorf("migration archive target already exists: %s", archiveRelative)
		} else if !os.IsNotExist(err) {
			return MigrationResult{}, err
		}
		if _, err := writeGenerated(assessment.Root, archiveRelative, plan.legacyGitHabits, false); err != nil {
			return MigrationResult{}, fmt.Errorf("archive legacy %s: %w", GitHabitsFilename, err)
		}
		result.Archived = append(result.Archived, archiveRelative)
	}

	scaffold, err := ScaffoldProject(assessment.Root, ScaffoldOptions{
		ProjectName: filepath.Base(assessment.Root),
		Problem:     plan.problem,
		Languages:   []string{plan.language},
		Profile:     "guided",
		AllowHere:   true,
	})
	if err != nil {
		return MigrationResult{}, fmt.Errorf("scaffold migrated project: %w", err)
	}
	result.Created = append(result.Created, scaffold.Created...)
	result.Skipped = append(result.Skipped, scaffold.Skipped...)
	if plan.archiveGitHabits {
		config, err := GitHabitsForProfile("guided")
		if err != nil {
			return MigrationResult{}, err
		}
		data, err := marshalYAML(config)
		if err != nil {
			return MigrationResult{}, err
		}
		created, err := writeGenerated(assessment.Root, GitHabitsFilename, data, true)
		if err != nil {
			return MigrationResult{}, fmt.Errorf("write replacement %s: %w", GitHabitsFilename, err)
		}
		if created {
			result.Created = append(result.Created, GitHabitsFilename)
			result.Skipped = removeString(result.Skipped, GitHabitsFilename)
		}
	}
	for _, planned := range plan.adrs {
		created, err := writeGenerated(assessment.Root, planned.Relative, planned.Content, false)
		if err != nil {
			return MigrationResult{}, fmt.Errorf("write migrated ADR %s: %w", planned.Relative, err)
		}
		if !created {
			return MigrationResult{}, fmt.Errorf("migration target became occupied: %s", planned.Relative)
		}
		result.Created = append(result.Created, planned.Relative)
		result.MigratedADRs++
	}
	projection, projectionCreated, err := projectWithOptions(assessment.Root, false)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("write migration projections: %w", err)
	}
	result.Created = append(result.Created, projectionCreated...)
	for _, relative := range projection.Paths {
		if !hasPath(projectionCreated, relative) {
			result.Skipped = append(result.Skipped, relative)
		}
	}
	if err := recordGeneratedOwnership(assessment.Root, result.Created); err != nil {
		return MigrationResult{}, fmt.Errorf("record migration ownership: %w", err)
	}
	if err := ValidateProject(assessment.Root); err != nil {
		return MigrationResult{}, fmt.Errorf("validate migrated project: %w", err)
	}
	sort.Strings(result.Created)
	sort.Strings(result.Skipped)
	sort.Strings(result.Archived)
	return result, nil
}

func buildMigrationPlan(root string) (legacyMigrationPlan, error) {
	root = filepath.Clean(root)
	if root == "." {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return legacyMigrationPlan{}, err
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		return legacyMigrationPlan{}, err
	}
	if !info.IsDir() {
		return legacyMigrationPlan{}, fmt.Errorf("migration root is not a directory: %s", root)
	}
	existingSet := make(map[string]bool)
	_, metadataErr := os.Lstat(filepath.Join(root, LegacyScaffoldFilename))
	if metadataErr == nil {
		existingSet[LegacyScaffoldFilename] = true
	}
	architectPath := filepath.Join(root, LegacyArchitectDir)
	architectInfo, architectErr := os.Lstat(architectPath)
	legacy, inventoryErr := inventoryLegacyPaths(root, metadataErr == nil, architectInfo, architectErr)
	if inventoryErr != nil {
		// Preserve a useful partial inventory and block apply below.
	}

	assessment := MigrationAssessment{
		Mode:      "migration-assessment",
		Root:      root,
		Detected:  metadataErr == nil || architectErr == nil,
		Legacy:    legacy,
		Conflicts: []string{},
		Blockers:  []string{},
		Warnings:  []string{"migration is read-only until approved; legacy files remain in place"},
		ReadOnly:  true,
	}
	if !assessment.Detected {
		assessment.Warnings = append(assessment.Warnings, "no legacy AI Software Architect scaffold was detected")
		return legacyMigrationPlan{assessment: assessment}, nil
	}
	if metadataErr != nil && !os.IsNotExist(metadataErr) {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf("inspect %s: %v", LegacyScaffoldFilename, metadataErr))
	}
	if inventoryErr != nil {
		assessment.Blockers = append(assessment.Blockers, inventoryErr.Error())
	}
	if architectErr != nil && !os.IsNotExist(architectErr) {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf("inspect %s: %v", LegacyArchitectDir, architectErr))
	} else if architectErr == nil && (architectInfo.Mode()&os.ModeSymlink != 0 || !architectInfo.IsDir()) {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf("%s must be a regular directory", LegacyArchitectDir))
	} else if os.IsNotExist(architectErr) {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf("legacy directory %s is missing", LegacyArchitectDir))
	}
	if metadataErr != nil && os.IsNotExist(metadataErr) {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf("legacy metadata %s is missing", LegacyScaffoldFilename))
	}

	plan := legacyMigrationPlan{assessment: assessment}
	if len(assessment.Blockers) == 0 {
		plan.language, plan.problem, err = legacyProjectInputs(root)
		if err != nil {
			plan.assessment.Blockers = append(plan.assessment.Blockers, err.Error())
		} else {
			plan.adrs, err = planLegacyADRs(root)
			if err != nil {
				plan.assessment.Blockers = append(plan.assessment.Blockers, err.Error())
			}
		}
	}
	if plan.assessment.Detected {
		plan.assessment.Warnings = append(plan.assessment.Warnings, "only the validated language and problem fields plus compatible decision title, context, decision, rationale, and status are converted; legacy files remain unchanged")
	}

	proposed := append([]string(nil), projectPaths(root)...)
	proposed = append(proposed, SessionIgnorePath)
	if plan.language != "" {
		proposed = append(proposed, filepath.ToSlash(filepath.Join(".agents", "skills", plan.language, "SKILL.md")))
	}
	for _, adr := range plan.adrs {
		proposed = append(proposed, adr.Relative)
	}
	for _, relative := range proposed {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			existingSet[relative] = true
		} else if !os.IsNotExist(err) {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("inspect target %s: %v", relative, err))
		}
		if existingSet[relative] {
			plan.assessment.Conflicts = append(plan.assessment.Conflicts, relative)
		}
	}
	sort.Strings(plan.assessment.Conflicts)
	plan.assessment.Proposed = uniqueSorted(proposed)
	plan.assessment.MigratedADRs = len(plan.adrs)
	for _, relative := range plan.assessment.Proposed {
		if _, err := ensureSafeGeneratedTarget(root, relative); err != nil {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("target %s is unsafe: %v", relative, err))
		}
	}

	if existingSet[ManifestFilename] {
		plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("%s already exists; migration will not replace it", ManifestFilename))
	}
	for _, projectionPath := range []string{ContractFilename, ImplementationPlanFilename} {
		if existingSet[projectionPath] {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("%s already exists and will not be overwritten", projectionPath))
		}
	}
	if existingSet[ADRIndexFilename] {
		plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("%s already exists and will not be overwritten", ADRIndexFilename))
	}
	for _, adr := range plan.adrs {
		if existingSet[adr.Relative] {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("migration target already exists: %s", adr.Relative))
		}
	}
	if ledgerInfo, statErr := os.Lstat(filepath.Join(root, OwnershipFilename)); statErr == nil {
		if ledgerInfo.Mode()&os.ModeSymlink != 0 || !ledgerInfo.Mode().IsRegular() {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("%s is not a regular file", OwnershipFilename))
		} else if _, err := readOwnershipLedger(root); err != nil {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("existing %s is invalid: %v", OwnershipFilename, err))
		}
	} else if !os.IsNotExist(statErr) {
		plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("inspect %s: %v", OwnershipFilename, statErr))
	}
	expectedDependency := DefaultManifest(filepath.Base(root)).Matt
	if _, statErr := os.Lstat(filepath.Join(root, MattDependencyMetadataPath)); statErr == nil {
		if err := ensureArtifactParents(root, MattDependencyMetadataPath); err != nil {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("existing %s is unsafe", MattDependencyMetadataPath))
		}
		var dependency MattDependency
		if err := readYAML(filepath.Join(root, MattDependencyMetadataPath), &dependency); err != nil || dependency != expectedDependency {
			plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("existing %s conflicts with the Git BBQ dependency pin", MattDependencyMetadataPath))
		}
	} else if !os.IsNotExist(statErr) {
		plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("inspect %s: %v", MattDependencyMetadataPath, statErr))
	}

	if data, readErr := readMigrationGitHabits(root); readErr == nil {
		plan.legacyGitHabits = data
		if _, validErr := ReadGitHabits(root); validErr != nil {
			plan.archiveGitHabits = true
			plan.assessment.RequiredApprovals = append(plan.assessment.RequiredApprovals, "archive and replace the incompatible .githabits.yaml")
			plan.assessment.Proposed = append(plan.assessment.Proposed, filepath.ToSlash(filepath.Join(".gitbbq", "migration", "legacy", GitHabitsFilename)))
			plan.assessment.Proposed = uniqueSorted(plan.assessment.Proposed)
			archiveTarget := filepath.Join(root, ".gitbbq", "migration", "legacy", GitHabitsFilename)
			if _, err := os.Lstat(archiveTarget); err == nil {
				plan.assessment.Blockers = append(plan.assessment.Blockers, "legacy Git habits archive target already exists")
			} else if !os.IsNotExist(err) {
				plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("inspect legacy archive target: %v", err))
			}
			archiveRelative := filepath.ToSlash(filepath.Join(".gitbbq", "migration", "legacy", GitHabitsFilename))
			if _, err := ensureSafeGeneratedTarget(root, archiveRelative); err != nil {
				plan.assessment.Blockers = append(plan.assessment.Blockers, fmt.Sprintf("legacy archive target is unsafe: %v", err))
			}
			if err := rejectLegacySecrets(GitHabitsFilename, plan.legacyGitHabits); err != nil {
				plan.assessment.Blockers = append(plan.assessment.Blockers, err.Error())
			}
		}
	} else if !os.IsNotExist(readErr) {
		plan.assessment.Blockers = append(plan.assessment.Blockers, readErr.Error())
	}

	if err := scanArchitectureSecretsWithGitHabits(root, false, !plan.archiveGitHabits); err != nil {
		plan.assessment.Blockers = append(plan.assessment.Blockers, err.Error())
	}
	if len(plan.assessment.Blockers) == 0 {
		plan.assessment.Compatible = true
	}
	return plan, nil
}

func legacyProjectInputs(root string) (string, string, error) {
	configData, err := readLegacyArtifact(root, LegacyScaffoldFilename, legacyDecisionMaxBytes)
	if err != nil {
		return "", "", fmt.Errorf("read legacy metadata %s: %w", LegacyScaffoldFilename, err)
	}
	if err := rejectLegacySecrets(LegacyScaffoldFilename, configData); err != nil {
		return "", "", err
	}
	var metadata legacyScaffoldMetadata
	if err := decodeLegacyYAML(configData, &metadata); err != nil {
		return "", "", fmt.Errorf("parse legacy metadata %s: %w", LegacyScaffoldFilename, err)
	}
	if metadata.Version != 1 {
		return "", "", fmt.Errorf("legacy scaffold version %d is not supported; expected version 1", metadata.Version)
	}
	language := normalizeLanguage(metadata.Language)
	if language == "" || language == "auto" {
		return "", "", fmt.Errorf("legacy scaffold language is missing or automatic; choose a supported language before migration")
	}
	if !supportedLanguage(language) {
		return "", "", fmt.Errorf("legacy scaffold language %q is not supported by Git BBQ", language)
	}
	if metadata.Paths.Architect != "" && metadata.Paths.Architect != LegacyArchitectDir {
		return "", "", fmt.Errorf("legacy architect path %q is not supported; expected %q", metadata.Paths.Architect, LegacyArchitectDir)
	}
	legacyDecisionsDir := filepath.ToSlash(filepath.Join(LegacyArchitectDir, "decisions"))
	if metadata.Paths.Decisions != "" && metadata.Paths.Decisions != legacyDecisionsDir {
		return "", "", fmt.Errorf("legacy decision path %q is not supported; expected %q", metadata.Paths.Decisions, legacyDecisionsDir)
	}

	problem := "Migrate the existing AI Software Architect project to Git BBQ."
	contextPath := filepath.ToSlash(filepath.Join(LegacyArchitectDir, "project-context.md"))
	contextData, err := readLegacyArtifact(root, contextPath, legacyDecisionMaxBytes)
	if os.IsNotExist(err) {
		return language, problem, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read legacy project context %s: %w", contextPath, err)
	}
	if err := rejectLegacySecrets(contextPath, contextData); err != nil {
		return "", "", err
	}
	frontmatter, _, err := splitLegacyFrontmatter(string(contextData))
	if err != nil {
		return "", "", fmt.Errorf("parse legacy project context %s: %w", contextPath, err)
	}
	var context legacyContextMetadata
	if err := decodeLegacyYAML(frontmatter, &context); err != nil {
		return "", "", fmt.Errorf("parse legacy project context metadata: %w", err)
	}
	if context.SchemaVersion != "" && context.SchemaVersion != "1.0.0" && context.SchemaVersion != "1.1.0" {
		return "", "", fmt.Errorf("legacy project context schema %q is not supported", context.SchemaVersion)
	}
	if context.Language != "" && normalizeLanguage(context.Language) != language {
		return "", "", fmt.Errorf("legacy scaffold and project context language fields disagree")
	}
	if value := strings.TrimSpace(context.Motivation.Problem); value != "" {
		problem = value
	}
	return language, problem, nil
}

func planLegacyADRs(root string) ([]plannedMigrationADR, error) {
	directory := filepath.Join(root, LegacyArchitectDir, "decisions")
	if err := ensureLegacyParents(root, filepath.ToSlash(filepath.Join(LegacyArchitectDir, "decisions", "placeholder.md"))); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return []plannedMigrationADR{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect legacy decision directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("legacy decision path must be a regular directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list legacy decisions: %w", err)
	}
	filenames := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			filenames = append(filenames, entry.Name())
		}
	}
	sort.Strings(filenames)
	if len(filenames) > legacyDecisionLimit {
		return nil, fmt.Errorf("legacy decision count exceeds the migration limit of %d", legacyDecisionLimit)
	}

	decisions := make([]legacyDecisionArtifact, 0, len(filenames))
	decisionNumbers := make(map[string]bool, len(filenames))
	for _, filename := range filenames {
		match := legacyDecisionFilenamePattern.FindStringSubmatch(filename)
		if match == nil {
			return nil, fmt.Errorf("legacy decision filename is not canonical: %s", filename)
		}
		relative := filepath.ToSlash(filepath.Join(LegacyArchitectDir, "decisions", filename))
		data, err := readLegacyArtifact(root, relative, legacyDecisionMaxBytes)
		if err != nil {
			return nil, fmt.Errorf("read legacy decision %s: %w", relative, err)
		}
		if err := rejectLegacySecrets(relative, data); err != nil {
			return nil, err
		}
		frontmatter, _, err := splitLegacyFrontmatter(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse legacy decision %s: %w", relative, err)
		}
		var artifact legacyDecisionArtifact
		if err := decodeLegacyYAML(frontmatter, &artifact); err != nil {
			return nil, fmt.Errorf("parse legacy decision metadata %s: %w", relative, err)
		}
		if err := validateLegacyDecision(match[1], artifact); err != nil {
			return nil, fmt.Errorf("legacy decision %s: %w", relative, err)
		}
		if decisionNumbers[artifact.Decision.ID] {
			return nil, fmt.Errorf("legacy decision id is duplicated: %s", artifact.Decision.ID)
		}
		decisionNumbers[artifact.Decision.ID] = true
		decisions = append(decisions, artifact)
	}

	number, err := nextADRNumber(root)
	if err != nil {
		return nil, err
	}
	planned := make([]plannedMigrationADR, 0, len(decisions))
	for _, artifact := range decisions {
		decision := artifact.Decision
		why := strings.TrimSpace(decision.Motivation.WhyThisSolution)
		if why == "" {
			why = "Preserve the validated legacy decision while adopting the Matt-native ADR path."
		}
		status := migratedADRStatus(decision.Status)
		input := ADRInput{Title: strings.TrimSpace(decision.Title), Context: strings.TrimSpace(decision.Context), Decision: strings.TrimSpace(decision.Decision), Why: why, Status: status}
		slug := slugify(input.Title)
		if slug == "" || number > 9999 {
			return nil, fmt.Errorf("legacy decision %s cannot be assigned a valid Git BBQ ADR filename", decision.ID)
		}
		filename := fmt.Sprintf("%04d-%s.md", number, slug)
		relative := filepath.ToSlash(filepath.Join(ADRDirectory, filename))
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			return nil, fmt.Errorf("migration target already exists: %s", relative)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		content := []byte(renderADR(input))
		for _, marker := range []string{"schema_version:", "decision_matrix:"} {
			if strings.Contains(string(content), marker) {
				return nil, fmt.Errorf("legacy decision %s contains structured metadata that cannot be converted safely", decision.ID)
			}
		}
		planned = append(planned, plannedMigrationADR{Relative: relative, Content: content})
		number++
	}
	return planned, nil
}

func validateLegacyDecision(filenameID string, artifact legacyDecisionArtifact) error {
	decision := artifact.Decision
	if artifact.SchemaVersion != "1.0.0" && artifact.SchemaVersion != "1.1.0" {
		return fmt.Errorf("schema version %q is not supported", artifact.SchemaVersion)
	}
	if artifact.Revision < 1 {
		return fmt.Errorf("revision must be positive")
	}
	if !legacyDecisionIDPattern.MatchString(decision.ID) || decision.ID != filenameID {
		return fmt.Errorf("decision id and filename do not match")
	}
	if !validLegacyText(decision.Title, 1, 500) || strings.ContainsAny(decision.Title, "\r\n") {
		return fmt.Errorf("title is missing or too long")
	}
	if !validLegacyText(decision.Context, 1, 20_000) {
		return fmt.Errorf("context is missing or too long")
	}
	if !validLegacyText(decision.Decision, 1, 20_000) {
		return fmt.Errorf("decision text is missing or too long")
	}
	if decision.Status != "proposed" && decision.Status != "accepted" && decision.Status != "rejected" && decision.Status != "deprecated" && decision.Status != "superseded" {
		return fmt.Errorf("status %q is not supported", decision.Status)
	}
	if decision.Date != "" {
		if !legacyDatePattern.MatchString(decision.Date) {
			return fmt.Errorf("date must use YYYY-MM-DD")
		}
		if _, err := time.Parse("2006-01-02", decision.Date); err != nil {
			return fmt.Errorf("date is invalid")
		}
	}
	if len(decision.Drivers) == 0 || len(decision.Drivers) > 30 {
		return fmt.Errorf("decision must include between 1 and 30 drivers")
	}
	if err := validateLegacyTextList(decision.Drivers, 30, 1, 2000); err != nil {
		return fmt.Errorf("decision drivers: %w", err)
	}
	if len(decision.ConsideredOptionIDs) == 0 || len(decision.ConsideredOptionIDs) > 5 {
		return fmt.Errorf("decision must include between 1 and 5 considered options")
	}
	considered := make(map[string]bool, len(decision.ConsideredOptionIDs))
	for _, optionID := range decision.ConsideredOptionIDs {
		if !legacyOptionIDPattern.MatchString(optionID) || considered[optionID] {
			return fmt.Errorf("considered option ids must be unique OPT-NNN values")
		}
		considered[optionID] = true
	}
	if decision.SelectedOptionID != nil && !considered[*decision.SelectedOptionID] {
		return fmt.Errorf("selected option does not match a considered option")
	}
	if artifact.SchemaVersion == "1.1.0" {
		if decision.Date == "" || strings.TrimSpace(decision.Motivation.WhyBuilding) == "" ||
			strings.TrimSpace(decision.Motivation.Problem) == "" || strings.TrimSpace(decision.Motivation.Audience) == "" ||
			strings.TrimSpace(decision.Motivation.Alternatives) == "" || strings.TrimSpace(decision.Motivation.WhyThisSolution) == "" {
			return fmt.Errorf("schema 1.1.0 requires a date and all motivation fields")
		}
		if len(decision.DecisionMatrix) < 2 || len(decision.DecisionMatrix) > 5 ||
			len(decision.ValidationCriteria) == 0 || len(decision.ActionContract.GoodOutcomes) == 0 ||
			len(decision.ActionContract.BadOutcomes) == 0 || strings.TrimSpace(decision.ActionContract.CorrectExample) == "" ||
			strings.TrimSpace(decision.ActionContract.IncorrectExample) == "" {
			return fmt.Errorf("schema 1.1.0 decision is missing required drivers, options, validation, or action-contract fields")
		}
		if err := validateLegacyTextList(decision.ValidationCriteria, 30, 1, 2000); err != nil {
			return fmt.Errorf("decision validation criteria: %w", err)
		}
		matrixIDs := make(map[string]bool, len(decision.DecisionMatrix))
		for _, option := range decision.DecisionMatrix {
			if !legacyOptionIDPattern.MatchString(option.ID) || matrixIDs[option.ID] || !considered[option.ID] {
				return fmt.Errorf("decision matrix option ids must be unique considered OPT-NNN values")
			}
			matrixIDs[option.ID] = true
			if !validLegacyText(option.Name, 1, 500) || option.FitScore < 0 || option.FitScore > 100 {
				return fmt.Errorf("decision matrix options require a valid name and fit score")
			}
			if option.Outcome != "proposed" && option.Outcome != "accepted" && option.Outcome != "rejected" {
				return fmt.Errorf("decision matrix option outcome is unsupported")
			}
			for _, list := range [][]string{option.Benefits, option.Drawbacks, option.Risks, option.Evidence} {
				if err := validateLegacyTextList(list, 20, 1, 2000); err != nil {
					return fmt.Errorf("decision matrix option text: %w", err)
				}
			}
		}
		for optionID := range considered {
			if !matrixIDs[optionID] {
				return fmt.Errorf("decision matrix is missing a considered option")
			}
		}
		if err := validateLegacyTextList(decision.ActionContract.GoodOutcomes, 20, 1, 2000); err != nil {
			return fmt.Errorf("decision action contract: %w", err)
		}
		if err := validateLegacyTextList(decision.ActionContract.BadOutcomes, 20, 1, 2000); err != nil {
			return fmt.Errorf("decision action contract: %w", err)
		}
		if !validLegacyText(decision.ActionContract.CorrectExample, 1, 2000) || !validLegacyText(decision.ActionContract.IncorrectExample, 1, 2000) {
			return fmt.Errorf("decision action contract examples are missing or invalid")
		}
	}
	if len(decision.ValidationCriteria) == 0 || len(decision.ValidationCriteria) > 30 {
		return fmt.Errorf("decision must include between 1 and 30 validation criteria")
	}
	if err := validateLegacyTextList(decision.ValidationCriteria, 30, 1, 2000); err != nil {
		return fmt.Errorf("decision validation criteria: %w", err)
	}
	for _, list := range [][]string{decision.PositiveConsequences, decision.NegativeConsequences, decision.Assumptions} {
		if err := validateLegacyTextList(list, 30, 0, 2000); err != nil {
			return fmt.Errorf("decision consequences or assumptions: %w", err)
		}
	}
	if len(decision.Supersedes) > 100 {
		return fmt.Errorf("decision supersedes too many records")
	}
	for _, superseded := range decision.Supersedes {
		if !legacyDecisionIDPattern.MatchString(superseded) || superseded == decision.ID {
			return fmt.Errorf("decision supersedes contains an invalid id")
		}
	}
	if decision.Status == "accepted" && decision.SelectedOptionID == nil {
		return fmt.Errorf("accepted decision has no selected option")
	}
	return nil
}

func validLegacyText(value string, minimum, maximum int) bool {
	value = strings.TrimSpace(value)
	return len(value) >= minimum && len(value) <= maximum
}

func validateLegacyTextList(values []string, maximumCount, minimumLength, maximumLength int) error {
	if len(values) > maximumCount {
		return fmt.Errorf("list exceeds the supported item count")
	}
	for _, value := range values {
		if !validLegacyText(value, minimumLength, maximumLength) {
			return fmt.Errorf("list contains an empty or oversized item")
		}
	}
	return nil
}

func migratedADRStatus(status string) string {
	switch status {
	case "accepted":
		return "accepted"
	case "proposed":
		return "proposed"
	default:
		return "deprecated"
	}
}

func readLegacyArtifact(root, relative string, maxBytes int64) ([]byte, error) {
	if err := ensureLegacyParents(root, relative); err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("legacy file is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !os.SameFile(info, openedInfo) {
		file.Close()
		return nil, fmt.Errorf("legacy file changed during migration assessment")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("legacy file exceeds the bounded migration limit")
	}
	return data, nil
}

func ensureLegacyParents(root, relative string) error {
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("legacy path has a symlink or non-directory parent")
		}
	}
	return nil
}

func rejectLegacySecrets(relative string, data []byte) error {
	for lineNumber, line := range strings.Split(string(data), "\n") {
		kinds := secretKindsOnLine(line)
		if len(kinds) > 0 {
			return fmt.Errorf("secret-like value in legacy artifact %q:%d (%s); remove it before migration", relative, lineNumber+1, kinds[0])
		}
	}
	return nil
}

func decodeLegacyYAML(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not supported")
		}
		return err
	}
	return nil
}

func splitLegacyFrontmatter(content string) ([]byte, string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return nil, "", fmt.Errorf("frontmatter is required")
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return nil, "", fmt.Errorf("frontmatter is not terminated")
	}
	end += 4
	return []byte(content[4:end]), content[end+5:], nil
}

func readMigrationGitHabits(root string) ([]byte, error) {
	data, err := readLegacyArtifact(root, GitHabitsFilename, legacyDecisionMaxBytes)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func inventoryLegacyPaths(root string, metadataExists bool, architectInfo os.FileInfo, architectErr error) ([]string, error) {
	paths := make([]string, 0)
	if metadataExists {
		paths = append(paths, LegacyScaffoldFilename)
	}
	if architectErr != nil {
		if os.IsNotExist(architectErr) {
			return paths, nil
		}
		return paths, fmt.Errorf("inspect %s: %w", LegacyArchitectDir, architectErr)
	}
	paths = append(paths, LegacyArchitectDir)
	if architectInfo.Mode()&os.ModeSymlink != 0 || !architectInfo.IsDir() {
		sort.Strings(paths)
		return paths, nil
	}
	count := 0
	err := filepath.WalkDir(filepath.Join(root, LegacyArchitectDir), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == filepath.Join(root, LegacyArchitectDir) {
			return nil
		}
		count++
		if count > legacyInventoryLimit {
			return fmt.Errorf("legacy inventory exceeds the migration limit of %d entries", legacyInventoryLimit)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(paths)
	if err != nil {
		return paths, fmt.Errorf("inventory legacy project files: %w", err)
	}
	return paths, nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	sort.Strings(unique)
	return unique
}

func hasPath(paths []string, expected string) bool {
	for _, path := range paths {
		if path == expected {
			return true
		}
	}
	return false
}

func removeString(values []string, value string) []string {
	result := make([]string, 0, len(values))
	for _, candidate := range values {
		if candidate != value {
			result = append(result, candidate)
		}
	}
	return result
}
