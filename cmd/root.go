package cmd

import (
	"fmt"
	"os"

	conf "github.com/dipjyotimetia/traffic-inspector/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	config  conf.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "traffic-inspector",
	Short: "A proxy server for recording and replaying HTTP traffic",
	Long: `Traffic Inspector is a tool that sits between clients and servers, 
recording HTTP traffic for later replay. It can operate in recording mode, 
replay mode, or simple passthrough mode.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.json)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	// Read environment variables prefixed with TRAFFIC_INSPECTOR_
	viper.SetEnvPrefix("traffic_inspector")
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("🔧 Using config file:", viper.ConfigFileUsed())
	}
}
