package gitbbq

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Preferences contains the user's default Git policy profile and optional
// language selection used by CLI setup.
type Preferences struct {
	Version         int        `yaml:"version" json:"version"`
	Profile         string     `yaml:"profile,omitempty" json:"profile,omitempty"`
	Languages       []string   `yaml:"languages,omitempty" json:"languages,omitempty"`
	GitHabitsPreset *GitHabits `yaml:"githabits,omitempty" json:"githabits,omitempty"`
}

// PreferencesPath returns the user configuration path, honoring GIT_BBQ_CONFIG
// when it is set.
func PreferencesPath() (string, error) {
	if override := os.Getenv("GIT_BBQ_CONFIG"); override != "" {
		return filepath.Clean(override), nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve Git BBQ user configuration directory: %w", err)
	}
	return filepath.Join(directory, "git-bbq", "config.yaml"), nil
}

// LoadPreferences reads and validates user preferences. A missing file returns
// zero-value preferences without an error.
func LoadPreferences() (Preferences, error) {
	path, err := PreferencesPath()
	if err != nil {
		return Preferences{}, err
	}
	var preferences Preferences
	if err := readYAML(path, &preferences); err != nil {
		if os.IsNotExist(err) {
			return Preferences{}, nil
		}
		return Preferences{}, err
	}
	if preferences.Version < 1 {
		return Preferences{}, fmt.Errorf("Git BBQ preferences version must be positive")
	}
	if preferences.Profile != "" {
		if _, err := GitHabitsForProfile(preferences.Profile); err != nil {
			return Preferences{}, err
		}
	}
	if preferences.GitHabitsPreset != nil {
		if err := validateGitHabitsPreset(preferences.GitHabitsPreset); err != nil {
			return Preferences{}, err
		}
		if preferences.Profile == "" {
			preferences.Profile = preferences.GitHabitsPreset.Profile
		}
	}
	preferences.Languages = sortedLanguages(preferences.Languages)
	return preferences, nil
}

// SavePreferences validates preferences and replaces the user configuration
// through a temporary file. A supported profile or GitHabits preset is required.
func SavePreferences(preferences Preferences) error {
	if preferences.Version == 0 {
		preferences.Version = 1
	}
	if preferences.GitHabitsPreset != nil {
		if err := validateGitHabitsPreset(preferences.GitHabitsPreset); err != nil {
			return err
		}
		if preferences.Profile == "" {
			preferences.Profile = preferences.GitHabitsPreset.Profile
		}
	}
	if preferences.Profile == "" {
		return fmt.Errorf("Git BBQ preference profile or GitHabits preset is required")
	}
	if _, err := GitHabitsForProfile(preferences.Profile); err != nil {
		return err
	}
	preferences.Languages = sortedLanguages(preferences.Languages)
	data, err := marshalYAML(preferences)
	if err != nil {
		return err
	}
	path, err := PreferencesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".git-bbq-preferences-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func validateGitHabitsPreset(preset *GitHabits) error {
	if preset == nil {
		return nil
	}
	if err := ValidateGitHabits(*preset); err != nil {
		return fmt.Errorf("invalid GitHabits preset: %w", err)
	}
	data, err := marshalYAML(preset)
	if err != nil {
		return err
	}
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if kinds := secretKindsOnLine(line); len(kinds) > 0 {
			return fmt.Errorf("credential-like value in GitHabits preset at line %d (%s)", lineNumber+1, kinds[0])
		}
	}
	return nil
}
