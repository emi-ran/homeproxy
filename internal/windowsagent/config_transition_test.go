package windowsagent

import (
	"strings"
	"testing"
)

// These tests deliberately have no Windows build tag: deciding whether local
// IPC needs persistence or a tunnel restart must not require DPAPI or a dial.
func TestConfigTransition(t *testing.T) {
	base := Config{
		Address:     "example.com:4433",
		ID:          "test-pc",
		Token:       "test-token-0123456789",
		Fingerprint: strings.Repeat("ab", 32),
		Enabled:     true,
	}
	redacted := base
	redacted.Token = ""
	formatted := redacted
	formatted.Fingerprint = "  " + strings.TrimSuffix(strings.Repeat("AB:", 32), ":") + "  "
	disabled := base
	disabled.Enabled = false
	changed := base
	changed.Address = "other.example:4433"
	newToken := base
	newToken.Token = "replacement-token-0123456789"

	cases := []struct {
		name                          string
		current                       Config
		configured                    bool
		desired                       Config
		started                       bool
		wantConfig                    Config
		wantSave, wantStop, wantStart bool
	}{
		{"same enabled settings", base, true, base, true, base, false, false, false},
		{"redacted token preserves effective settings", base, true, redacted, true, base, false, false, false},
		{"fingerprint representation is not a connection change", base, true, formatted, true, base, false, false, false},
		// Connect supplies current settings with Enabled=true. Started includes
		// connecting and retry-backoff, not merely an authenticated connection.
		{"connect while already started", base, true, base, true, base, false, false, false},
		{"connect recovers genuinely stopped agent", base, true, base, false, base, false, false, true},
		{"connect enables saved disabled agent", disabled, true, base, false, base, true, false, true},
		{"same disabled settings", disabled, true, disabled, false, disabled, false, false, false},
		{"disconnect stops existing agent", base, true, disabled, true, disabled, true, true, false},
		{"disabled saved state still stops live agent", disabled, true, disabled, true, disabled, false, true, false},
		{"endpoint change restarts live agent", base, true, changed, true, changed, true, true, true},
		{"token change restarts live agent", base, true, newToken, true, newToken, true, true, true},
		{"changed enabled config starts stopped agent", base, true, changed, false, changed, true, false, true},
		{"first config persists and starts", Config{}, false, base, false, base, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planConfigTransition(tc.current, tc.configured, tc.desired, tc.started)
			if err != nil {
				t.Fatalf("valid transition rejected: %v", err)
			}
			if got.Config != tc.wantConfig {
				// Do not print credential-bearing configs on failure.
				t.Fatal("effective config does not match normalized expected settings")
			}
			if got.Save != tc.wantSave || got.Stop != tc.wantStop || got.Start != tc.wantStart {
				t.Fatalf("actions save/stop/start = %t/%t/%t; want %t/%t/%t",
					got.Save, got.Stop, got.Start, tc.wantSave, tc.wantStop, tc.wantStart)
			}
		})
	}
}

func TestConfigTransitionRejectsInvalidEffectiveConfig(t *testing.T) {
	base := Config{Address: "example.com:4433", ID: "test-pc", Token: "test-token-0123456789", Fingerprint: strings.Repeat("ab", 32), Enabled: true}
	for _, tc := range []struct {
		name       string
		configured bool
		mutate     func(*Config)
	}{
		{"missing initial token", false, func(c *Config) { c.Token = "" }},
		{"explicit invalid replacement token", true, func(c *Config) { c.Token = "short" }},
		{"invalid endpoint", true, func(c *Config) { c.Address = "host:0" }},
		{"invalid fingerprint", true, func(c *Config) { c.Fingerprint = "invalid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desired := base
			tc.mutate(&desired)
			got, err := planConfigTransition(base, tc.configured, desired, true)
			if err == nil {
				t.Fatal("invalid effective config accepted")
			}
			if got.Save || got.Stop || got.Start {
				t.Fatal("invalid config must not request persistence or lifecycle effects")
			}
		})
	}
}
