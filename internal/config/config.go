package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName  = ".alexa-cli"
	configFileName = "config.json"
)

// Config holds the Alexa CLI configuration
type Config struct {
	RefreshToken string `json:"refresh_token"`
	AmazonDomain string `json:"amazon_domain,omitempty"` // e.g., "amazon.com", "amazon.de"
	DeviceSerial string `json:"default_device,omitempty"`
}

// Path returns the full path to the config file
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// Dir returns the config directory path
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, configDirName), nil
}

// BinDir returns the path to the binary cache directory (~/.alexa-cli/bin/)
func BinDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin"), nil
}

// Load reads the configuration from disk
func Load() (*Config, error) {
	// Check environment variable first
	if token := os.Getenv("ALEXA_REFRESH_TOKEN"); token != "" {
		return &Config{
			RefreshToken: token,
			AmazonDomain: getEnvOrDefault("ALEXA_AMAZON_DOMAIN", "amazon.com"),
		}, nil
	}

	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not configured. Run 'alexacli auth' first or set ALEXA_REFRESH_TOKEN")
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.RefreshToken == "" {
		return nil, fmt.Errorf("refresh_token not set in config. Run 'alexacli auth'")
	}

	// Default domain
	if cfg.AmazonDomain == "" {
		cfg.AmazonDomain = "amazon.com"
	}

	return &cfg, nil
}

// Save writes the configuration to disk
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path, err := Path()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
