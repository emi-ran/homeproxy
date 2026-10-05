package windowsagent

import "testing"

func TestConfigValidation(t *testing.T) {
	good := Config{Address: "example.com:4433", ID: "ev-pc", Token: "0123456789abcdef", Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	insecure := good
	insecure.Insecure, insecure.Fingerprint = true, ""
	if err := insecure.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.Address = "host:0" }, func(c *Config) { c.ID = "../bad" }, func(c *Config) { c.Token = "short" }, func(c *Config) { c.Fingerprint = "" }} {
		c := good
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("accepted invalid config")
		}
	}
}
