package setup_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/discovery"
	"github.com/dublin/emusync/internal/setup"
)

func TestRunWizard_writesFullConfig(t *testing.T) {
	home := t.TempDir()
	em := filepath.Join(home, "Emulation", "saves")
	if err := os.MkdirAll(filepath.Join(em, "retroarch"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(home, "config.toml")
	input := strings.Join([]string{
		"192.168.1.50",
		"8080",
		"secret-token",
		"testdevice",
		"1",
		"y",
	}, "\n") + "\n"

	err := setup.RunWizard(setup.WizardOptions{
		CfgPath:    cfgPath,
		Force:      true,
		HomeDir:    home,
		In:         strings.NewReader(input),
		Out:        io.Discard,
		ErrOut:     io.Discard,
		Discover:   func(context.Context, time.Duration) []discovery.Server { return nil },
		LookupWait: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "192.168.1.50" || cfg.Server.Port != 8080 || cfg.Server.AuthToken != "secret-token" {
		t.Fatalf("server config: %+v", cfg.Server)
	}
	if cfg.Client.DeviceID != "testdevice" {
		t.Fatalf("device id %q", cfg.Client.DeviceID)
	}
	if len(cfg.Emulators) == 0 {
		t.Fatal("expected default emulators")
	}
	if !strings.HasPrefix(cfg.Client.SavesPath, "~") && !filepath.IsAbs(cfg.Client.SavesPath) {
		t.Fatalf("unexpected saves path %q", cfg.Client.SavesPath)
	}
}

func TestRunWizard_rejectsExistingWithoutForce(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("."), 0o644); err != nil {
		t.Fatal(err)
	}
	err := setup.RunWizard(setup.WizardOptions{
		CfgPath:    cfgPath,
		Force:      false,
		HomeDir:    home,
		In:         strings.NewReader("\n"),
		Out:        io.Discard,
		ErrOut:     io.Discard,
		Discover:   func(context.Context, time.Duration) []discovery.Server { return nil },
		LookupWait: time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected err: %v", err)
	}
}
