// Package agent talks to the noBGP agent through its own `nobgp` CLI.
//
// The Omarchy app never reads the agent's config directory or API socket
// directly. Everything goes through `nobgp ... --json`, so the agent stays
// the one owner of its state and this app keeps working across agent
// upgrades that move files around.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// candidates are where the agent's installers put the binary: the AUR/pacman
// package links /usr/local/bin/nobgp to /usr/share/nobgp/nobgp, and
// install.sh writes /usr/local/bin/nobgp.
var candidates = []string{"/usr/local/bin/nobgp", "/usr/bin/nobgp", "/usr/share/nobgp/nobgp"}

// lookPath and runCmd are package variables so tests can replace them.
var lookPath = exec.LookPath

var geteuid = os.Geteuid

var runCmd = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// ErrNotInstalled means no `nobgp` binary was found.
var ErrNotInstalled = errors.New("the noBGP agent is not installed")

// ErrNeedsAuth means the privileged status read needs a sudo password that
// nobody can type in this context (the bar, or a TUI before `sudo -v`).
var ErrNeedsAuth = errors.New("reading agent status needs sudo")

// ErrNotLoggedIn means the user-level commands (network list, proxy) need
// `nobgp login` first.
var ErrNotLoggedIn = errors.New("not logged in to noBGP")

// Binary returns the path of the `nobgp` CLI. NOBGP_BIN overrides it.
func Binary() (string, error) {
	if b := os.Getenv("NOBGP_BIN"); b != "" {
		return b, nil
	}
	if p, err := lookPath("nobgp"); err == nil {
		return p, nil
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", ErrNotInstalled
}

// Status is the subset of `nobgp status --json` this app shows.
type Status struct {
	Version     string              `json:"version"`
	Platform    string              `json:"platform"`
	Hostname    string              `json:"hostname"`
	Service     string              `json:"service"`
	User        *User               `json:"user"`
	Profile     *Profile            `json:"profile"`
	Profiles    map[string]*Profile `json:"profiles"`
	Unavailable map[string]string   `json:"unavailable"`
}

type User struct {
	Email  string `json:"email"`
	Status string `json:"status"`
}

type Profile struct {
	PID          int   `json:"pid"`
	Uptime       int64 `json:"uptime_secs"`
	Registration struct {
		NodeID string `json:"node_id"`
	} `json:"registration"`
	Router struct {
		URL        string `json:"url"`
		Connected  bool   `json:"connected"`
		Since      string `json:"since"`
		OfflineFor *int64 `json:"offline_for_secs"`
		Transport  string `json:"transport"`
	} `json:"router"`
	Network struct {
		LocalIP      string   `json:"local_ip"`
		TunDevice    string   `json:"tun_device"`
		Domain       string   `json:"domain"`
		PeerLoopback *bool    `json:"peer_loopback"`
		Targets      []Target `json:"targets"`
	} `json:"network"`
	MCP struct {
		Granted  bool   `json:"granted"`
		LocalMCP string `json:"local_mcp"`
		Running  bool   `json:"running"`
		Port     int    `json:"port"`
	} `json:"mcp"`
	Allow struct {
		Tools       []string `json:"tools"`
		Roots       []string `json:"roots"`
		Root        bool     `json:"root"`
		WithoutRoot string   `json:"without_root"`
	} `json:"allow"`
	FS *struct {
		Type    string `json:"type"`
		Mount   string `json:"mount"`
		Mounted bool   `json:"mounted"`
	} `json:"fs"`
	HostServices *struct {
		Reporting bool `json:"reporting"`
		Services  []struct {
			Type string `json:"type"`
			Port int    `json:"port"`
			Name string `json:"name"`
		} `json:"services"`
	} `json:"host_services"`
}

type Target struct {
	Name      string `json:"name"`
	NodeID    string `json:"node_id"`
	Address   string `json:"address"`
	Local     bool   `json:"local"`
	Reachable *bool  `json:"reachable"`
	RTTMillis *int64 `json:"rtt_ms"`
}

// Main returns the profile to show: the only one, or "default" when the
// node runs several.
func (s *Status) Main() *Profile {
	if s == nil {
		return nil
	}
	if s.Profile != nil {
		return s.Profile
	}
	if p, ok := s.Profiles["default"]; ok {
		return p
	}
	for _, p := range s.Profiles {
		return p
	}
	return nil
}

// NodeName is this node's name on the network: the name of the local
// target, which is what peers and the console call it. It falls back to the
// machine's hostname, which can differ (a VM named omarchy-test registered as
// omarchy-vm, say).
func (s *Status) NodeName() string {
	if p := s.Main(); p != nil {
		for _, t := range p.Network.Targets {
			if t.Local && t.Name != "" {
				return t.Name
			}
		}
	}
	if s == nil {
		return ""
	}
	return s.Hostname
}

// Peers returns the remote targets (everything but this node).
func (p *Profile) Peers() []Target {
	if p == nil {
		return nil
	}
	out := make([]Target, 0, len(p.Network.Targets))
	for _, t := range p.Network.Targets {
		if !t.Local {
			out = append(out, t)
		}
	}
	return out
}

// ParseStatus decodes `nobgp status --json`. The agent prints a line such as
// "Root privileges required, re-executing with sudo..." before the JSON when
// it elevates itself, so anything before the first '{' is skipped.
func ParseStatus(out []byte) (*Status, error) {
	i := bytes.IndexByte(out, '{')
	if i < 0 {
		return nil, fmt.Errorf("no JSON in status output: %q", truncate(string(out), 200))
	}
	var s Status
	if err := json.Unmarshal(out[i:], &s); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &s, nil
}

// Client runs the CLI.
type Client struct {
	Bin     string
	Timeout time.Duration
}

// New finds the binary and returns a client.
func New() (*Client, error) {
	bin, err := Binary()
	if err != nil {
		return nil, err
	}
	return &Client{Bin: bin, Timeout: 15 * time.Second}, nil
}

func (c *Client) run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	return runCmd(ctx, name, args...)
}

