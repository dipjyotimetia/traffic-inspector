package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// initStartFlags initializes the flags for the start command
func initStartFlags() {
	startCmd.Flags().BoolP("record", "r", false, "Enable recording mode")
	startCmd.Flags().BoolP("replay", "p", false, "Enable replay mode")
	startCmd.Flags().BoolP("tls", "t", false, "Enable TLS")
	startCmd.Flags().String("cert", "", "TLS certificate file path")
	startCmd.Flags().String("key", "", "TLS key file path")
	startCmd.Flags().Int("tls-port", 443, "TLS port")
}

func TestVersionCommand(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute version command
	rootCmd.SetArgs([]string{"version"})
	err := rootCmd.Execute()

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Errorf("version command failed: %v", err)
	}

	// Check that output contains version info
	if !strings.Contains(output, "Traffic Inspector v") {
		t.Errorf("version command output does not contain version info: %s", output)
	}
}

func TestCertificateCommand(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "cert_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Capture stdout
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	// Execute certificate command with temp directory
	rootCmd.SetArgs([]string{
		"certificate",
		"--cert-dir", tempDir,
	})

	err = rootCmd.Execute()

	// Restore stdout
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("certificate command failed: %v", err)
	}

	// Check that files were created
	certPath := tempDir + "/server.crt"
	keyPath := tempDir + "/server.key"

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Errorf("Certificate file was not created at %s", certPath)
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("Key file was not created at %s", keyPath)
	}
}

func TestRootCommand(t *testing.T) {
	// Test with no args (should not error)
	rootCmd.SetArgs([]string{})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("root command failed: %v", err)
	}

	// Test with help flag
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"--help"})
	rootCmd.Execute()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "traffic-inspector") {
		t.Errorf("help output does not contain expected text: %s", output)
	}
}

func TestStartCommandWithMockDB(t *testing.T) {
	// This would be a more complex test involving mocked database
	// and capturing the HTTP server behavior. For this test, we'll
	// just verify command parsing works without actual server start.

	// Reset viper settings
	viper.Reset()

	// Temporarily redirect stdout/stderr
	oldOut := os.Stdout
	oldErr := os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	os.Stderr = devNull

	// Mock command execution without actually starting server
	// This is just testing flag parsing, not actual execution
	rootCmd.SetArgs([]string{"start", "--record", "--tls-port", "9443"})

	// Execute will fail without a valid db, but we just want to verify flags are set
	rootCmd.Execute()

	// Restore stdout/stderr
	os.Stdout = oldOut
	os.Stderr = oldErr

	// Verify flags were processed by checking viper values
	if !viper.GetBool("recording_mode") {
		t.Error("recording_mode flag not set properly")
	}

	if viper.GetInt("tls.port") != 9443 {
		t.Errorf("tls-port flag not set properly, got: %d, want: 9443", viper.GetInt("tls.port"))
	}
}
