package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/discovery"
	"github.com/dublin/emusync/internal/model"
)

// WizardOptions configures interactive setup (dependency injection for tests).
type WizardOptions struct {
	CfgPath string
	Force   bool
	HomeDir string
	In      io.Reader
	Out     io.Writer
	ErrOut  io.Writer
	// Discover defaults to discovery.Lookup when nil.
	Discover func(context.Context, time.Duration) []discovery.Server
	// LookupWait defaults to 2s.
	LookupWait time.Duration
}

// RunWizard writes ~/.config/emusync/config.toml after prompting.
func RunWizard(opts WizardOptions) error {
	if opts.LookupWait <= 0 {
		opts.LookupWait = 2 * time.Second
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ErrOut == nil {
		opts.ErrOut = opts.Out
	}

	if opts.CfgPath == "" {
		return fmt.Errorf("config path is required")
	}

	if _, err := os.Stat(opts.CfgPath); err == nil && !opts.Force {
		return fmt.Errorf("config already exists at %s (delete it, use --force, or edit manually)", opts.CfgPath)
	}
	if err := os.MkdirAll(filepath.Dir(opts.CfgPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
	}

	r := bufio.NewReader(opts.In)
	readLine := func(def string) (string, error) {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		s := strings.TrimSpace(line)
		if s == "" {
			return def, nil
		}
		return s, nil
	}

	fmt.Fprintf(opts.ErrOut, "Searching LAN for emusync servers (mDNS, ~%ds)...\n", int(opts.LookupWait.Round(time.Second)))
	var servers []discovery.Server
	if opts.Discover != nil {
		servers = opts.Discover(context.Background(), opts.LookupWait)
	} else {
		servers = discovery.Lookup(context.Background(), opts.LookupWait)
	}

	var host string
	var port int

	if len(servers) > 0 {
		for i, s := range servers {
			fmt.Fprintf(opts.ErrOut, "  [%d] http://%s:%d (%s)\n", i+1, s.Host, s.Port, s.Instance)
		}
		fmt.Fprintf(opts.ErrOut, "  [m] Enter host manually\n")
		fmt.Fprintf(opts.ErrOut, "Choice [1]: ")
		ch, err := readLine("1")
		if err != nil {
			return err
		}
		ch = strings.ToLower(strings.TrimSpace(ch))
		if ch != "m" && ch != "manual" {
			idx, err := strconv.Atoi(ch)
			if err != nil || idx < 1 || idx > len(servers) {
				return fmt.Errorf("invalid choice %q", ch)
			}
			host = servers[idx-1].Host
			port = servers[idx-1].Port
		}
	}

	if host == "" {
		fmt.Fprintf(opts.ErrOut, "Server host [127.0.0.1]: ")
		h, err := readLine("127.0.0.1")
		if err != nil {
			return err
		}
		host = strings.TrimSpace(h)
		fmt.Fprintf(opts.ErrOut, "Port [8080]: ")
		ps, err := readLine("8080")
		if err != nil {
			return err
		}
		p, err := strconv.Atoi(strings.TrimSpace(ps))
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port %q", ps)
		}
		port = p
	}

	fmt.Fprintf(opts.ErrOut, "Auth token (from server bootstrap output): ")
	token, err := readLine("")
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("auth token is required")
	}

	defDev, _ := os.Hostname()
	defDev = config.SanitizeDeviceID(defDev)
	fmt.Fprintf(opts.ErrOut, "Device ID [%s]: ", defDev)
	deviceID, err := readLine(defDev)
	if err != nil {
		return err
	}
	deviceID = config.SanitizeDeviceID(deviceID)

	defaultSaves := "~/Emulation/saves"
	var savesForCfg string

	roots := FindSaveRoots(home)
	if len(roots) == 0 {
		fmt.Fprintf(opts.ErrOut, "Saves root path [%s]: ", defaultSaves)
		sp, err := readLine(defaultSaves)
		if err != nil {
			return err
		}
		savesAbs := expandPathAgainstHome(strings.TrimSpace(sp), home)
		savesForCfg = ShortenHome(savesAbs, home)
	} else {
		for i, root := range roots {
			fmt.Fprintf(opts.ErrOut, "  [%d] %s — %s\n", i+1, root.Path, root.Reason)
		}
		fmt.Fprintf(opts.ErrOut, "  [c] Custom path\n")
		fmt.Fprintf(opts.ErrOut, "Choose saves root [1]: ")
		ch, err := readLine("1")
		if err != nil {
			return err
		}
		ch = strings.ToLower(strings.TrimSpace(ch))
		switch ch {
		case "c", "custom":
			fmt.Fprintf(opts.ErrOut, "Path to saves root: ")
			sp, err := readLine("")
			if err != nil {
				return err
			}
			sp = strings.TrimSpace(sp)
			if sp == "" {
				return fmt.Errorf("path required")
			}
			savesAbs := expandPathAgainstHome(sp, home)
			savesForCfg = ShortenHome(savesAbs, home)
		default:
			idx, err := strconv.Atoi(ch)
			if err != nil || idx < 1 || idx > len(roots) {
				return fmt.Errorf("invalid choice %q", ch)
			}
			savesForCfg = ShortenHome(roots[idx-1].Path, home)
		}
	}

	fmt.Fprintf(opts.ErrOut, "Include bundled emulator directory mappings? [Y/n]: ")
	inc, err := readLine("y")
	if err != nil {
		return err
	}
	var emulators []model.EmulatorConfig
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(inc)), "n") {
		fmt.Fprintf(opts.ErrOut, "Continuing with no emulator mappings (you can add [[emulators]] later).\n")
	} else {
		emulators = append(emulators, config.DefaultEmulators()...)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:      host,
			Port:      port,
			AuthToken: token,
		},
		Client: config.ClientConfig{
			DeviceID:        deviceID,
			SavesPath:       savesForCfg,
			BackupPath:      "",
			MaxLocalBackups: 0,
		},
		Sync: config.SyncConfig{
			AutoSyncOnClose:  true,
			AutoSyncOnLaunch: true,
			ConflictStrategy: "prompt",
			PollIntervalMs:   0,
			PostExitDelayMs:  0,
		},
		Emulators: emulators,
	}

	if err := config.Save(opts.CfgPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(opts.ErrOut, "\nWrote %s\nStart sync with: emusync watch\n", opts.CfgPath)
	return nil
}

func expandPathAgainstHome(path, home string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		return filepath.Clean(filepath.Join(home, path[2:]))
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(home, path))
}
