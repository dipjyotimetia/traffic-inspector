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

	viper.BindPFlag("recording_mode", startCmd.Flags().Lookup("record"))
	viper.BindPFlag("replay_mode", startCmd.Flags().Lookup("replay"))
	viper.BindPFlag("tls.enabled", startCmd.Flags().Lookup("tls"))
	viper.BindPFlag("tls.cert_file", startCmd.Flags().Lookup("cert"))
	viper.BindPFlag("tls.key_file", startCmd.Flags().Lookup("key"))
	viper.BindPFlag("tls.port", startCmd.Flags().Lookup("tls-port"))
	viper.BindPFlag("tls.allow_insecure", startCmd.Flags().Lookup("insecure"))
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

	return mode
}
