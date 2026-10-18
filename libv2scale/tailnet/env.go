package tailnet

import (
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"

	"tailscale.com/net/tshttpproxy"
)

// controlProxy holds the proxy for Tailscale's own HTTP clients. It is process wide but
// only consulted by tailscale.com code, never by Xray.
var controlProxy atomic.Pointer[url.URL]

func init() {
	// Must run before any Tailscale HTTP client is created: SetProxyFunc is refused once
	// the environment-based configuration has been read.
	tshttpproxy.SetProxyFunc(func(*url.URL) (*url.URL, error) {
		return controlProxy.Load(), nil
	})
}

// prepareEnv makes Tailscale usable inside an app process: on Android there is no HOME or
// TMPDIR and the working directory is "/", which makes its log policy panic, and logs must
// not be uploaded to Tailscale's servers.
func prepareEnv(stateDir string) error {
	tmp := filepath.Join(stateDir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	os.Setenv("TS_NO_LOGS_NO_SUPPORT", "true")
	if os.Getenv("HOME") == "" {
		os.Setenv("HOME", stateDir)
	}
	if os.Getenv("XDG_CACHE_HOME") == "" {
		os.Setenv("XDG_CACHE_HOME", stateDir)
	}
	if os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", tmp)
	}
	return nil
}
