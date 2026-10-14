package gitbbq

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"
)

// GenerateSchemas returns the JSON schemas for Git BBQ's manifest, Git habits,
// Git plan and execution, and architecture contract types. Map keys are schema
// filenames and values are newline-terminated JSON documents.
func GenerateSchemas() (map[string][]byte, error) {
	values := []struct {
		name  string
		value any
	}{
		{name: "gitbbq-preferences.schema.json", value: Preferences{}},
		{name: "gitbbq-manifest.schema.json", value: Manifest{}},
		{name: "githabits.schema.json", value: GitHabits{}},
		{name: "githabits-plan.schema.json", value: GitPlan{}},
		{name: "githabits-execution.schema.json", value: GitExecution{}},
		{name: "githabits-release.schema.json", value: GitReleaseRecommendation{}},
		{name: "githabits-baseline-plan.schema.json", value: GitBaselinePlan{}},
		{name: "githabits-baseline-execution.schema.json", value: GitBaselineExecution{}},
		{name: "architecture-contract.schema.json", value: ArchitectureContract{}},
	}
	result := make(map[string][]byte, len(values))
	for _, item := range values {
		var schema *jsonschema.Schema
		var err error
		switch item.value.(type) {
		case Preferences:
			schema, err = jsonschema.For[Preferences](nil)
		case Manifest:
			schema, err = jsonschema.For[Manifest](nil)
		case GitHabits:
			schema, err = jsonschema.For[GitHabits](nil)
		case GitPlan:
			schema, err = jsonschema.For[GitPlan](nil)
		case GitExecution:
			schema, err = jsonschema.For[GitExecution](nil)
		case GitReleaseRecommendation:
			schema, err = jsonschema.For[GitReleaseRecommendation](nil)
		case GitBaselinePlan:
			schema, err = jsonschema.For[GitBaselinePlan](nil)
		case GitBaselineExecution:
			schema, err = jsonschema.For[GitBaselineExecution](nil)
		case ArchitectureContract:
			schema, err = jsonschema.For[ArchitectureContract](nil)
		}
		if err != nil {
			return nil, err
		}
		schema.Schema = "https://json-schema.org/draft/2020-12/schema"
		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return nil, err
		}
		result[item.name] = append(data, '\n')
	}
	return result, nil
}

// WriteSchemas generates the supported JSON schemas under root/schemas/gitbbq
// and returns their project-relative paths in sorted order.
func WriteSchemas(root string) ([]string, error) {
	schemas, err := GenerateSchemas()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(schemas))
	for name, data := range schemas {
		relative := filepath.ToSlash(filepath.Join("schemas", "gitbbq", name))
		if _, err := writeGenerated(root, relative, data, true); err != nil {
			return nil, err
		}
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	return paths, nil
}
