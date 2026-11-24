package bar

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
)

func status(t *testing.T) *agent.Status {
	t.Helper()
	b, err := os.ReadFile("../agent/testdata/status.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := agent.ParseStatus(b)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

var now = time.Date(2026, 11, 9, 4, 33, 42, 0, time.UTC)

func TestConnected(t *testing.T) {
	s := FromStatus(status(t), now)
	if s.Class != "connected" || s.Text != GlyphConnected || s.PeerCount != 3 {
		t.Fatalf("%+v", s)
	}
	for _, want := range []string{"workstation", "over QUIC", "for 4d", "3 peers", "Drive at /mnt/nobgp"} {
		if !strings.Contains(s.Tooltip, want) {
			t.Errorf("tooltip %q lacks %q", s.Tooltip, want)
		}
	}
}

func TestOffline(t *testing.T) {
	st := status(t)
	p := st.Main()
	p.Router.Connected = false
	secs := int64(125)
	p.Router.OfflineFor = &secs
	s := FromStatus(st, now)
	if s.Class != "offline" || !strings.Contains(s.Tooltip, "Offline for 2m") {
		t.Fatalf("%+v", s)
	}
}

func TestUnregisteredAndStopped(t *testing.T) {
	st := status(t)
	st.Main().Registration.NodeID = ""
	if s := FromStatus(st, now); s.Class != "unregistered" {
		t.Errorf("class = %s", s.Class)
	}
	st = status(t)
	st.Service = "stopped"
	if s := FromStatus(st, now); s.Class != "stopped" {
		t.Errorf("class = %s", s.Class)
	}
	if s := FromStatus(&agent.Status{Service: "running", Unavailable: map[string]string{"default": "agent not running"}}, now); s.Class != "stopped" || !strings.Contains(s.Tooltip, "agent not running") {
		t.Errorf("%+v", s)
	}
}

type fakeSource struct {
	err    error
	active bool
}

func (f fakeSource) Status(context.Context) (*agent.Status, error) { return nil, f.err }
func (f fakeSource) ServiceActive(context.Context) bool            { return f.active }

// Without sudo the bar still tells a running agent from a stopped one.
func TestNeedsAuthFallsBackToServiceState(t *testing.T) {
	if s := Summarize(context.Background(), fakeSource{agent.ErrNeedsAuth, true}, now); s.Class != "auth" {
		t.Errorf("running: %+v", s)
	}
	if s := Summarize(context.Background(), fakeSource{agent.ErrNeedsAuth, false}, now); s.Class != "stopped" {
		t.Errorf("stopped: %+v", s)
	}
}
