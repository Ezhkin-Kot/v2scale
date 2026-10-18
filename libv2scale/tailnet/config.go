package tailnet

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"tailscale.com/ipn"
	"tailscale.com/types/logger"
)

// Config describes the embedded tailnet node. It is passed from the app as JSON.
type Config struct {
	// ControlURL is the Headscale or Tailscale control server (http or https).
	ControlURL string `json:"control_url"`
	// AuthKey is a pre-auth key, only needed for the first registration.
	AuthKey string `json:"auth_key,omitempty"`
	// Hostname is the node name in the tailnet.
	Hostname string `json:"hostname,omitempty"`
	// StateDir is a persistent private directory (node key, logs). Required.
	StateDir string `json:"state_dir"`
	// ControlProxy is an optional HTTP proxy used only for Tailscale's own control and
	// log connections (for example an i2pd tunnel); it does not affect Xray.
	ControlProxy string `json:"control_proxy,omitempty"`
	// Hosts maps names to tailnet IPs and takes precedence over any other resolution.
	Hosts map[string]string `json:"hosts,omitempty"`

	// Test hooks, never serialized.
	Store     ipn.StateStore `json:"-"`
	Ephemeral bool           `json:"-"`
	Logf      logger.Logf    `json:"-"`
}

// ParseConfig decodes and validates a JSON config. Unknown fields are errors so that
// typos do not silently disable a setting.
func ParseConfig(data string) (*Config, error) {
	var c Config
	dec := json.NewDecoder(strings.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("tailnet config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if u, err := url.Parse(c.ControlURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("tailnet config: control_url must be an http(s) URL")
	}
	if c.StateDir == "" {
		return fmt.Errorf("tailnet config: state_dir is required")
	}
	if c.ControlProxy != "" {
		if u, err := url.Parse(c.ControlProxy); err != nil || u.Host == "" {
			return fmt.Errorf("tailnet config: control_proxy must be a URL such as http://127.0.0.1:4444")
		}
	}
	for h, ip := range c.Hosts {
		if h == "" || strings.ContainsAny(h, " /:") {
			return fmt.Errorf("tailnet config: hosts: bad name %q", h)
		}
		if _, err := netip.ParseAddr(ip); err != nil {
			return fmt.Errorf("tailnet config: hosts: %q is not an IP address", ip)
		}
	}
	return nil
}
