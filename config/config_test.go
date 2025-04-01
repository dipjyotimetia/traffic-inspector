package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name      string
		configMap map[string]interface{}
		wantErr   bool
	}{
		// Existing test cases...
		{
			name: "Valid configuration",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"sqlite_db_path":  "test.db",
			},
			wantErr: false,
		},
		{
			name: "Missing HTTP port",
			configMap: map[string]interface{}{
				"http_target_url": "http://example.com",
			},
			wantErr: true,
		},
		{
			name: "Missing target URL",
			configMap: map[string]interface{}{
				"http_port": 8080,
			},
			wantErr: true,
		},
		{
			name: "Both recording and replay mode",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"recording_mode":  true,
				"replay_mode":     true,
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without cert",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"tls": map[string]interface{}{
					"enabled": true,
					"port":    8443,
				},
			},
			wantErr: true,
		},
		{
			name: "Valid TLS config",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"tls": map[string]interface{}{
					"enabled":   true,
					"port":      8443,
					"cert_file": "cert.pem",
					"key_file":  "key.pem",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid target routes",
			configMap: map[string]interface{}{
				"http_port": 8080,
				"target_routes": []map[string]interface{}{
					{
						"path_prefix": "/api",
						"target_url":  "http://api.example.com",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid target routes - missing path prefix",
			configMap: map[string]interface{}{
				"http_port": 8080,
				"target_routes": []map[string]interface{}{
					{
						"target_url": "http://api.example.com",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid target routes - path prefix without leading slash",
			configMap: map[string]interface{}{
				"http_port": 8080,
				"target_routes": []map[string]interface{}{
					{
						"path_prefix": "api",
						"target_url":  "http://api.example.com",
					},
				},
			},
			wantErr: true,
		},
		// New test cases for WebSocket configuration
		{
			name: "Valid WebSocket config",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"websocket": map[string]interface{}{
					"enabled":           true,
					"read_buffer_size":  4096,
					"write_buffer_size": 4096,
					"routes": []map[string]interface{}{
						{
							"path_prefix": "/ws",
							"target_url":  "ws://websocket.example.com",
							"description": "Test WebSocket",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "WebSocket enabled without routes",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"websocket": map[string]interface{}{
					"enabled": true,
				},
			},
			wantErr: true,
		},
		{
			name: "WebSocket route with invalid path prefix",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"websocket": map[string]interface{}{
					"enabled": true,
					"routes": []map[string]interface{}{
						{
							"path_prefix": "ws", // Missing leading slash
							"target_url":  "ws://websocket.example.com",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "WebSocket route without target URL",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"websocket": map[string]interface{}{
					"enabled": true,
					"routes": []map[string]interface{}{
						{
							"path_prefix": "/ws",
							// Missing target_url
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "WebSocket with custom timeouts",
			configMap: map[string]interface{}{
				"http_port":       8080,
				"http_target_url": "http://example.com",
				"websocket": map[string]interface{}{
					"enabled":           true,
					"handshake_timeout": "15s",
					"ping_interval":     "45s",
					"routes": []map[string]interface{}{
						{
							"path_prefix": "/ws",
							"target_url":  "ws://websocket.example.com",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			for key, value := range tt.configMap {
				v.Set(key, value)
			}

			config, err := LoadConfig(v)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				// Verify defaults are set
				if config.SQLiteDBPath == "" {
					t.Error("Default SQLiteDBPath not set")
				}

				// Verify TLS port default
				if config.TLS.Enabled && config.TLS.Port == 0 {
					t.Error("Default TLS port not set when TLS enabled")
				}

				// Verify WebSocket defaults when enabled
				if config.WebSocket.Enabled {
					if config.WebSocket.ReadBufferSize <= 0 {
						t.Error("Default WebSocket ReadBufferSize not set")
					}

					if config.WebSocket.WriteBufferSize <= 0 {
						t.Error("Default WebSocket WriteBufferSize not set")
					}

					if config.WebSocket.HandshakeTimeout <= 0 {
						t.Error("Default WebSocket HandshakeTimeout not set")
					}

					if config.WebSocket.PingInterval <= 0 {
						t.Error("Default WebSocket PingInterval not set")
					}
				}
			}
		})
	}
}

func TestGetTargetURL(t *testing.T) {
	config := &Config{
		HTTPTargetURL: "https://default.example.com",
		TargetRoutes: []TargetRoute{
			{
				PathPrefix: "/api/users",
				TargetURL:  "https://users.example.com",
			},
			{
				PathPrefix: "/api/products",
				TargetURL:  "https://products.example.com",
			},
		},
	}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Match first route",
			path:     "/api/users/123",
			expected: "https://users.example.com",
		},
		{
			name:     "Match second route",
			path:     "/api/products/456",
			expected: "https://products.example.com",
		},
		{
			name:     "Use default for non-matching path",
			path:     "/other/path",
			expected: "https://default.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetTargetURL(tt.path)
			if result != tt.expected {
				t.Errorf("GetTargetURL() got = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetWebSocketTargetURL tests the GetWebSocketTargetURL method
func TestGetWebSocketTargetURL(t *testing.T) {
	config := &Config{
		WebSocket: WebSocketConfig{
			Enabled: true,
			WebSocketRoutes: []WebSocketRoute{
				{
					PathPrefix:  "/ws/chat",
					TargetURL:   "wss://chat.example.com",
					Description: "Chat WebSocket",
				},
				{
					PathPrefix:  "/ws/events",
					TargetURL:   "wss://events.example.com",
					Description: "Events WebSocket",
				},
			},
		},
	}

	disabledConfig := &Config{
		WebSocket: WebSocketConfig{
			Enabled: false,
			WebSocketRoutes: []WebSocketRoute{
				{
					PathPrefix:  "/ws/chat",
					TargetURL:   "wss://chat.example.com",
					Description: "Chat WebSocket",
				},
			},
		},
	}

	tests := []struct {
		name     string
		config   *Config
		path     string
		wantURL  string
		wantBool bool
	}{
		{
			name:     "Match first WebSocket route",
			config:   config,
			path:     "/ws/chat/room/123",
			wantURL:  "wss://chat.example.com",
			wantBool: true,
		},
		{
			name:     "Match second WebSocket route",
			config:   config,
			path:     "/ws/events/stream",
			wantURL:  "wss://events.example.com",
			wantBool: true,
		},
		{
			name:     "No matching WebSocket route",
			config:   config,
			path:     "/api/data",
			wantURL:  "",
			wantBool: false,
		},
		{
			name:     "WebSocket disabled",
			config:   disabledConfig,
			path:     "/ws/chat/room/123",
			wantURL:  "",
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotBool := tt.config.GetWebSocketTargetURL(tt.path)
			if gotURL != tt.wantURL || gotBool != tt.wantBool {
				t.Errorf("GetWebSocketTargetURL() = (%v, %v), want (%v, %v)",
					gotURL, gotBool, tt.wantURL, tt.wantBool)
			}
		})
	}
}

// TestIsWebSocketPath tests the IsWebSocketPath method
func TestIsWebSocketPath(t *testing.T) {
	config := &Config{
		WebSocket: WebSocketConfig{
			Enabled: true,
			WebSocketRoutes: []WebSocketRoute{
				{
					PathPrefix:  "/ws/chat",
					TargetURL:   "wss://chat.example.com",
					Description: "Chat WebSocket",
				},
				{
					PathPrefix:  "/socket/",
					TargetURL:   "wss://socket.example.com",
					Description: "Socket API",
				},
			},
		},
	}

	disabledConfig := &Config{
		WebSocket: WebSocketConfig{
			Enabled: false,
			WebSocketRoutes: []WebSocketRoute{
				{
					PathPrefix:  "/ws/chat",
					TargetURL:   "wss://chat.example.com",
					Description: "Chat WebSocket",
				},
			},
		},
	}

	tests := []struct {
		name   string
		config *Config
		path   string
		want   bool
	}{
		{
			name:   "WebSocket path first route",
			config: config,
			path:   "/ws/chat/user/123",
			want:   true,
		},
		{
			name:   "WebSocket path second route",
			config: config,
			path:   "/socket/notifications",
			want:   true,
		},
		{
			name:   "Not a WebSocket path",
			config: config,
			path:   "/api/users",
			want:   false,
		},
		{
			name:   "WebSocket disabled",
			config: disabledConfig,
			path:   "/ws/chat/user/123",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsWebSocketPath(tt.path)
			if got != tt.want {
				t.Errorf("IsWebSocketPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWebSocketTimeoutDefaults tests that timeout defaults are applied correctly
func TestWebSocketTimeoutDefaults(t *testing.T) {
	v := viper.New()
	v.Set("http_port", 8080)
	v.Set("http_target_url", "http://example.com")
	v.Set("websocket.enabled", true)
	v.Set("websocket.routes", []map[string]interface{}{
		{
			"path_prefix": "/ws",
			"target_url":  "ws://example.com/ws",
		},
	})

	config, err := LoadConfig(v)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Check default timeout values
	if config.WebSocket.HandshakeTimeout != 10*time.Second {
		t.Errorf("Default HandshakeTimeout not set correctly, got %v, want %v",
			config.WebSocket.HandshakeTimeout, 10*time.Second)
	}

	if config.WebSocket.PingInterval != 30*time.Second {
		t.Errorf("Default PingInterval not set correctly, got %v, want %v",
			config.WebSocket.PingInterval, 30*time.Second)
	}

	// Check default buffer sizes
	if config.WebSocket.ReadBufferSize != 1024 {
		t.Errorf("Default ReadBufferSize not set correctly, got %v, want %v",
			config.WebSocket.ReadBufferSize, 1024)
	}

	if config.WebSocket.WriteBufferSize != 1024 {
		t.Errorf("Default WriteBufferSize not set correctly, got %v, want %v",
			config.WebSocket.WriteBufferSize, 1024)
	}
}
