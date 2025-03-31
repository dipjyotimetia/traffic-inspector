package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	conf "github.com/dipjyotimetia/traffic-inspector/config"
	"github.com/dipjyotimetia/traffic-inspector/internal/db"
	"github.com/dipjyotimetia/traffic-inspector/internal/proxy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the traffic inspector proxy server",
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		cfg, err := conf.LoadConfig(viper.GetViper())
		if err != nil {
			log.Fatalf("❌ Failed to load configuration: %v", err)
		}
		log.Printf("🔧 Configuration loaded: Mode=%s", getMode(cfg))

		// Initialize database
		database, stmt, err := db.Initialize(cfg.SQLiteDBPath)
		if err != nil {
			log.Fatalf("❌ Failed to initialize database: %v", err)
		}
		defer database.Close()
		defer stmt.Close()

		// Set up signal handling for graceful shutdown
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer cancel()

		var wg sync.WaitGroup

		// Start HTTP proxy server
		var httpServer proxy.Server
		if cfg.HTTPPort > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				httpServer = proxy.StartHTTPProxy(ctx, cfg, database, stmt)
			}()
		}

		// Wait for shutdown signal
		<-ctx.Done()
		log.Println("🚨 Shutdown signal received, initiating graceful shutdown...")

		// Graceful shutdown with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown HTTP server
		if httpServer != nil {
			log.Println("⏳ Shutting down HTTP server...")
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("⚠️ HTTP server shutdown error: %v", err)
			} else {
				log.Println("✅ HTTP server stopped gracefully")
			}
		}

		// Wait for server goroutines to finish completely
		wg.Wait()
		log.Println("🏁 All servers stopped")
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Command-specific flags
	startCmd.Flags().BoolP("record", "r", false, "Run in recording mode")
	startCmd.Flags().BoolP("replay", "p", false, "Run in replay mode")

	// Bind flags to viper
	viper.BindPFlag("recording_mode", startCmd.Flags().Lookup("record"))
	viper.BindPFlag("replay_mode", startCmd.Flags().Lookup("replay"))
}

func getMode(cfg *conf.Config) string {
	if cfg.RecordingMode {
		return "Recording"
	}
	if cfg.ReplayMode {
		return "Replay"
	}
	return "Passthrough"
}
