package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Database DatabaseConfig `yaml:"database"`
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

func GetRootDir() (string, error) {
	// Get current working directory (like Perl's FindBin)
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're in golang directory, go up one level to project root
	if filepath.Base(wd) == "golang" {
		rootDir := filepath.Dir(wd)
		absRoot, err := filepath.Abs(rootDir)
		if err != nil {
			return "", err
		}
		return absRoot, nil
	}

	// If we're already in project root, check if golang subdirectory exists
	golangDir := filepath.Join(wd, "golang")
	if _, err := os.Stat(golangDir); err == nil {
		// We're in project root
		absRoot, err := filepath.Abs(wd)
		if err != nil {
			return "", err
		}
		return absRoot, nil
	}

	// Try to find project root by looking for golang directory in parent
	current := wd
	for i := 0; i < 5; i++ { // Max 5 levels up
		golangDir := filepath.Join(current, "golang")
		if _, err := os.Stat(golangDir); err == nil {
			absRoot, err := filepath.Abs(current)
			if err != nil {
				return "", err
			}
			return absRoot, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break // Reached root
		}
		current = parent
	}

	// Fallback: assume current directory is root
	return wd, nil
}

func GetFontDir() (string, error) {
	rootDir, err := GetRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, "res", "fonts"), nil
}
