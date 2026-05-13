package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dublin/emusync/internal/config"
)

func resetRootArgsAndIO(t *testing.T) {
	t.Helper()
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestCLI_Help(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"--help"})
	if err := Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestCLI_ServerFindable(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	cmd, _, err := rootCmd.Find([]string{"server"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cmd.Name() != "server" {
		t.Fatalf("command name = %q, want server", cmd.Name())
	}
}

func TestCLI_SetupFindable(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	cmd, _, err := rootCmd.Find([]string{"setup"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cmd.Name() != "setup" {
		t.Fatalf("command name = %q, want setup", cmd.Name())
	}
}

func TestInit_WritesMinimalStarterConfig(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfgPath := filepath.Join(dir, ".config", "emusync", "config.toml")

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"init", "--config", cfgPath})
	if err := Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Emulators) != 0 {
		t.Fatalf("expected skeleton without emulators, got %d entries", len(cfg.Emulators))
	}
	if cfg.Server.AuthToken != "configure-with-emusync-setup" {
		t.Fatalf("placeholder token %q", cfg.Server.AuthToken)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("host %q", cfg.Server.Host)
	}
}

func TestInit_ErrorWhenAlreadyExists(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgFile := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[server]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"init", "--config", cfgFile})
	err := Execute()
	if err == nil {
		t.Fatal("expected error when config exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInit_AcceptsVerboseFlag(t *testing.T) {
	t.Cleanup(func() { resetRootArgsAndIO(t) })
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgFile := filepath.Join(dir, "config.toml")

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"init", "-v", "--config", cfgFile})
	if err := Execute(); err != nil {
		t.Fatal(err)
	}
}
