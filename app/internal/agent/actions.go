package agent

import (
	"context"
	"fmt"
	"strings"
)

// Interactive actions return an argv for the caller to run in a real
// terminal, because they can prompt: sudo for a password, `nobgp login` for
// a browser approval, `nobgp config` for a confirmation when a setting could
// cut off remote access. The TUI hands them to tea.ExecProcess; the menu runs
// them in a floating terminal.

// LoginArgs signs this user in to noBGP (needed for services and networks).
func (c *Client) LoginArgs() []string { return []string{c.Bin, "login"} }

// RegisterArgs joins this machine to a network. Registration writes the
// root-owned config, so the agent elevates; running it through sudo here
// makes the password prompt explicit.
func (c *Client) RegisterArgs(network, name string) []string {
	args := []string{"sudo", c.Bin, "register"}
	if network != "" {
		args = append(args, "--network", network)
	}
	if name != "" {
		args = append(args, "--name", name)
	}
	return args
}

// Setting is one host setting the app lets the owner change, mapped to a
// `nobgp config` flag.
type Setting struct {
	Key   string // the `nobgp config` flag, without dashes
	Label string
	Help  string
	Kind  string // "bool", "enum", "list", "string"
	Enum  []string
}

// Settings are the host-configuration controls of the This device tab. Only
// flags the agent documents are listed; anything else stays a `nobgp config`
// away.
var Settings = []Setting{
	{Key: "allow-root", Label: "Allow root", Kind: "bool",
		Help: "Let callers run commands and file operations as root on this machine."},
	{Key: "user", Label: "Run-as account", Kind: "string",
		Help: "The account commands and file operations without root run as."},
	{Key: "allow-tools", Label: "Allowed tools", Kind: "list",
		Help: "Capability domains served to the network: fs, command, logs."},
	{Key: "allow-roots", Label: "Shared folders", Kind: "list",
		Help: "Filesystem roots remote file tools may touch (default: /)."},
	{Key: "local-mcp", Label: "Local MCP server", Kind: "enum", Enum: []string{"on", "off"},
		Help: "off keeps the MCP server off whatever the router grants."},
	{Key: "peer-loopback", Label: "Peers reach loopback", Kind: "bool",
		Help: "Whether peers may reach services bound only to 127.0.0.1."},
	{Key: "mdns-report-services", Label: "Report host services", Kind: "bool",
		Help: "Tell the network about SSH, SMB and similar services on this host."},
	{Key: "mount", Label: "Network drive mount", Kind: "string",
		Help: "Where the network's shared drive is mounted (default /mnt/nobgp)."},
}

// Current returns the value the running agent reports for a setting, or ""
// when the status does not carry it.
func (s Setting) Current(p *Profile) string {
	if p == nil {
		return ""
	}
	switch s.Key {
	case "allow-root":
		return fmt.Sprint(p.Allow.Root)
	case "user":
		return p.Allow.WithoutRoot
	case "allow-tools":
		return strings.Join(p.Allow.Tools, ",")
	case "allow-roots":
		return strings.Join(p.Allow.Roots, ",")
	case "local-mcp":
		return p.MCP.LocalMCP
	case "peer-loopback":
		if p.Network.PeerLoopback != nil {
			return fmt.Sprint(*p.Network.PeerLoopback)
		}
	case "mdns-report-services":
		if p.HostServices != nil {
			return fmt.Sprint(p.HostServices.Reporting)
		}
	case "mount":
		if p.FS != nil {
			return p.FS.Mount
		}
	}
	return ""
}

