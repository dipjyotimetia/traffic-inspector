package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	conf "github.com/dipjyotimetia/traffic-inspector/config"
	"github.com/dipjyotimetia/traffic-inspector/internal/db"
	"github.com/dipjyotimetia/traffic-inspector/internal/proxy"
	"github.com/dipjyotimetia/traffic-inspector/internal/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the traffic inspector proxy server",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := conf.LoadConfig(viper.GetViper())
		if err != nil {
			log.Fatalf("❌ Failed to load configuration: %v", err)
		}
		log.Printf("🔧 Configuration loaded: Mode=%s", getMode(cfg))

		database, stmt, err := db.Initialize(cfg.SQLiteDBPath)
		if err != nil {
			log.Fatalf("❌ Failed to initialize database: %v", err)
		}
		defer database.Close()
		defer stmt.Close()

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer cancel()

		var wg sync.WaitGroup
		var servers []proxy.Server

		if cfg.HTTPPort > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				httpServer := proxy.StartHTTPProxy(ctx, cfg, database, stmt)
				if httpServer != nil {
					servers = append(servers, httpServer)
				}
			}()
		}

		if cfg.TLS.Enabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				httpsServer := proxy.StartHTTPSProxy(ctx, cfg, database, stmt)
				if httpsServer != nil {
					servers = append(servers, httpsServer)
				}
			}()
		}

		if true { // Always start the UI
			wg.Add(1)
			go func() {
				defer wg.Done()
				uiPort := viper.GetInt("ui_port")
				if uiPort == 0 {
					uiPort = 9090 // Default UI port
				}

				// Create UI handler
				uiHandler := web.NewUIHandler(database)

				// Create a mux and register routes
				mux := http.NewServeMux()
				uiHandler.RegisterRoutes(mux)

				// Create the server
				uiServer := &http.Server{
					Addr:    fmt.Sprintf(":%d", uiPort),
					Handler: mux,
				}

				log.Printf("🌐 Starting web UI at http://localhost:%d/ui/", uiPort)
				if err := uiServer.ListenAndServe(); err != http.ErrServerClosed {
					log.Printf("⚠️ Web UI server error: %v", err)
				}
			}()
		}

		if cfg.WebSocket.Enabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Printf("🔌 Starting WebSocket proxy on port %d", cfg.HTTPPort)
				// WebSocket proxy uses the same HTTP/HTTPS servers, so no need to add to servers list
				wsProxy := proxy.NewWebSocketProxy(cfg, database, stmt)

				// Log WebSocket routes
				log.Println("📝 WebSocket route configuration:")
				for _, route := range cfg.WebSocket.WebSocketRoutes {
					log.Printf("  %s -> %s (%s)", route.PathPrefix, route.TargetURL, route.Description)
				}

				// Register the WebSocket handler with the HTTP servers
				http.Handle("/ws/", wsProxy)
			}()
		}

		<-ctx.Done()
		log.Println("🚨 Shutdown signal received, initiating graceful shutdown...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		var shutdownWg sync.WaitGroup
		for i, server := range servers {
			shutdownWg.Add(1)
			go func(idx int, srv proxy.Server) {
				defer shutdownWg.Done()
				serverType := "HTTP"
				if idx == 1 {
					serverType = "HTTPS"
				}
				log.Printf("⏳ Shutting down %s server...", serverType)
				if err := srv.Shutdown(shutdownCtx); err != nil {
					log.Printf("⚠️ %s server shutdown error: %v", serverType, err)
				} else {
					log.Printf("✅ %s server stopped gracefully", serverType)
				}
			}(i, server)
		}

		shutdownWg.Wait()

		wg.Wait()
		log.Println("🏁 All servers stopped")
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().BoolP("record", "r", false, "Run in recording mode")
	startCmd.Flags().BoolP("replay", "p", false, "Run in replay mode")
	startCmd.Flags().Bool("tls", false, "Enable TLS")
	startCmd.Flags().String("cert", "", "TLS certificate file path")
	startCmd.Flags().String("key", "", "TLS key file path")
	startCmd.Flags().Int("tls-port", 8443, "HTTPS port")
	startCmd.Flags().Bool("insecure", false, "Allow insecure TLS connections to target servers")

	startCmd.Flags().Int("ui-port", 9090, "Port for the web UI")

	// Add WebSocket specific flags
	startCmd.Flags().Bool("websocket", false, "Enable WebSocket support")
	startCmd.Flags().Int("ws-read-buffer", 4096, "WebSocket read buffer size in bytes")
	startCmd.Flags().Int("ws-write-buffer", 4096, "WebSocket write buffer size in bytes")
	startCmd.Flags().Bool("ws-compression", false, "Enable WebSocket compression")
	startCmd.Flags().Duration("ws-timeout", 10*time.Second, "WebSocket handshake timeout")
	startCmd.Flags().Duration("ws-ping", 30*time.Second, "WebSocket ping interval")

	viper.BindPFlag("ui_port", startCmd.Flags().Lookup("ui-port"))
	viper.BindPFlag("recording_mode", startCmd.Flags().Lookup("record"))
	viper.BindPFlag("replay_mode", startCmd.Flags().Lookup("replay"))
	viper.BindPFlag("tls.enabled", startCmd.Flags().Lookup("tls"))
	viper.BindPFlag("tls.cert_file", startCmd.Flags().Lookup("cert"))
	viper.BindPFlag("tls.key_file", startCmd.Flags().Lookup("key"))
	viper.BindPFlag("tls.port", startCmd.Flags().Lookup("tls-port"))
	viper.BindPFlag("tls.allow_insecure", startCmd.Flags().Lookup("insecure"))

	// Bind WebSocket flags to config
	viper.BindPFlag("websocket.enabled", startCmd.Flags().Lookup("websocket"))
	viper.BindPFlag("websocket.read_buffer_size", startCmd.Flags().Lookup("ws-read-buffer"))
	viper.BindPFlag("websocket.write_buffer_size", startCmd.Flags().Lookup("ws-write-buffer"))
	viper.BindPFlag("websocket.enable_compression", startCmd.Flags().Lookup("ws-compression"))
	viper.BindPFlag("websocket.handshake_timeout", startCmd.Flags().Lookup("ws-timeout"))
	viper.BindPFlag("websocket.ping_interval", startCmd.Flags().Lookup("ws-ping"))
}

func getMode(cfg *conf.Config) string {
	mode := "Passthrough"
	if cfg.RecordingMode {
		mode = "Recording"
	}
	if cfg.ReplayMode {
		mode = "Replay"
	}

	if cfg.TLS.Enabled {
		mode += " with TLS"
	}

	if cfg.WebSocket.Enabled {
		mode += " with WebSocket support"
	}

	return mode
}
