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

func TestInit_WritesDefaultConfig(t *testing.T) {
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

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	wantContent := strings.TrimSpace(config.DefaultConfigContent())
	if got != wantContent {
		t.Fatal("generated config differs from DefaultConfigContent()")
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
