package tailnet

import (
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	good := `{"control_url":"http://127.0.0.1:7080","state_dir":"/data/ts","hostname":"phone",
	  "control_proxy":"http://127.0.0.1:4444","hosts":{"svc.example":"100.64.0.2"}}`
	c, err := ParseConfig(good)
	if err != nil {
		t.Fatal(err)
	}
	if c.Hostname != "phone" || c.Hosts["svc.example"] != "100.64.0.2" {
		t.Errorf("parsed config: %+v", c)
	}

	bad := map[string]string{
		`{"control_url":"","state_dir":"/d"}`:                                   "control_url",
		`{"control_url":"ftp://h","state_dir":"/d"}`:                            "control_url",
		`{"control_url":"http://h"}`:                                            "state_dir",
		`{"control_url":"http://h","state_dir":"/d","control_proxy":"nope"}`:    "control_proxy",
		`{"control_url":"http://h","state_dir":"/d","hosts":{"a b":"1.2.3.4"}}`: "bad name",
		`{"control_url":"http://h","state_dir":"/d","hosts":{"a":"nope"}}`:      "not an IP",
		`{"control_url":"http://h","state_dir":"/d","typo":1}`:                  "unknown field",
	}
	for in, want := range bad {
		if _, err := ParseConfig(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseConfig(%s) error = %v, want substring %q", in, err, want)
		}
	}
}