// Status reads the agent status. It never prompts: as root it runs the CLI
// directly, otherwise it uses `sudo -n`, which succeeds with a cached sudo
// credential or the NOPASSWD rule the package installs for exactly this
// command, and fails fast otherwise (ErrNeedsAuth).
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var out, errOut []byte
	var err error
	if geteuid() == 0 {
		out, errOut, err = c.run(ctx, c.Bin, "status", "--json")
	} else {
		out, errOut, err = c.run(ctx, "sudo", "-n", c.Bin, "status", "--json")
	}
	if err != nil {
		msg := strings.ToLower(string(errOut))
		if strings.Contains(msg, "password") || strings.Contains(msg, "sudo:") {
			return nil, ErrNeedsAuth
		}
		return nil, cliError("status", err, errOut)
	}
	return ParseStatus(out)
}

// ServiceActive reports whether the agent's systemd unit runs. It needs no
// privileges, so the bar can fall back to it when Status needs sudo.
func (c *Client) ServiceActive(ctx context.Context) bool {
	_, _, err := c.run(ctx, "systemctl", "is-active", "--quiet", "nobgp")
	return err == nil
}

// Directory is `nobgp network list --json`: the networks the signed-in user
// sees, their nodes and published services.
type Directory struct {
	Networks []Network `json:"networks"`
}

type Network struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

type Node struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
	Info   *struct {
		Platform string `json:"platform"`
	} `json:"info"`
	Services []Service `json:"services"`
}

type Service struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	PublicURL string `json:"public_url"`
	// Filled in by Services(), not by the router.
	NodeName    string `json:"-"`
	NodeID      string `json:"-"`
	NetworkName string `json:"-"`
}

// ParseDirectory decodes the network directory. The CLI's JSON may come
// wrapped in an envelope ({op, done, network_directory: {...}}) or bare, at
// the top level, depending on the version; both are accepted.
func ParseDirectory(out []byte) (*Directory, error) {
	i := bytes.IndexByte(out, '{')
	if i < 0 {
		return nil, fmt.Errorf("no JSON in network list output: %q", truncate(string(out), 200))
	}
	out = out[i:]
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, fmt.Errorf("decode network list: %w", err)
	}
	payload := out
	if p := bytes.TrimSpace(envelope["network_directory"]); len(p) > 0 && p[0] == '{' {
		payload = p
	}
	var d Directory
	if err := json.Unmarshal(payload, &d); err != nil {
		return nil, fmt.Errorf("decode network list: %w", err)
	}
	return &d, nil
}

// Services flattens every published service with its node and network.
func (d *Directory) Services() []Service {
	if d == nil {
		return nil
	}
	var out []Service
	for _, n := range d.Networks {
		for _, node := range n.Nodes {
			for _, s := range node.Services {
				s.NodeName, s.NodeID, s.NetworkName = node.Name, node.ID, n.Name
				out = append(out, s)
			}
		}
	}
	return out
}

// Directory lists networks, nodes and services. It runs as the user: the
// sign-in is per user, not root's.
func (c *Client) Directory(ctx context.Context) (*Directory, error) {
	out, errOut, err := c.run(ctx, c.Bin, "network", "list", "--json")
	if err != nil {
		return nil, cliError("network list", err, errOut, out)
	}
	return ParseDirectory(out)
}

func cliError(what string, err error, outputs ...[]byte) error {
	var msg string
	for _, o := range outputs {
		if s := strings.TrimSpace(string(o)); s != "" {
			msg = s
			break
		}
	}
	if strings.Contains(strings.ToLower(msg), "not logged in") {
		return ErrNotLoggedIn
	}
	msg = strings.TrimPrefix(msg, "Error: ")
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("nobgp %s: %s", what, truncate(msg, 300))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
