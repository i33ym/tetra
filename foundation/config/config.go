// Package config loads service configuration from a TOML file with optional
// environment-variable overrides (which take precedence so k8s ConfigMaps and
// Secrets win over the baked-in file).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration wraps time.Duration so TOML strings like "5s" decode via the encoding.
// TextUnmarshaler interface (BurntSushi/toml does not parse durations natively).
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Config is the full service configuration.
type Config struct {
	HTTP      HTTP      `toml:"http"`
	DB        DB        `toml:"db"`
	MinIO     MinIO     `toml:"minio"`
	Otel      Otel      `toml:"otel"`
	Worker    Worker    `toml:"worker"`
	Processor Processor `toml:"processor"`
}

// HTTP holds the API/debug server configuration.
type HTTP struct {
	APIHost           string   `toml:"api_host"`
	DebugHost         string   `toml:"debug_host"`
	ReadTimeout       Duration `toml:"read_timeout"`
	ReadHeaderTimeout Duration `toml:"read_header_timeout"`
	WriteTimeout      Duration `toml:"write_timeout"`
	IdleTimeout       Duration `toml:"idle_timeout"`
	ShutdownTimeout   Duration `toml:"shutdown_timeout"`
	CORSOrigins       []string `toml:"cors_origins"`
	MaxUploadBytes    int64    `toml:"max_upload_bytes"`
}

// DB holds the Postgres configuration.
type DB struct {
	User         string `toml:"user"`
	Password     string `toml:"password"`
	Host         string `toml:"host"`
	Name         string `toml:"name"`
	MaxOpenConns int    `toml:"max_open_conns"`
	MaxIdleConns int    `toml:"max_idle_conns"`
	DisableTLS   bool   `toml:"disable_tls"`
}

// MinIO holds the object-storage configuration.
type MinIO struct {
	Endpoint  string `toml:"endpoint"`
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
	Bucket    string `toml:"bucket"`
	UseSSL    bool   `toml:"use_ssl"`
	Region    string `toml:"region"`
}

// Otel holds the tracing configuration.
type Otel struct {
	ServiceName string  `toml:"service_name"`
	Version     string  `toml:"version"`
	Host        string  `toml:"host"`
	Probability float64 `toml:"probability"`
}

// Worker holds the background-worker configuration.
type Worker struct {
	Concurrency  int      `toml:"concurrency"`
	PollInterval Duration `toml:"poll_interval"`
	LeaseSeconds int      `toml:"lease_seconds"`
	MaxAttempts  int      `toml:"max_attempts"`
	BackoffBase  Duration `toml:"backoff_base"`
	InProc       bool     `toml:"inproc"`
}

// Processor holds the mock external processor client configuration.
type Processor struct {
	URL     string   `toml:"url"`
	Timeout Duration `toml:"timeout"`
}

// defaults returns a Config pre-populated with sensible defaults so a TOML file
// only needs to override what differs.
func defaults() Config {
	return Config{
		HTTP: HTTP{
			APIHost:   "0.0.0.0:3000",
			DebugHost: "0.0.0.0:3010",
			// Read/Write timeouts are 0 (unlimited) so large file uploads and
			// downloads are not cut off; slowloris on headers is guarded by
			// ReadHeaderTimeout and idle connections by IdleTimeout.
			ReadTimeout:       Duration{0},
			ReadHeaderTimeout: Duration{10 * time.Second},
			WriteTimeout:      Duration{0},
			IdleTimeout:       Duration{120 * time.Second},
			ShutdownTimeout:   Duration{20 * time.Second},
			CORSOrigins:       []string{"*"},
			MaxUploadBytes:    100 << 20, // 100 MiB
		},
		DB: DB{
			User:         "postgres",
			Password:     "postgres",
			Host:         "localhost:5432",
			Name:         "tetra",
			MaxOpenConns: 25,
			MaxIdleConns: 25,
			DisableTLS:   true,
		},
		MinIO: MinIO{
			Endpoint:  "localhost:9000",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    "tetra-payloads",
			UseSSL:    false,
			Region:    "us-east-1",
		},
		Otel: Otel{
			ServiceName: "tetra",
			Version:     "develop",
			Host:        "localhost:4317",
			Probability: 1.0,
		},
		Worker: Worker{
			Concurrency:  8,
			PollInterval: Duration{500 * time.Millisecond},
			LeaseSeconds: 60,
			MaxAttempts:  5,
			BackoffBase:  Duration{5 * time.Second},
			InProc:       false,
		},
		Processor: Processor{
			URL:     "http://localhost:7000/process",
			Timeout: Duration{30 * time.Second},
		},
	}
}

// Load reads configuration from the given TOML file (if path is non-empty),
// layering it over the defaults, then applies environment overrides.
func Load(path string) (Config, error) {
	cfg := defaults()

	if path != "" {
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return Config{}, fmt.Errorf("decoding config %q: %w", path, err)
		}
	}

	applyEnvOverrides(&cfg)

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.HTTP.APIHost = envStr("TETRA_HTTP_API_HOST", cfg.HTTP.APIHost)
	cfg.HTTP.DebugHost = envStr("TETRA_HTTP_DEBUG_HOST", cfg.HTTP.DebugHost)

	cfg.DB.User = envStr("TETRA_DB_USER", cfg.DB.User)
	cfg.DB.Password = envStr("TETRA_DB_PASSWORD", cfg.DB.Password)
	cfg.DB.Host = envStr("TETRA_DB_HOST", cfg.DB.Host)
	cfg.DB.Name = envStr("TETRA_DB_NAME", cfg.DB.Name)
	cfg.DB.DisableTLS = envBool("TETRA_DB_DISABLE_TLS", cfg.DB.DisableTLS)

	cfg.MinIO.Endpoint = envStr("TETRA_MINIO_ENDPOINT", cfg.MinIO.Endpoint)
	cfg.MinIO.AccessKey = envStr("TETRA_MINIO_ACCESS_KEY", cfg.MinIO.AccessKey)
	cfg.MinIO.SecretKey = envStr("TETRA_MINIO_SECRET_KEY", cfg.MinIO.SecretKey)
	cfg.MinIO.Bucket = envStr("TETRA_MINIO_BUCKET", cfg.MinIO.Bucket)
	cfg.MinIO.UseSSL = envBool("TETRA_MINIO_USE_SSL", cfg.MinIO.UseSSL)

	cfg.Otel.ServiceName = envStr("TETRA_OTEL_SERVICE_NAME", cfg.Otel.ServiceName)
	cfg.Otel.Host = envStr("TETRA_OTEL_HOST", cfg.Otel.Host)

	cfg.Worker.InProc = envBool("TETRA_WORKER_INPROC", cfg.Worker.InProc)

	cfg.Processor.URL = envStr("TETRA_PROCESSOR_URL", cfg.Processor.URL)
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// String renders the configuration for logging with the DB password and MinIO
// secret key masked. The plain alias strips the String method so %+v formats
// the fields instead of recursing into this method.
func (c Config) String() string {
	type plain Config

	masked := c
	masked.DB.Password = "xxxxxx"
	masked.MinIO.SecretKey = "xxxxxx"

	return fmt.Sprintf("%+v", plain(masked))
}
