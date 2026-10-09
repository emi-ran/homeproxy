package windowsagent

import "strings"

// configTransition separates validated effective settings from persistence and
// lifecycle effects. Started means the controller owns an active run, including
// authentication and reconnect backoff, not just an authenticated connection.
type configTransition struct {
	Config Config
	Save   bool
	Stop   bool
	Start  bool
}

func normalizeConfig(c Config) Config {
	c.Fingerprint = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(c.Fingerprint), ":", ""))
	return c
}

func planConfigTransition(current Config, configured bool, desired Config, started bool) (configTransition, error) {
	if desired.Token == "" && configured {
		desired.Token = current.Token
	}
	desired = normalizeConfig(desired)
	if err := desired.Validate(); err != nil {
		return configTransition{}, err
	}
	current = normalizeConfig(current)
	connectionCurrent, connectionDesired := current, desired
	connectionCurrent.Enabled, connectionDesired.Enabled = false, false
	changed := !configured || connectionCurrent != connectionDesired
	return configTransition{
		Config: desired,
		Save:   !configured || current != desired,
		Stop:   started && (!desired.Enabled || changed),
		Start:  desired.Enabled && (!started || changed),
	}, nil
}
