package bootstrap_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// minimalComposeYAML is enough for bootstrap (compose file exists); fake docker never parses it.
const minimalComposeYAML = `services:
  emusync:
    image: scratch
`

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func writeFakeDocker(t *testing.T, binDir, logFile string) string {
	t.Helper()
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + logFile + "\"\n" +
		"case \"$*\" in\n" +
		"  *compose*version*) exit 0 ;;\n" +
		"  *up*-d*--build*) exit 0 ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dockerPath
}

func runBootstrap(t *testing.T, repoRoot string, extraEnv []string, args ...string) *exec.Cmd {
	t.Helper()
	script := filepath.Join(projectRoot(t), "scripts", "bootstrap-server.sh")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), extraEnv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bootstrap: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	t.Logf("stdout: %s", string(out))
	return cmd
}

func TestBootstrapFailsWithoutDocker(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(projectRoot(t), "scripts", "bootstrap-server.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = repo
	// Hide docker (normally /usr/bin/docker); keep /bin for cat used by the script's heredocs.
	cmd.Env = append(os.Environ(), "EMUSYNC_REPO_ROOT="+repo, "PATH=/bin")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected failure without docker on PATH")
	}
	if !strings.Contains(stderr.String(), "docker") {
		t.Fatalf("expected docker hint on stderr, got: %q", stderr.String())
	}
}

func TestBootstrapFailsWhenComposeUnavailable(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	// docker exists but does not implement compose version
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(projectRoot(t), "scripts", "bootstrap-server.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "EMUSYNC_REPO_ROOT="+repo, "PATH="+binDir+string(os.PathListSeparator)+"/bin")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected failure when docker compose fails")
	}
	if !strings.Contains(stderr.String(), "compose") {
		t.Fatalf("expected compose hint on stderr, got: %q", stderr.String())
	}
}

func TestBootstrapCreatesEnvAndRunsCompose(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	logFile := filepath.Join(repo, "docker-invocations.log")
	writeFakeDocker(t, binDir, logFile)

	runBootstrap(t, repo, []string{
		"EMUSYNC_REPO_ROOT=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})

	envBytes, err := os.ReadFile(filepath.Join(repo, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSpace(string(envBytes))
	parseEnvTokens(t, s)

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logStr := string(logBytes)
	if !strings.Contains(logStr, "compose version") {
		t.Fatalf("expected compose version call, log:\n%s", logStr)
	}
	if !strings.Contains(logStr, "up -d --build") {
		t.Fatalf("expected compose up, log:\n%s", logStr)
	}
}

func TestBootstrapPreservesTokenByDefault(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	logFile := filepath.Join(repo, "docker-invocations.log")
	writeFakeDocker(t, binDir, logFile)
	envPrefix := []string{
		"EMUSYNC_REPO_ROOT=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}

	runBootstrap(t, repo, envPrefix)

	first, err := os.ReadFile(filepath.Join(repo, ".env"))
	if err != nil {
		t.Fatal(err)
	}

	runBootstrap(t, repo, envPrefix)

	second, err := os.ReadFile(filepath.Join(repo, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("token should be preserved; first=%q second=%q", string(first), string(second))
	}
}

func TestBootstrapForceTokenRotates(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	logFile := filepath.Join(repo, "docker-invocations.log")
	writeFakeDocker(t, binDir, logFile)
	envPrefix := []string{
		"EMUSYNC_REPO_ROOT=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}

	runBootstrap(t, repo, envPrefix)
	first := strings.TrimSpace(string(readFile(t, filepath.Join(repo, ".env"))))
	auth1, admin1 := parseEnvTokens(t, first)

	script := filepath.Join(projectRoot(t), "scripts", "bootstrap-server.sh")
	cmd := exec.Command("bash", script, "--force-token")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), envPrefix...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if _, err := cmd.Output(); err != nil {
		t.Fatalf("bootstrap --force-token: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Rotating") {
		t.Fatalf("expected rotation warning on stderr: %q", stderr.String())
	}

	secondRaw := strings.TrimSpace(string(readFile(t, filepath.Join(repo, ".env"))))
	auth2, admin2 := parseEnvTokens(t, secondRaw)

	if auth1 == auth2 {
		t.Fatal("expected auth token to change with --force-token")
	}
	if admin2 != admin1 {
		t.Fatalf("admin token should not rotate with --force-token; was %q now %q", admin1, admin2)
	}
}

func parseEnvTokens(t *testing.T, envContent string) (auth string, admin string) {
	t.Helper()
	auth = envValue(envContent, "EMUSYNC_AUTH_TOKEN")
	if auth == "" {
		t.Fatal("missing EMUSYNC_AUTH_TOKEN")
	}
	admin = envValue(envContent, "EMUSYNC_ADMIN_TOKEN")
	if admin == "" {
		t.Fatal("missing EMUSYNC_ADMIN_TOKEN")
	}
	return auth, admin
}

func envValue(envContent, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(envContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBootstrapAddsAdminTokenWhenLegacyEnv(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyEnv := "EMUSYNC_AUTH_TOKEN=fixed_legacy_auth_token_xx\n"
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte(legacyEnv), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	writeFakeDocker(t, binDir, filepath.Join(repo, "docker-invocations.log"))

	runBootstrap(t, repo, []string{
		"EMUSYNC_REPO_ROOT=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})

	raw := strings.TrimSpace(string(readFile(t, filepath.Join(repo, ".env"))))
	auth, admin := parseEnvTokens(t, raw)
	if auth != "fixed_legacy_auth_token_xx" {
		t.Fatalf("auth should stay legacy, got %q", auth)
	}
	if len(admin) < 16 {
		t.Fatalf("expected generated admin token, got %q", admin)
	}
}

func TestBootstrapEnvFilePermissions(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte(minimalComposeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	writeFakeDocker(t, binDir, filepath.Join(repo, "docker-invocations.log"))

	runBootstrap(t, repo, []string{
		"EMUSYNC_REPO_ROOT=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})

	fi, err := os.Stat(filepath.Join(repo, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf(".env should be group/other inaccessible, mode=%v", fi.Mode())
	}
}
