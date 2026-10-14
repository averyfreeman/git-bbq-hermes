package gitbbq

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ValidateProject checks required project files, configuration, generated
// architecture projections, ownership metadata, and bounded
// secret scanning for architecture artifacts. It returns the first validation
// or filesystem error encountered.
func ValidateProject(root string) error {
	return validateProject(root, true)
}

func validateProject(root string, checkProjections bool) error {
	if err := scanArchitectureSecrets(root, checkProjections); err != nil {
		return err
	}
	manifest, err := loadManifest(root)
	if err != nil {
		return err
	}
	if _, err := loadGitHabits(root); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ContextFilename)); err != nil {
		return fmt.Errorf("missing %s: %w", ContextFilename, err)
	}
	if _, err := os.Stat(filepath.Join(root, MattDependencyMetadataPath)); err != nil {
		return fmt.Errorf("missing %s: %w", MattDependencyMetadataPath, err)
	}
	var dependency MattDependency
	if err := readYAML(filepath.Join(root, MattDependencyMetadataPath), &dependency); err != nil {
		return err
	}
	if dependency != manifest.Matt {
		return fmt.Errorf("Matt dependency metadata does not match the manifest pin")
	}
	if _, err := os.Stat(filepath.Join(root, ContextMapFilename)); err != nil {
		return fmt.Errorf("missing %s: %w", ContextMapFilename, err)
	}
	if _, err := readOwnershipLedger(root); err != nil {
		return fmt.Errorf("invalid %s: %w", OwnershipFilename, err)
	}
	index, err := buildADRIndex(root)
	if err != nil {
		return err
	}
	if checkProjections {
		if err := validateArchitectureProjections(root, manifest, index); err != nil {
			return err
		}
	}
	return nil
}

func validateArchitectureProjections(root string, manifest Manifest, index ADRIndex) error {
	expectedContract := buildArchitectureContract(manifest, index)
	contractPath := filepath.Join(root, ContractFilename)
	contractExists, err := regularFileExists(contractPath)
	if err != nil {
		return err
	}
	planPath := filepath.Join(root, ImplementationPlanFilename)
	planExists, err := regularFileExists(planPath)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(root, ADRIndexFilename)
	indexExists, err := regularFileExists(indexPath)
	if err != nil {
		return err
	}
	projectionSetStarted := contractExists || planExists || indexExists || index.DecisionCount > 0
	if !projectionSetStarted {
		return nil
	}
	if !contractExists {
		return fmt.Errorf("missing %s projection; run git-bbq project to generate architecture projections", ContractFilename)
	}
	if !planExists {
		return fmt.Errorf("missing %s projection; run git-bbq project to generate architecture projections", ImplementationPlanFilename)
	}
	if index.DecisionCount > 0 && !indexExists {
		return fmt.Errorf("missing %s projection; run git-bbq project to generate architecture projections", ADRIndexFilename)
	}

	var actualContract ArchitectureContract
	if err := readYAML(contractPath, &actualContract); err != nil {
		return fmt.Errorf("read %s projection: %w", ContractFilename, err)
	}
	if !sameArchitectureContract(actualContract, expectedContract) {
		return fmt.Errorf("%s is out of date; run git-bbq project to regenerate projections", ContractFilename)
	}

	actualPlan, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("read %s projection: %w", ImplementationPlanFilename, err)
	}
	if string(actualPlan) != renderImplementationPlan(manifest, index) {
		return fmt.Errorf("%s is out of date; run git-bbq project to regenerate projections", ImplementationPlanFilename)
	}

	if indexExists {
		var actual ADRIndex
		file, err := os.Open(indexPath)
		if err != nil {
			return fmt.Errorf("read %s projection: %w", ADRIndexFilename, err)
		}
		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&actual); err != nil {
			file.Close()
			return fmt.Errorf("read %s projection: %w", ADRIndexFilename, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			file.Close()
			if err == nil {
				return fmt.Errorf("read %s projection: unexpected trailing JSON value", ADRIndexFilename)
			}
			return fmt.Errorf("read %s projection: %w", ADRIndexFilename, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s projection: %w", ADRIndexFilename, err)
		}
		if !sameADRIndexProjection(actual, index) {
			return fmt.Errorf("%s is out of date; run git-bbq project to regenerate projections", ADRIndexFilename)
		}
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("projection is not a regular file: %s", path)
	}
	return true, nil
}

func sameArchitectureContract(actual, expected ArchitectureContract) bool {
	return actual.SchemaVersion == expected.SchemaVersion &&
		actual.Revision == expected.Revision &&
		actual.Scope == expected.Scope &&
		actual.Problem == expected.Problem &&
		sameStrings(actual.Languages, expected.Languages) &&
		sameStrings(actual.ADRFiles, expected.ADRFiles)
}

func sameADRIndexProjection(actual, expected ADRIndex) bool {
	if actual.SchemaVersion != expected.SchemaVersion || actual.GeneratedAt.IsZero() ||
		actual.DecisionCount != expected.DecisionCount || len(actual.Decisions) != len(expected.Decisions) {
		return false
	}
	for i, record := range actual.Decisions {
		want := expected.Decisions[i]
		if record.Number != want.Number || record.Title != want.Title || record.Status != want.Status ||
			record.Filename != want.Filename || record.Path != want.Path || record.Body != want.Body {
			return false
		}
	}
	return true
}
