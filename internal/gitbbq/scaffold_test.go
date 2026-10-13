package gitbbq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldWritesMattNativeOutputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "example")
	result, err := ScaffoldProject(root, ScaffoldOptions{
		ProjectName: "Example",
		Problem:     "Agents need a repeatable project starting point.",
		Languages:   []string{"go", "python"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) == 0 {
		t.Fatal("scaffold created no files")
	}
	for _, relative := range []string{
		ManifestFilename,
		GitHabitsFilename,
		ContextFilename,
		"AGENTS.md",
		ContextMapFilename,
		MattDependencyMetadataPath,
		SessionPath,
		".agents/skills/githabits/SKILL.md",
		".agents/skills/go/SKILL.md",
		".agents/skills/python/SKILL.md",
		HookConfigPath,
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy compatibility file was created or returned unexpected error: %v", err)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "CONTEXT.md") || !strings.Contains(string(agents), "docs/adr") {
		t.Fatalf("AGENTS.md does not route to Matt documents: %s", agents)
	}
	if !strings.Contains(string(agents), "[Go](./.agents/skills/go/SKILL.md)") || !strings.Contains(string(agents), "[Python](./.agents/skills/python/SKILL.md)") {
		t.Fatalf("AGENTS.md omits language-profile routes: %s", agents)
	}
	goSkill, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "go", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goSkill), "go test ./...") || !strings.Contains(string(goSkill), "gofmt -w .") {
		t.Fatalf("Go skill omits profile guidance: %s", goSkill)
	}
	context, err := os.ReadFile(filepath.Join(root, ContextFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(context) != renderContext("Example") {
		t.Fatalf("generated CONTEXT.md contract changed: %s", context)
	}
	contextMap, err := os.ReadFile(filepath.Join(root, ContextMapFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(contextMap) != renderContextMap() {
		t.Fatalf("generated CONTEXT-MAP.md contract changed: %s", contextMap)
	}
	if _, err := os.Stat(filepath.Join(root, "PLAN.md")); !os.IsNotExist(err) {
		t.Fatalf("competing PLAN.md was created or returned unexpected error: %v", err)
	}
	githabits, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "githabits", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(githabits), "actions.push") || !strings.Contains(string(githabits), "SemVer") {
		t.Fatalf("githabits skill omits action conventions: %s", githabits)
	}
}

func TestGithabitsSkillDocumentsVersionTwoPolicy(t *testing.T) {
	skill := renderGithabitsSkill()
	for _, policyField := range []string{
		"authorization.scope",
		"release.initial_tag_scope",
		"release.commit_bumps",
		"push.after_commit",
		"push.after_tag",
		"remote.visibility_policy",
	} {
		if !strings.Contains(skill, policyField) {
			t.Errorf("githabits skill omitted v2 policy field %q", policyField)
		}
	}
	if strings.Contains(skill, "Codex") {
		t.Fatal("generated githabits skill still describes Codex host permissions")
	}
}

func TestAssessIsReadOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := SnapshotFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := Assess(root)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Mode != "assessment" || len(assessment.Existing) == 0 {
		t.Fatalf("assessment = %#v", assessment)
	}
	after, err := SnapshotFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatal("assessment changed the repository")
	}
}

func TestProjectGeneratesProjectionsWithoutSecondADRSet(t *testing.T) {
	root := t.TempDir()
	if _, err := ScaffoldProject(root, ScaffoldOptions{ProjectName: "Example", Problem: "Keep decisions consumable.", Languages: []string{"go"}}); err != nil {
		t.Fatal(err)
	}
	record, err := CreateADR(root, ADRInput{Title: "Use Matt ADRs", Context: "Agents need portable records.", Decision: "Use docs/adr files.", Why: "Matt skills already define the format.", Status: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := readOwnershipLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Files[relativePath(root, record.Path)] == "" {
		t.Fatalf("created ADR was not added to ownership ledger: %#v", ledger.Files)
	}
	projection, err := Project(root)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ADRCount != 1 {
		t.Fatalf("projection = %#v", projection)
	}
	for _, relative := range []string{ContractFilename, ImplementationPlanFilename, ADRIndexFilename} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("missing projection %s: %v", relative, err)
		}
	}
	plan, err := os.ReadFile(filepath.Join(root, ImplementationPlanFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plan), "### Go") || !strings.Contains(string(plan), "go test ./...") {
		t.Fatalf("implementation plan omits selected language guidance: %s", plan)
	}
}

func TestScaffoldLeavesADRDirectoryLazy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "example")
	if _, err := ScaffoldProject(root, ScaffoldOptions{ProjectName: "Example", Problem: "Keep decisions lazy.", Languages: []string{"go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ADRDirectory)); !os.IsNotExist(err) {
		t.Fatalf("ADR directory was created before the first decision: %v", err)
	}
	if _, err := Project(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ADRDirectory)); !os.IsNotExist(err) {
		t.Fatalf("empty projection created ADR directory: %v", err)
	}
}

func TestValidateProjectRejectsDriftedDependencyMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "example")
	if _, err := ScaffoldProject(root, ScaffoldOptions{ProjectName: "Example", Problem: "Keep pins coherent.", Languages: []string{"go"}}); err != nil {
		t.Fatal(err)
	}
	dependencyPath := filepath.Join(root, MattDependencyMetadataPath)
	data, err := os.ReadFile(dependencyPath)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("commit: drifted\n")...)
	if err := os.WriteFile(dependencyPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProject(root); err == nil {
		t.Fatal("drifted dependency metadata was accepted")
	}
}

func containsPath(paths []string, expected string) bool {
	for _, path := range paths {
		if path == expected {
			return true
		}
	}
	return false
}
