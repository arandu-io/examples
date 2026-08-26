package unit_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	hconfig "github.com/arandu-io/hesape/config"

	appconfig "github.com/arandu-io/examples/config"
)

func TestConfigurationRejectsAnInvalidBoolean(t *testing.T) {
	clearParsedConfiguration(t)
	t.Setenv("SESSION_SECURE", "sometimes")

	_, err := appconfig.From(configurationBase())

	if err == nil || err.Error() != `SESSION_SECURE must be a boolean, got "sometimes"` {
		t.Fatalf("error = %v, want the invalid SESSION_SECURE value", err)
	}
}

func TestConfigurationRejectsAnInvalidInteger(t *testing.T) {
	prepareConfigurationLoad(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "many")

	_, err := appconfig.Load()

	if err == nil || err.Error() != `DB_MAX_OPEN_CONNS must be an integer, got "many"` {
		t.Fatalf("error = %v, want the invalid DB_MAX_OPEN_CONNS value", err)
	}
}

func TestConfigurationRejectsANonPositiveDuration(t *testing.T) {
	clearParsedConfiguration(t)
	t.Setenv("SESSION_TTL", "0")

	_, err := appconfig.From(configurationBase())

	want := `SESSION_TTL must be a positive number of seconds, got "0"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestConfigurationRejectsAnInvalidMailURL(t *testing.T) {
	clearParsedConfiguration(t)
	t.Setenv("MAIL_URL", "://broken")

	_, err := appconfig.From(configurationBase())

	if err == nil || err.Error() != `MAIL_URL is not a URL: "://broken"` {
		t.Fatalf("error = %v, want the invalid MAIL_URL value", err)
	}
}

func TestConfigurationRejectsARetiredMailVariable(t *testing.T) {
	clearParsedConfiguration(t)
	t.Setenv("MAIL_HOST", "smtp.example.com")

	_, err := appconfig.From(configurationBase())

	want := "MAIL_HOST is retired; remove it and configure the mailer with MAIL_URL"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestConfigurationRejectsInvalidClosedDiscriminators(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "cache Redis without its URL",
			env:  map[string]string{"CACHE_STORE": "redis"},
			want: []string{"CACHE_STORE", `"redis"`, "REDIS_URL"},
		},
		{
			name: "KV session without its URL",
			env:  map[string]string{"SESSION_DRIVER": "kv"},
			want: []string{"SESSION_DRIVER", `"kv"`, "REDIS_URL"},
		},
		{
			name: "Redis queue without its URL",
			env:  map[string]string{"QUEUE_CONNECTION": "redis"},
			want: []string{"QUEUE_CONNECTION", `"redis"`, "REDIS_URL"},
		},
		{
			name: "unknown cache store",
			env:  map[string]string{"CACHE_STORE": "memcached"},
			want: []string{"CACHE_STORE", `"memcached"`, "memory", "redis"},
		},
		{
			name: "unknown session driver",
			env:  map[string]string{"SESSION_DRIVER": "database"},
			want: []string{"SESSION_DRIVER", `"database"`, "memory", "kv"},
		},
		{
			name: "unknown queue connection",
			env:  map[string]string{"QUEUE_CONNECTION": "sqs"},
			want: []string{"QUEUE_CONNECTION", `"sqs"`, "database", "redis"},
		},
		{
			name: "unknown filesystem disk",
			env:  map[string]string{"FILESYSTEM_DISK": "gcs"},
			want: []string{"FILESYSTEM_DISK", `"gcs"`, "local", "r2", "s3"},
		},
		{
			name: "unknown mailer",
			env:  map[string]string{"MAIL_URL": "ses://secret"},
			want: []string{"MAIL_URL", `"ses"`, "unsupported"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearParsedConfiguration(t)
			for key, value := range test.env {
				t.Setenv(key, value)
			}

			_, err := appconfig.From(configurationBase())
			if err == nil {
				t.Fatal("From accepted an invalid discriminator")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestFromRejectsInvalidFrameworkConfigurationWithoutPanicking(t *testing.T) {
	clearParsedConfiguration(t)

	_, err := appconfig.From(bootstrap.Configuration{})

	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("error = %v, want it to identify APP_ENV", err)
	}
}

func TestLoadPreservesTheFrameworkBootstrapError(t *testing.T) {
	prepareConfigurationLoad(t)
	t.Setenv("APP_KEY", "short")
	t.Setenv("SESSION_SECURE", "sometimes")

	_, err := appconfig.Load()

	if err == nil || !strings.Contains(err.Error(), "APP_KEY") {
		t.Fatalf("error = %v, want the APP_KEY bootstrap error", err)
	}
	if strings.Contains(err.Error(), "SESSION_SECURE") {
		t.Fatalf("error = %q, and the later application error masked the bootstrap error", err)
	}
}

func prepareConfigurationLoad(t *testing.T) {
	t.Helper()
	clearParsedConfiguration(t)
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
}

func TestConfigurationUsesDefaultsForExplicitlyEmptyValues(t *testing.T) {
	clearParsedConfiguration(t)

	cfg, err := appconfig.From(configurationBase())
	if err != nil {
		t.Fatalf("From: %v", err)
	}

	if cfg.Session.Secure || cfg.Session.TTL != 12*time.Hour || cfg.Session.CSRFTTL != 2*time.Hour {
		t.Errorf("session defaults = secure %t, TTL %s, CSRF TTL %s", cfg.Session.Secure, cfg.Session.TTL, cfg.Session.CSRFTTL)
	}
	if cfg.Database.MaxOpenConns != 25 || cfg.Database.MaxIdleConns != 5 || cfg.Database.ConnMaxLifetime != time.Hour {
		t.Errorf("database defaults = %d open, %d idle, %s lifetime",
			cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	}
	if cfg.Mail.Mailer != appconfig.MailerLog {
		t.Errorf("mail default = %q, want %q", cfg.Mail.Mailer, appconfig.MailerLog)
	}
	if cfg.Queue.Workers != 4 || cfg.Queue.RetryAfter != 90*time.Second || cfg.Queue.MaxAttempts != 5 {
		t.Errorf("queue defaults = %d workers, %s retry, %d attempts",
			cfg.Queue.Workers, cfg.Queue.RetryAfter, cfg.Queue.MaxAttempts)
	}
}

func configurationBase() bootstrap.Configuration {
	return bootstrap.Configuration{App: hconfig.App{
		Name:     "test",
		Env:      hconfig.EnvDev,
		URL:      &url.URL{Scheme: "http", Host: "localhost"},
		Timezone: time.UTC,
		Key:      []byte("0123456789abcdef0123456789abcdef"),
	}}
}

func clearParsedConfiguration(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CACHE_STORE", "SESSION_DRIVER", "QUEUE_CONNECTION", "FILESYSTEM_DISK",
		"REDIS_URL",
		"SESSION_SECURE",
		"SESSION_TTL", "CSRF_TTL",
		"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME",
		"CACHE_TTL",
		"QUEUE_WORKERS", "QUEUE_RETRY_AFTER", "QUEUE_MAX_ATTEMPTS",
		"LOG_SAMPLING",
		"AUTH_PASSWORD_MIN_LENGTH", "AUTH_PASSWORD_RESET_TTL",
		"MAIL_URL",
		"MAIL_MAILER", "MAIL_HOST", "MAIL_PORT", "MAIL_USERNAME",
		"MAIL_PASSWORD", "MAIL_ENCRYPTION", "MAIL_KEY",
	} {
		t.Setenv(name, "")
	}
}