// Validate checks a value before it reaches sudo.
func (s Setting) Validate(v string) error {
	v = strings.TrimSpace(v)
	switch s.Kind {
	case "bool":
		if v != "true" && v != "false" {
			return fmt.Errorf("%s takes true or false", s.Key)
		}
	case "enum":
		for _, e := range s.Enum {
			if v == e {
				return nil
			}
		}
		return fmt.Errorf("%s takes one of %s", s.Key, strings.Join(s.Enum, ", "))
	case "list":
		if v == "" {
			return fmt.Errorf("%s needs at least one value", s.Key)
		}
		if s.Key == "allow-tools" {
			for _, t := range strings.Split(v, ",") {
				switch strings.TrimSpace(t) {
				case "fs", "command", "logs":
				default:
					return fmt.Errorf("unknown tool %q (fs, command, logs)", t)
				}
			}
		}
		if s.Key == "allow-roots" {
			for _, r := range strings.Split(v, ",") {
				if !strings.HasPrefix(strings.TrimSpace(r), "/") {
					return fmt.Errorf("%q is not an absolute path", r)
				}
			}
		}
	case "string":
		if v == "" {
			return fmt.Errorf("%s needs a value", s.Key)
		}
		if strings.ContainsAny(v, " \t\n") {
			return fmt.Errorf("%s cannot contain spaces", s.Key)
		}
	}
	return nil
}

// Toggle returns the opposite of a bool or two-value enum setting.
func (s Setting) Toggle(current string) (string, bool) {
	switch s.Kind {
	case "bool":
		if current == "true" {
			return "false", true
		}
		return "true", true
	case "enum":
		if len(s.Enum) == 2 {
			if current == s.Enum[0] {
				return s.Enum[1], true
			}
			return s.Enum[0], true
		}
	}
	return "", false
}

// ConfigArgs changes one setting. It runs in a terminal so the agent's own
// confirmation (for settings that can cut off remote access) is shown to the
// owner rather than answered for them, which is why --yes is never passed.
func (c *Client) ConfigArgs(s Setting, value string) []string {
	value = strings.TrimSpace(value)
	if s.Kind == "list" {
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		value = strings.Join(parts, ",")
	}
	return []string{"sudo", c.Bin, "config", "--" + s.Key + "=" + value}
}

// RestartArgs restarts the agent service.
func (c *Client) RestartArgs() []string { return []string{"sudo", c.Bin, "service", "restart"} }

// AuthArgs caches a sudo credential so the non-interactive status read works.
func AuthArgs() []string { return []string{"sudo", "-v"} }

// Publish describes a service to publish on this node.
type Publish struct {
	NodeID   string
	Title    string
	ProxyURL string // http://localhost:8080
	Command  string // or a terminal command
	Share    []string
}

// PublishArgs builds `nobgp proxy publish`.
func (c *Client) PublishArgs(p Publish) ([]string, error) {
	if p.NodeID == "" {
		return nil, fmt.Errorf("this node has no node id yet: register it first")
	}
	if (p.ProxyURL == "") == (p.Command == "") {
		return nil, fmt.Errorf("give either a URL to proxy or a terminal command")
	}
	args := []string{c.Bin, "proxy", "publish", "--node-id", p.NodeID}
	if p.Title != "" {
		args = append(args, "--title", p.Title)
	}
	if p.ProxyURL != "" {
		u := p.ProxyURL
		if !strings.Contains(u, "://") {
			u = "http://" + u
		}
		args = append(args, "--proxy-url", u)
	} else {
		args = append(args, "--command", p.Command)
	}
	if len(p.Share) > 0 {
		args = append(args, "--share", strings.Join(p.Share, ","))
	}
	return args, nil
}

// DeleteService removes a published service. It does not prompt.
func (c *Client) DeleteService(ctx context.Context, id string) error {
	out, errOut, err := c.run(ctx, c.Bin, "proxy", "delete", "--id", id)
	if err != nil {
		return cliError("proxy delete", err, errOut, out)
	}
	return nil
}

// ShareService gives more people access to a published service.
func (c *Client) ShareService(ctx context.Context, id string, emails []string) error {
	args := append([]string{"proxy", "share", "add", "--id", id}, emails...)
	out, errOut, err := c.run(ctx, c.Bin, args...)
	if err != nil {
		return cliError("proxy share add", err, errOut, out)
	}
	return nil
}
