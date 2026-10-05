package windowsagent

import (
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
)

const ServiceName = "HomeProxyAgent"
const PipePath = `\\.\pipe\HomeProxyAgent.v1`

type Config struct {
	Address     string `json:"address"`
	ID          string `json:"id"`
	Token       string `json:"token,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Enabled     bool   `json:"enabled"`
	Insecure    bool   `json:"insecure"`
}

func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.Address)
	n, pe := strconv.Atoi(port)
	if err != nil || host == "" || pe != nil || n < 1 || n > 65535 {
		return errors.New("address must be host:port")
	}
	if len(c.ID) < 1 || len(c.ID) > 64 || strings.ContainsAny(c.ID, "\x00\r\n\\/") {
		return errors.New("id must be 1-64 bytes without control characters or slashes")
	}
	for _, ch := range c.ID {
		if ch < 32 || ch == 127 {
			return errors.New("id must not contain control characters")
		}
	}
	if len(c.Token) < 16 || len(c.Token) > 4096 {
		return errors.New("token must be 16-4096 bytes")
	}
	pin, err := hex.DecodeString(c.Fingerprint)
	if !c.Insecure && (err != nil || len(pin) != 32) {
		return errors.New("fingerprint must be 64 hex characters")
	}
	return nil
}

type Request struct {
	Version int     `json:"version"`
	Op      string  `json:"op"`
	Config  *Config `json:"config,omitempty"`
}
type Response struct {
	Version int     `json:"version"`
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	Config  *Config `json:"config,omitempty"`
	Status  string  `json:"status,omitempty"`
}
