package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration loaded from ~/.config/nlci/config.yaml.
type Config struct {
	Backend     BackendConfig     `yaml:"backend"`
	Definitions DefinitionsConfig `yaml:"definitions"`
	Apple       AppleConfig       `yaml:"apple"`
}

type BackendConfig struct {
	Priority []string      `yaml:"priority"`
	Ollama   OllamaConfig  `yaml:"ollama"`
	LlamaCpp LlamaCppConfig `yaml:"llamacpp"`
	LMStudio LMStudioConfig `yaml:"lmstudio"`
}

type OllamaConfig struct {
	Host  string `yaml:"host"`
	Model string `yaml:"model"`
}

type LlamaCppConfig struct {
	Host  string `yaml:"host"`
	Model string `yaml:"model"`
}

type LMStudioConfig struct {
	Host  string `yaml:"host"`
	Model string `yaml:"model"`
}

type DefinitionsConfig struct {
	Paths []string `yaml:"paths"`
}

type AppleConfig struct {
	Binary string `yaml:"binary"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Backend: BackendConfig{
			Priority: []string{"apple", "ollama", "llamacpp", "lmstudio"},
			Ollama: OllamaConfig{
				Host:  "localhost:11434",
				Model: "llama3.2:3b",
			},
			LlamaCpp: LlamaCppConfig{
				Host:  "localhost:8080",
				Model: "",
			},
			LMStudio: LMStudioConfig{
				Host:  "localhost:1234",
				Model: "",
			},
		},
		Definitions: DefinitionsConfig{
			Paths: []string{
				filepath.Join(home, ".config", "nlci", "definitions"),
			},
		},
		Apple: AppleConfig{
			Binary: filepath.Join(home, ".config", "nlci", "bin", "nlci-apple"),
		},
	}
}

// Load reads the config file at ~/.config/nlci/config.yaml.
// If the file does not exist, Default() is returned.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Default(), fmt.Errorf("config: could not determine home directory: %w", err)
	}

	path := filepath.Join(home, ".config", "nlci", "config.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Default(), fmt.Errorf("config: could not read %s: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("config: could not parse %s: %w", path, err)
	}

	return cfg, nil
}

// Save writes the config to ~/.config/nlci/config.yaml.
func Save(cfg Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("config: could not determine home directory: %w", err)
	}

	dir := filepath.Join(home, ".config", "nlci")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("config: could not create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: could not marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: could not write %s: %w", path, err)
	}

	return nil
}
