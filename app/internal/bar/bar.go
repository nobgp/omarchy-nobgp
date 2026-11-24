// Package bar turns the agent's status into the small JSON the Omarchy bar
// widget renders. The same shape works as a Waybar-style custom module
// ({"text","tooltip","class"}), so it can also be used from a plain
// `type: "command"` entry in shell.json.
package bar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
)

// Glyphs are Nerd Font icons (Omarchy ships a Nerd Font in the bar).
const (
	GlyphConnected = "󰛳" // nf-md-network
	GlyphOffline   = "󰲜" // nf-md-network_off
	GlyphStopped   = "󰲛" // nf-md-network_off_outline
	GlyphUnknown   = "󰛵" // nf-md-network_outline
)

// State is what the widget shows.
type State struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	// Class is one of: connected, offline, stopped, auth, unregistered, missing.
	Class     string `json:"class"`
	Node      string `json:"node,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	PeerCount int    `json:"peers"`
}

// Source is what Summarize needs from the agent client.
type Source interface {
	Status(ctx context.Context) (*agent.Status, error)
	ServiceActive(ctx context.Context) bool
}

// Summarize reads the agent and builds the bar state. It never prompts.
func Summarize(ctx context.Context, src Source, now time.Time) State {
	st, err := src.Status(ctx)
	switch {
	case errors.Is(err, agent.ErrNeedsAuth):
		// No status without sudo; the unprivileged unit state still tells
		// running from stopped.
		if src.ServiceActive(ctx) {
			return State{Text: GlyphUnknown, Class: "auth",
				Tooltip: "noBGP agent running\nStatus needs sudo: run `nobgp-omarchy setup` to allow the bar to read it"}
		}
		return State{Text: GlyphStopped, Class: "stopped", Tooltip: "noBGP agent is not running"}
	case err != nil:
		return State{Text: GlyphStopped, Class: "stopped", Tooltip: "noBGP: " + err.Error()}
	}
	return FromStatus(st, now)
}

// FromStatus is the pure part of Summarize.
func FromStatus(st *agent.Status, now time.Time) State {
	if st.Service != "" && st.Service != "running" {
		return State{Text: GlyphStopped, Class: "stopped",
			Tooltip: fmt.Sprintf("noBGP agent %s", st.Service)}
	}
	p := st.Main()
	if p == nil {
		reason := "no profile is running"
		for _, r := range st.Unavailable {
			reason = r
			break
		}
		return State{Text: GlyphStopped, Class: "stopped", Tooltip: "noBGP: " + reason}
	}
	if p.Registration.NodeID == "" {
		return State{Text: GlyphUnknown, Class: "unregistered",
			Tooltip: "noBGP: this machine is not registered\nOpen noBGP to join a network"}
	}

	peers := p.Peers()
	var b strings.Builder
	name := st.NodeName()
	fmt.Fprintf(&b, "noBGP · %s\n", name)
	s := State{Node: name, NodeID: p.Registration.NodeID, PeerCount: len(peers)}
	if p.Router.Connected {
		s.Text, s.Class = GlyphConnected, "connected"
		fmt.Fprintf(&b, "Connected to %s", p.Router.URL)
		if p.Router.Transport != "" {
			fmt.Fprintf(&b, " over %s", strings.ToUpper(p.Router.Transport))
		}
		if d := since(p.Router.Since, now); d != "" {
			fmt.Fprintf(&b, " for %s", d)
		}
	} else {
		s.Text, s.Class = GlyphOffline, "offline"
		fmt.Fprintf(&b, "Offline")
		if p.Router.OfflineFor != nil {
			fmt.Fprintf(&b, " for %s", human(time.Duration(*p.Router.OfflineFor)*time.Second))
		}
		fmt.Fprintf(&b, ", reconnecting to %s", p.Router.URL)
	}
	fmt.Fprintf(&b, "\n%d peer%s", len(peers), plural(len(peers)))
	if p.Network.LocalIP != "" {
		fmt.Fprintf(&b, " · %s", p.Network.LocalIP)
	}
	if p.FS != nil && p.FS.Mounted {
		fmt.Fprintf(&b, "\nDrive at %s", p.FS.Mount)
	}
	s.Tooltip = b.String()
	return s
}

func since(rfc3339 string, now time.Time) string {
	if rfc3339 == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil || t.After(now) {
		return ""
	}
	return human(now.Sub(t))
}

func human(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
