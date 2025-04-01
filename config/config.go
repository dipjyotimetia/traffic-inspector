package config

import (
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// TargetRoute defines a mapping between a path prefix and a target URL
type TargetRoute struct {
	PathPrefix string `mapstructure:"path_prefix"`
	TargetURL  string `mapstructure:"target_url"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	CertFile      string `mapstructure:"cert_file"`
	KeyFile       string `mapstructure:"key_file"`
	Port          int    `mapstructure:"port"`
	AllowInsecure bool   `mapstructure:"allow_insecure"`
}

// WebSocketConfig holds WebSocket specific configuration
type WebSocketConfig struct {
	Enabled           bool             `mapstructure:"enabled"`
	ReadBufferSize    int              `mapstructure:"read_buffer_size"`
	WriteBufferSize   int              `mapstructure:"write_buffer_size"`
	EnableCompression bool             `mapstructure:"enable_compression"`
	HandshakeTimeout  time.Duration    `mapstructure:"handshake_timeout"`
	PingInterval      time.Duration    `mapstructure:"ping_interval"`
	WebSocketRoutes   []WebSocketRoute `mapstructure:"routes"`
}

// WebSocketRoute defines a WebSocket specific routing configuration
type WebSocketRoute struct {
	PathPrefix  string `mapstructure:"path_prefix"`
	TargetURL   string `mapstructure:"target_url"`
	Description string `mapstructure:"description"`
}

// Config holds the application configuration
type Config struct {
	HTTPPort      int             `mapstructure:"http_port"`
	HTTPTargetURL string          `mapstructure:"http_target_url"` // Default target for backward compatibility
	TargetRoutes  []TargetRoute   `mapstructure:"target_routes"`   // New field for path-based routing
	SQLiteDBPath  string          `mapstructure:"sqlite_db_path"`
	RecordingMode bool            `mapstructure:"recording_mode"`
	ReplayMode    bool            `mapstructure:"replay_mode"`
	TLS           TLSConfig       `mapstructure:"tls"`       // TLS configuration
	WebSocket     WebSocketConfig `mapstructure:"websocket"` // WebSocket configuration
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

	// Set default TLS port if TLS is enabled but no port specified
	if config.TLS.Enabled && config.TLS.Port == 0 {
		config.TLS.Port = 8443 // Default HTTPS port for the proxy
	}

	// Set WebSocket defaults
	if config.WebSocket.Enabled {
		// Default WebSocket buffer sizes if not specified
		if config.WebSocket.ReadBufferSize <= 0 {
			config.WebSocket.ReadBufferSize = 1024
		}
		if config.WebSocket.WriteBufferSize <= 0 {
			config.WebSocket.WriteBufferSize = 1024
		}
		// Default handshake timeout
		if config.WebSocket.HandshakeTimeout <= 0 {
			config.WebSocket.HandshakeTimeout = 10 * time.Second
		}
		// Default ping interval
		if config.WebSocket.PingInterval <= 0 {
			config.WebSocket.PingInterval = 30 * time.Second
		}
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

	// Validate TLS config if enabled
	if config.TLS.Enabled {
		if config.TLS.CertFile == "" {
			return errors.New("tls.cert_file must be provided when TLS is enabled")
		}
		if config.TLS.KeyFile == "" {
			return errors.New("tls.key_file must be provided when TLS is enabled")
		}
	}

	// Validate WebSocket config if enabled
	if config.WebSocket.Enabled {
		if len(config.WebSocket.WebSocketRoutes) == 0 {
			return errors.New("at least one WebSocket route must be defined when WebSocket is enabled")
		}

		for _, route := range config.WebSocket.WebSocketRoutes {
			if route.PathPrefix == "" {
				return errors.New("path_prefix cannot be empty for WebSocket routes")
			}

			// Ensure path_prefix starts with "/"
			if !strings.HasPrefix(route.PathPrefix, "/") {
				return errors.New("path_prefix must start with a '/' character for WebSocket routes")
			}

			if route.TargetURL == "" {
				return errors.New("target_url cannot be empty for WebSocket routes")
			}
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

// GetWebSocketTargetURL returns the appropriate WebSocket target URL for a given path
func (c *Config) GetWebSocketTargetURL(path string) (string, bool) {
	// Check if WebSocket is enabled
	if !c.WebSocket.Enabled {
		return "", false
	}

	// Try to find a matching WebSocket route
	for _, route := range c.WebSocket.WebSocketRoutes {
		if strings.HasPrefix(path, route.PathPrefix) {
			return route.TargetURL, true
		}
	}

	// No matching WebSocket route
	return "", false
}

// IsWebSocketPath checks if a path should be handled as WebSocket
func (c *Config) IsWebSocketPath(path string) bool {
	if !c.WebSocket.Enabled {
		return false
	}

	for _, route := range c.WebSocket.WebSocketRoutes {
		if strings.HasPrefix(path, route.PathPrefix) {
			return true
		}
	}

	return false
}

// GetTLSConfig returns a TLS configuration for clients
func (c *Config) GetTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: c.TLS.AllowInsecure,
	}
}
