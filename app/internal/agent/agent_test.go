package agent

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The fixture is real `nobgp status --json` output from a Linux node
// (agent 0.4.188), with the sudo re-exec line the agent prints first.
func TestParseStatusRealOutput(t *testing.T) {
	st, err := ParseStatus(readFixture(t, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := st.Main()
	if p == nil {
		t.Fatal("no profile")
	}
	if st.Hostname != "workstation" || st.Service != "running" {
		t.Errorf("host/service = %q/%q", st.Hostname, st.Service)
	}
	if p.Registration.NodeID != "11111111-1111-4111-8111-111111111111" || !p.Router.Connected || p.Router.Transport != "quic" {
		t.Errorf("unexpected profile: %+v", p.Router)
	}
	if got := len(p.Peers()); got != 3 {
		t.Errorf("peers = %d, want 3 (local target excluded)", got)
	}
	if p.FS == nil || p.FS.Mount != "/mnt/nobgp" || !p.FS.Mounted {
		t.Errorf("fs = %+v", p.FS)
	}
}

func TestParseStatusNoJSON(t *testing.T) {
	if _, err := ParseStatus([]byte("sudo: a password is required\n")); err == nil {
		t.Fatal("want error")
	}
}

func TestMainPicksDefaultProfile(t *testing.T) {
	a, d := &Profile{PID: 1}, &Profile{PID: 2}
	st := &Status{Profiles: map[string]*Profile{"lab": a, "default": d}}
	if st.Main() != d {
		t.Fatal("want the default profile")
	}
	var nilStatus *Status
	if nilStatus.Main() != nil || (*Profile)(nil).Peers() != nil {
		t.Fatal("nil receivers must be safe")
	}
}

func TestParseDirectoryBothShapes(t *testing.T) {
	env, err := ParseDirectory(readFixture(t, "directory-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	ss := env.Services()
	if len(ss) != 2 || ss[0].NodeName != "workstation" || ss[1].NetworkName != "Home" {
		t.Fatalf("services = %+v", ss)
	}
	// A router from before the result envelope puts the directory at the top.
	flat, err := ParseDirectory([]byte(`{"networks":[{"name":"n","nodes":[{"id":"x","name":"a","services":[{"id":"s","title":"t"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := flat.Services(); len(got) != 1 || got[0].NodeID != "x" {
		t.Fatalf("flat services = %+v", got)
	}
}

func withRunner(t *testing.T, fn func(name string, args ...string) ([]byte, []byte, error)) {
	t.Helper()
	old := runCmd
	runCmd = func(_ context.Context, name string, args ...string) ([]byte, []byte, error) { return fn(name, args...) }
	t.Cleanup(func() { runCmd = old })
}

func TestStatusNeedsAuthWithoutPassword(t *testing.T) {
	oldEUID := geteuid
	geteuid = func() int { return 1000 }
	t.Cleanup(func() { geteuid = oldEUID })
	withRunner(t, func(name string, args ...string) ([]byte, []byte, error) {
		if name != "sudo" || args[0] != "-n" {
			t.Fatalf("status must use sudo -n, got %s %v", name, args)
		}
		return nil, []byte("sudo: a password is required\n"), errors.New("exit status 1")
	})
	c := &Client{Bin: "/usr/local/bin/nobgp"}
	if _, err := c.Status(context.Background()); !errors.Is(err, ErrNeedsAuth) {
		t.Fatalf("err = %v, want ErrNeedsAuth", err)
	}
}

func TestDirectoryNotLoggedIn(t *testing.T) {
	withRunner(t, func(name string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("Error: not logged in — run 'nobgp login' first\n"), errors.New("exit status 1")
	})
	c := &Client{Bin: "nobgp"}
	if _, err := c.Directory(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestSettingsCurrentFromStatus(t *testing.T) {
	st, _ := ParseStatus(readFixture(t, "status.json"))
	p := st.Main()
	want := map[string]string{
		"allow-root":           "true",
		"allow-tools":          "fs,command,logs",
		"allow-roots":          "/",
		"local-mcp":            "on",
		"peer-loopback":        "true",
		"mdns-report-services": "true",
		"mount":                "/mnt/nobgp",
		"user":                 "",
	}
	for _, s := range Settings {
		if got := s.Current(p); got != want[s.Key] {
			t.Errorf("%s = %q, want %q", s.Key, got, want[s.Key])
		}
	}
}

func setting(key string) Setting {
	for _, s := range Settings {
		if s.Key == key {
			return s
		}
	}
	panic(key)
}

func TestSettingValidate(t *testing.T) {
	cases := []struct {
		key, v string
		ok     bool
	}{
		{"allow-root", "false", true},
		{"allow-root", "no", false},
		{"local-mcp", "off", true},
		{"local-mcp", "maybe", false},
		{"allow-tools", "fs, command", true},
		{"allow-tools", "fs,shell", false},
		{"allow-roots", "/home/alex,/srv", true},
		{"allow-roots", "home", false},
		{"user", "alex", true},
		{"user", "two words", false},
	}
	for _, c := range cases {
		err := setting(c.key).Validate(c.v)
		if (err == nil) != c.ok {
			t.Errorf("Validate(%s=%q) err=%v, want ok=%v", c.key, c.v, err, c.ok)
		}
	}
}

func TestToggle(t *testing.T) {
	if v, ok := setting("allow-root").Toggle("true"); !ok || v != "false" {
		t.Errorf("toggle bool = %q %v", v, ok)
	}
	if v, ok := setting("local-mcp").Toggle("on"); !ok || v != "off" {
		t.Errorf("toggle enum = %q %v", v, ok)
	}
	if _, ok := setting("allow-roots").Toggle("/"); ok {
		t.Error("a list is edited, not toggled")
	}
}

func TestConfigArgsNeverAnswersForTheOwner(t *testing.T) {
	c := &Client{Bin: "/usr/local/bin/nobgp"}
	got := c.ConfigArgs(setting("allow-roots"), " /home/alex , /srv ")
	want := []string{"sudo", "/usr/local/bin/nobgp", "config", "--allow-roots=/home/alex,/srv"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, a := range got {
		if a == "--yes" || a == "-y" {
			t.Fatal("ConfigArgs must not pass --yes: the agent's confirmation is the owner's to answer")
		}
	}
}

func TestPublishArgs(t *testing.T) {
	c := &Client{Bin: "nobgp"}
	if _, err := c.PublishArgs(Publish{Title: "x", ProxyURL: "localhost:3000"}); err == nil {
		t.Error("want error without a node id")
	}
	if _, err := c.PublishArgs(Publish{NodeID: "n", ProxyURL: "a", Command: "b"}); err == nil {
		t.Error("want error with both URL and command")
	}
	got, err := c.PublishArgs(Publish{NodeID: "n", Title: "Dev", ProxyURL: "localhost:3000", Share: []string{"a@x", "b@x"}})
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Join(got, " ")
	if !strings.Contains(s, "--proxy-url http://localhost:3000") || !strings.Contains(s, "--share a@x,b@x") || !strings.Contains(s, "--node-id n") {
		t.Fatalf("args = %s", s)
	}
}

// The node's noBGP name comes from its local target, not the hostname: a VM
// whose hostname is omarchy-test can be registered as omarchy-vm.
func TestNodeNamePrefersTheLocalTarget(t *testing.T) {
	st := &Status{Hostname: "omarchy-test", Profile: &Profile{}}
	st.Profile.Network.Targets = []Target{{Name: "workstation"}, {Name: "omarchy-vm", Local: true}}
	if got := st.NodeName(); got != "omarchy-vm" {
		t.Fatalf("NodeName = %q", got)
	}
	st.Profile.Network.Targets = nil
	if got := st.NodeName(); got != "omarchy-test" {
		t.Fatalf("fallback = %q", got)
	}
	if (*Status)(nil).NodeName() != "" {
		t.Fatal("nil status")
	}
}
