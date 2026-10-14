package gitbbq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSchemasReflectsGoContracts(t *testing.T) {
	schemas, err := GenerateSchemas()
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 9 {
		t.Fatalf("schema count = %d", len(schemas))
	}
	for name, data := range schemas {
		content := string(data)
		if !strings.Contains(content, "2020-12") || !strings.Contains(content, "schema_version") {
			t.Fatalf("schema %s is not a Go-generated public contract: %s", name, content)
		}
	}
}

func TestGenerateSchemasIncludesReleaseAndBaselineContracts(t *testing.T) {
	schemas, err := GenerateSchemas()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"githabits-release.schema.json",
		"githabits-baseline-plan.schema.json",
		"githabits-baseline-execution.schema.json",
	} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("generated schemas omitted %s", name)
		}
	}
	plan := string(schemas["githabits-plan.schema.json"])
	if !strings.Contains(plan, "needs_input") || !strings.Contains(plan, "required_inputs") {
		t.Errorf("GitHub remote plan schema omitted resolvable input requirements: %s", plan)
	}
}

func TestGenerateSchemasIncludesUserGitHabitsPresetContract(t *testing.T) {
	schemas, err := GenerateSchemas()
	if err != nil {
		t.Fatal(err)
	}
	data, ok := schemas["gitbbq-preferences.schema.json"]
	if !ok {
		t.Fatal("generated schemas omitted the user preferences contract")
	}
	content := string(data)
	for _, field := range []string{"githabits", "languages", "profile"} {
		if !strings.Contains(content, field) {
			t.Errorf("user preferences schema omitted field %q: %s", field, content)
		}
	}
}

func TestWriteSchemasUsesStableProjectDirectory(t *testing.T) {
	root := t.TempDir()
	paths, err := WriteSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 9 {
		t.Fatalf("paths = %#v", paths)
	}
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("missing schema %s: %v", path, err)
		}
	}
}
