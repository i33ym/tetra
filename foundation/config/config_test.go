package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationUnmarshal(t *testing.T) {
	toml := `
[http]
read_timeout = "7s"
idle_timeout = "2m"

[worker]
poll_interval = "250ms"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(toml), 0o600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got, want := cfg.HTTP.ReadTimeout.Duration, 7*time.Second; got != want {
		t.Errorf("ReadTimeout = %v, want %v", got, want)
	}
	if got, want := cfg.HTTP.IdleTimeout.Duration, 2*time.Minute; got != want {
		t.Errorf("IdleTimeout = %v, want %v", got, want)
	}
	if got, want := cfg.Worker.PollInterval.Duration, 250*time.Millisecond; got != want {
		t.Errorf("PollInterval = %v, want %v", got, want)
	}

	// A field absent from the file must keep its default.
	if got, want := cfg.DB.User, "postgres"; got != want {
		t.Errorf("DB.User = %q, want default %q", got, want)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("TETRA_DB_HOST", "db.internal:5432")
	t.Setenv("TETRA_WORKER_INPROC", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got, want := cfg.DB.Host, "db.internal:5432"; got != want {
		t.Errorf("DB.Host = %q, want %q", got, want)
	}
	if !cfg.Worker.InProc {
		t.Errorf("Worker.InProc = false, want true")
	}
}

func TestStringMasksSecrets(t *testing.T) {
	cfg, _ := Load("")
	out := cfg.String()
	if contains(out, "postgres") && !contains(out, "xxxxxx") {
		t.Errorf("expected password to be masked in %q", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
