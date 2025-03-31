package config

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

// TargetRoute defines a mapping between a path prefix and a target URL
type TargetRoute struct {
	PathPrefix string `mapstructure:"path_prefix"`
	TargetURL  string `mapstructure:"target_url"`
}

// Config holds the application configuration
type Config struct {
	HTTPPort      int           `mapstructure:"http_port"`
	HTTPTargetURL string        `mapstructure:"http_target_url"` // Default target for backward compatibility
	TargetRoutes  []TargetRoute `mapstructure:"target_routes"`   // New field for path-based routing
	SQLiteDBPath  string        `mapstructure:"sqlite_db_path"`
	RecordingMode bool          `mapstructure:"recording_mode"`
	ReplayMode    bool          `mapstructure:"replay_mode"`
}

// LoadConfig reads configuration from Viper
func LoadConfig(v *viper.Viper) (*Config, error) {
	var config Config

	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Set default SQLite path if not provided
	if config.SQLiteDBPath == "" {
		config.SQLiteDBPath = "traffic_inspector.db"
	}

	// Validate config
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ValidateConfig ensures the configuration is valid
func validateConfig(config *Config) error {
	if config.RecordingMode && config.ReplayMode {
		return errors.New("cannot enable both RecordingMode and ReplayMode simultaneously")
	}

	if config.HTTPPort <= 0 {
		return errors.New("http_port must be configured")
	}

	// Check if we have either a default target URL or at least one route
	if config.HTTPTargetURL == "" && len(config.TargetRoutes) == 0 {
		return errors.New("either http_target_url or at least one target_route must be set")
	}

	// Validate each target route
	for _, route := range config.TargetRoutes {
		if route.PathPrefix == "" {
			return errors.New("path_prefix cannot be empty for target routes")
		}

		// Ensure path_prefix starts with "/"
		if !strings.HasPrefix(route.PathPrefix, "/") {
			return errors.New("path_prefix must start with a '/' character")
		}

		if route.TargetURL == "" {
			return errors.New("target_url cannot be empty for target routes")
		}
	}

	return nil
}

// GetTargetURL returns the appropriate target URL for a given path
func (c *Config) GetTargetURL(path string) string {
	// First check if we have any matching target routes
	for _, route := range c.TargetRoutes {
		if strings.HasPrefix(path, route.PathPrefix) {
			return route.TargetURL
		}
	}

	// Fall back to default target URL
	return c.HTTPTargetURL
}
