package sys1

import (
	"errors"
	"log/slog"
	"testing"
)

func TestNewPrecedence(t *testing.T) {
	t.Run("option beats env beats default", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "env-key")
		t.Setenv(EnvBaseURL, "https://env.example.com")
		t.Setenv(EnvModel, "env-model")

		c, err := New(
			WithAPIKey("opt-key"),
			WithBaseURL("https://opt.example.com/"),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if c.apiKey != "opt-key" {
			t.Errorf("apiKey = %q, want opt-key", c.apiKey)
		}
		if got := c.baseURL.String(); got != "https://opt.example.com" {
			t.Errorf("baseURL = %q, want https://opt.example.com", got)
		}
		// The model has no construction-time Option of its own; a
		// caller overrides the environment by deriving a client with
		// Client.WithModel instead.
		if got := c.WithModel("opt-model").model; got != "opt-model" {
			t.Errorf("WithModel(\"opt-model\").model = %q, want opt-model", got)
		}
	})

	t.Run("env beats default", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "env-key")
		t.Setenv(EnvBaseURL, "https://env.example.com/")
		t.Setenv(EnvModel, "env-model")

		c, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if c.apiKey != "env-key" {
			t.Errorf("apiKey = %q, want env-key", c.apiKey)
		}
		if got := c.baseURL.String(); got != "https://env.example.com" {
			t.Errorf("baseURL = %q, want https://env.example.com", got)
		}
		if c.model != "env-model" {
			t.Errorf("model = %q, want env-model", c.model)
		}
	})

	t.Run("default when nothing set", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "key-for-default-test")
		t.Setenv(EnvBaseURL, "")
		t.Setenv(EnvModel, "")
		t.Setenv(EnvLogLevel, "")

		c, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := c.baseURL.String(); got != DefaultBaseURL {
			t.Errorf("baseURL = %q, want %q", got, DefaultBaseURL)
		}
		if c.model != DefaultModel {
			t.Errorf("model = %q, want %q", c.model, DefaultModel)
		}
		if c.logger != nil {
			t.Error("logger set without TYPESAFE_LOG_LEVEL, want nil")
		}
	})

	t.Run("blank env ignored", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "key-for-blank-test")
		t.Setenv(EnvBaseURL, "   ")
		t.Setenv(EnvModel, "\t\n")
		t.Setenv(EnvLogLevel, " ")

		c, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := c.baseURL.String(); got != DefaultBaseURL {
			t.Errorf("baseURL = %q, want %q", got, DefaultBaseURL)
		}
		if c.model != DefaultModel {
			t.Errorf("model = %q, want %q", c.model, DefaultModel)
		}
		if c.logger != nil {
			t.Error("logger set with blank TYPESAFE_LOG_LEVEL, want nil")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "")
		if _, err := New(); !errors.Is(err, ErrMissingAPIKey) {
			t.Errorf("New() error = %v, want ErrMissingAPIKey", err)
		}
	})

	t.Run("invalid env base URL", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "key")
		t.Setenv(EnvBaseURL, "not a url")
		if _, err := New(); err == nil {
			t.Error("New() error = nil, want error for invalid base URL")
		}
	})
}

func TestNewLogLevel(t *testing.T) {
	cases := []struct {
		value   string
		enabled bool
		level   slog.Level
	}{
		{"debug", true, slog.LevelDebug},
		{"DEBUG", true, slog.LevelDebug},
		{"info", true, slog.LevelInfo},
		{"warning", true, slog.LevelWarn},
		{"warn", true, slog.LevelWarn},
		{"error", true, slog.LevelError},
		{"off", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(EnvAPIKey, "key")
			t.Setenv(EnvLogLevel, tc.value)

			c, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if !tc.enabled {
				if c.logger != nil {
					t.Fatal("logger set for level off, want nil")
				}
				return
			}
			if c.logger == nil {
				t.Fatal("logger nil, want one configured from the environment")
			}
			// The handler has to filter at exactly the configured level,
			// otherwise the env var isn't actually doing anything.
			if !c.logger.Enabled(t.Context(), tc.level) {
				t.Errorf("logger does not enable %v", tc.level)
			}
			if tc.level > slog.LevelDebug && c.logger.Enabled(t.Context(), tc.level-1) {
				t.Errorf("logger enables %v, which is below the configured %v", tc.level-1, tc.level)
			}
		})
	}

	t.Run("unknown value", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "key")
		t.Setenv(EnvLogLevel, "verbose")
		if _, err := New(); err == nil {
			t.Error("New() error = nil, want error for unknown log level")
		}
	})

	t.Run("explicit logger wins", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "key")
		t.Setenv(EnvLogLevel, "debug")
		own := slog.New(slog.DiscardHandler)
		c, err := New(WithLogger(own))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if c.logger != own {
			t.Error("WithLogger did not override the environment logger")
		}
	})
}
