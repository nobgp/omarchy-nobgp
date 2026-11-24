// Package tui is the noBGP terminal app for Omarchy: status, peers,
// published services and this machine's host settings, keyboard-driven in
// the lazygit/lazydocker style Omarchy users expect.
package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
)

// Backend is the part of agent.Client the TUI uses; tests pass a fake.
type Backend interface {
	Status(ctx context.Context) (*agent.Status, error)
	Directory(ctx context.Context) (*agent.Directory, error)
	DeleteService(ctx context.Context, id string) error
	ShareService(ctx context.Context, id string, emails []string) error
	LoginArgs() []string
	RegisterArgs(network, name string) []string
	ConfigArgs(s agent.Setting, value string) []string
	RestartArgs() []string
	PublishArgs(p agent.Publish) ([]string, error)
}

type tab int

const (
	tabOverview tab = iota
	tabPeers
	tabServices
	tabDevice
)

var tabNames = []string{"Overview", "Peers", "Services", "This device"}

// WebConsole is the full graphical console (network map, LLM, sharing).
const WebConsole = "https://app.nobgp.com"

type statusMsg struct {
	st  *agent.Status
	err error
}

type dirMsg struct {
	dir *agent.Directory
	err error
}

type flashMsg struct {
	text string
	err  bool
}

type tickMsg time.Time

// form is a small modal of text fields.
type form struct {
	title    string
	labels   []string
	inputs   []textinput.Model
	focus    int
	err      string
	onSubmit func(values []string) (tea.Cmd, error)
}

type confirm struct {
	prompt string
	onYes  tea.Cmd
}

// Model is the Bubble Tea model.
type Model struct {
	be  Backend
	tab tab

	st     *agent.Status
	stErr  error
	dir    *agent.Directory
	dirErr error
	loaded bool

	cursor [4]int

	form    *form
	confirm *confirm

	flash      string
	flashIsErr bool
	flashAt    time.Time

	width, height int
	now           func() time.Time
}

// New builds the model. startTab lets `nobgp-omarchy services` open there.
func New(be Backend, startTab string) Model {
	m := Model{be: be, now: time.Now}
	switch startTab {
	case "peers":
		m.tab = tabPeers
	case "services":
		m.tab = tabServices
	case "device", "settings":
		m.tab = tabDevice
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadStatus(), m.loadDir(), tick())
}

// The TUI refreshes while it is open and on screen only; nothing runs in
// the background once it is closed.
func tick() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) loadStatus() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		st, err := m.be.Status(ctx)
		return statusMsg{st, err}
	}
}

func (m Model) loadDir() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		d, err := m.be.Directory(ctx)
		return dirMsg{d, err}
	}
}

func flash(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return flashMsg{text, isErr} }
}

func (m Model) profile() *agent.Profile { return m.st.Main() }

func (m Model) peers() []agent.Target {
	ps := m.profile().Peers()
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	return ps
}

func (m Model) services() []agent.Service {
	ss := m.dir.Services()
	self := ""
	if p := m.profile(); p != nil {
		self = p.Registration.NodeID
	}
	// This machine's services first, then by node and title.
	sort.SliceStable(ss, func(i, j int) bool {
		a, b := ss[i].NodeID == self, ss[j].NodeID == self
		if a != b {
			return a
		}
		if ss[i].NodeName != ss[j].NodeName {
			return ss[i].NodeName < ss[j].NodeName
		}
		return ss[i].Title < ss[j].Title
	})
	return ss
}

func (m Model) listLen() int {
	switch m.tab {
	case tabPeers:
		return len(m.peers())
	case tabServices:
		return len(m.services())
	case tabDevice:
		return len(agent.Settings)
	}
	return 0
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case statusMsg:
		m.st, m.stErr, m.loaded = msg.st, msg.err, true
		m.clampCursor()
		return m, nil
	case dirMsg:
		m.dir, m.dirErr = msg.dir, msg.err
		m.clampCursor()
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadStatus(), m.loadDir(), tick())
	case flashMsg:
		m.flash, m.flashIsErr, m.flashAt = msg.text, msg.err, m.now()
		return m, nil
	case execDoneMsg:
		cmds := []tea.Cmd{m.loadStatus(), m.loadDir()}
		if msg.err != nil {
			cmds = append(cmds, flash(msg.what+" did not finish", true))
		} else {
			cmds = append(cmds, flash(msg.what+" done", false))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		if m.confirm != nil {
			return m.updateConfirm(msg)
		}
		if m.form != nil {
			return m.updateForm(msg)
		}
		return m.updateKeys(msg)
	}
	if m.form != nil {
		var cmd tea.Cmd
		m.form.inputs[m.form.focus], cmd = m.form.inputs[m.form.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) clampCursor() {
	n := m.listLen()
	if m.cursor[m.tab] >= n {
		m.cursor[m.tab] = max(0, n-1)
	}
}

func (m Model) updateConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	m.confirm = nil
	switch k.String() {
	case "y", "Y":
		return m, c.onYes
	}
	return m, flash("Cancelled", false)
}

func (m Model) updateForm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	switch k.String() {
	case "esc":
		m.form = nil
		return m, nil
	case "tab", "down":
		f.inputs[f.focus].Blur()
		f.focus = (f.focus + 1) % len(f.inputs)
		return m, f.inputs[f.focus].Focus()
	case "shift+tab", "up":
		f.inputs[f.focus].Blur()
		f.focus = (f.focus - 1 + len(f.inputs)) % len(f.inputs)
		return m, f.inputs[f.focus].Focus()
	case "enter":
		if f.focus < len(f.inputs)-1 {
			f.inputs[f.focus].Blur()
			f.focus++
			return m, f.inputs[f.focus].Focus()
		}
		vals := make([]string, len(f.inputs))
		for i, in := range f.inputs {
			vals[i] = strings.TrimSpace(in.Value())
		}
		cmd, err := f.onSubmit(vals)
		if err != nil {
			f.err = err.Error()
			return m, nil
		}
		m.form = nil
		return m, cmd
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(k)
	return m, cmd
}

func newForm(title string, labels, values, placeholders []string, onSubmit func([]string) (tea.Cmd, error)) *form {
	f := &form{title: title, labels: labels, onSubmit: onSubmit}
	for i := range labels {
		in := textinput.New()
		in.Prompt = ""
		in.CharLimit = 512
		in.Width = 48
		if i < len(values) {
			in.SetValue(values[i])
		}
		if i < len(placeholders) {
			in.Placeholder = placeholders[i]
		}
		f.inputs = append(f.inputs, in)
	}
	f.inputs[0].Focus()
	return f
}

func (m Model) updateKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "1", "2", "3", "4":
		m.tab = tab(key[0] - '1')
		return m, nil
	case "tab", "l", "right":
		m.tab = (m.tab + 1) % 4
		return m, nil
	case "shift+tab", "h", "left":
		m.tab = (m.tab + 3) % 4
		return m, nil
	case "j", "down":
		if m.cursor[m.tab] < m.listLen()-1 {
			m.cursor[m.tab]++
		}
		return m, nil
	case "k", "up":
		if m.cursor[m.tab] > 0 {
			m.cursor[m.tab]--
		}
		return m, nil
	case "g", "home":
		m.cursor[m.tab] = 0
		return m, nil
	case "G", "end":
		m.cursor[m.tab] = max(0, m.listLen()-1)
		return m, nil
	case "r":
		return m, tea.Batch(m.loadStatus(), m.loadDir(), flash("Refreshing…", false))
	case "w":
		if err := openURL(WebConsole); err != nil {
			return m, flash("Could not open the console: "+err.Error(), true)
		}
		return m, flash("Opening the noBGP console", false)
	case "L":
		return m, runInTerminal("Login", m.be.LoginArgs())
	case "A":
		// Cache a sudo credential so status can be read.
		return m, runInTerminal("Sudo", agent.AuthArgs())
	case "X":
		m.confirm = &confirm{prompt: "Restart the noBGP agent?", onYes: runInTerminal("Restart", m.be.RestartArgs())}
		return m, nil
	case "R":
		m.form = newForm("Join a network",
			[]string{"Network", "Node name"},
			[]string{"", m.st.NodeName()},
			[]string{"leave empty to choose in the browser", ""},
			func(v []string) (tea.Cmd, error) {
				return runInTerminal("Registration", m.be.RegisterArgs(v[0], v[1])), nil
			})
		return m, textinput.Blink
	}

	switch m.tab {
	case tabPeers:
		return m.peerKeys(key)
	case tabServices:
		return m.serviceKeys(key)
	case tabDevice:
		return m.deviceKeys(key)
	}
	return m, nil
}

func (m Model) peerKeys(key string) (tea.Model, tea.Cmd) {
	ps := m.peers()
	if len(ps) == 0 {
		return m, nil
	}
	p := ps[m.cursor[tabPeers]]
	switch key {
	case "enter", "c", "y":
		return m, m.copy(p.Address, "address of "+p.Name)
	case "n":
		return m, m.copy(p.Name, "name")
	case "i":
		return m, m.copy(p.NodeID, "node id of "+p.Name)
	case "s":
		return m, runInTerminal("SSH to "+p.Name, []string{"ssh", p.Address})
	}
	return m, nil
}

func (m Model) copy(s, what string) tea.Cmd {
	if err := copyToClipboard(s); err != nil {
		return flash("Could not copy: "+err.Error(), true)
	}
	return flash("Copied "+what, false)
}

func (m Model) serviceKeys(key string) (tea.Model, tea.Cmd) {
	if key == "a" {
		p := m.profile()
		if p == nil || p.Registration.NodeID == "" {
			return m, flash("Register this machine first (R)", true)
		}
		nodeID := p.Registration.NodeID
		m.form = newForm("Publish a service on this machine",
			[]string{"Title", "URL or command", "Share with (emails)"},
			nil,
			[]string{"My dev server", "localhost:3000   or   cmd: htop", "optional, comma separated"},
			func(v []string) (tea.Cmd, error) {
				pub := agent.Publish{NodeID: nodeID, Title: v[0], Share: splitList(v[2])}
				if cmdline, ok := strings.CutPrefix(v[1], "cmd:"); ok {
					pub.Command = strings.TrimSpace(cmdline)
				} else {
					pub.ProxyURL = v[1]
				}
				args, err := m.be.PublishArgs(pub)
				if err != nil {
					return nil, err
				}
				return runInTerminal("Publish", args), nil
			})
		return m, textinput.Blink
	}
	ss := m.services()
	if len(ss) == 0 {
		return m, nil
	}
	s := ss[m.cursor[tabServices]]
	switch key {
	case "enter", "o":
		if s.PublicURL == "" {
			return m, flash("This service has no public URL", true)
		}
		if err := openURL(s.PublicURL); err != nil {
			return m, flash("Could not open: "+err.Error(), true)
		}
		return m, flash("Opening "+s.Title, false)
	case "c", "y":
		return m, m.copy(s.PublicURL, "URL")
	case "d":
		id, title := s.ID, s.Title
		m.confirm = &confirm{prompt: fmt.Sprintf("Delete %q? Its URL stops working.", title), onYes: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := m.be.DeleteService(ctx, id); err != nil {
				return flashMsg{err.Error(), true}
			}
			return execDoneMsg{what: "Delete"}
		}}
		return m, nil
	case "S":
		id := s.ID
		m.form = newForm("Share "+s.Title, []string{"Emails"}, nil, []string{"alice@example.com, bob@example.com"},
			func(v []string) (tea.Cmd, error) {
				emails := splitList(v[0])
				if len(emails) == 0 {
					return nil, errors.New("enter at least one email")
				}
				return func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					if err := m.be.ShareService(ctx, id, emails); err != nil {
						return flashMsg{err.Error(), true}
					}
					return execDoneMsg{what: "Share"}
				}, nil
			})
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) deviceKeys(key string) (tea.Model, tea.Cmd) {
	if key != "enter" && key != "e" && key != " " {
		return m, nil
	}
	s := agent.Settings[m.cursor[tabDevice]]
	cur := s.Current(m.profile())
	if v, ok := s.Toggle(cur); ok && key != "e" {
		m.confirm = &confirm{
			prompt: fmt.Sprintf("Set %s to %s? (sudo)", s.Label, v),
			onYes:  runInTerminal(s.Label, m.be.ConfigArgs(s, v)),
		}
		return m, nil
	}
	m.form = newForm(s.Label, []string{s.Key}, []string{cur}, nil, func(v []string) (tea.Cmd, error) {
		if err := s.Validate(v[0]); err != nil {
			return nil, err
		}
		return runInTerminal(s.Label, m.be.ConfigArgs(s, v[0])), nil
	})
	return m, textinput.Blink
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- view ----

var (
	accent  = lipgloss.AdaptiveColor{Light: "4", Dark: "4"}
	dim     = lipgloss.NewStyle().Faint(true)
	bold    = lipgloss.NewStyle().Bold(true)
	green   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	selRow  = lipgloss.NewStyle().Reverse(true)
	tabOn   = lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true)
	tabOff  = lipgloss.NewStyle().Faint(true)
	boxed   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	heading = lipgloss.NewStyle().Bold(true).Foreground(accent)
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")

	switch {
	case m.confirm != nil:
		b.WriteString(boxed.Render(m.confirm.prompt + "\n\n" + dim.Render("y: yes · any other key: no")))
	case m.form != nil:
		b.WriteString(m.formView())
	default:
		switch m.tab {
		case tabOverview:
			b.WriteString(m.overview())
		case tabPeers:
			b.WriteString(m.peerView())
		case tabServices:
			b.WriteString(m.serviceView())
		case tabDevice:
			b.WriteString(m.deviceView())
		}
	}

	b.WriteString("\n\n")
	if m.flash != "" && m.now().Sub(m.flashAt) < 6*time.Second {
		if m.flashIsErr {
			b.WriteString(red.Render(m.flash))
		} else {
			b.WriteString(green.Render(m.flash))
		}
		b.WriteString("\n")
	}
	b.WriteString(dim.Render(m.footer()))
	return b.String()
}

func (m Model) header() string {
	title := heading.Render("󰛳 noBGP")
	state := dim.Render("loading…")
	if m.loaded {
		state = m.stateBadge()
	}
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == m.tab {
			tabs = append(tabs, tabOn.Render(label))
		} else {
			tabs = append(tabs, tabOff.Render(label))
		}
	}
	return title + "  " + state + "\n" + strings.Join(tabs, "   ")
}

func (m Model) stateBadge() string {
	if errors.Is(m.stErr, agent.ErrNeedsAuth) {
		return yellow.Render("● status needs sudo (A)")
	}
	if m.stErr != nil {
		return red.Render("● " + m.stErr.Error())
	}
	p := m.profile()
	switch {
	case m.st.Service != "" && m.st.Service != "running":
		return red.Render("● agent " + m.st.Service)
	case p == nil:
		return red.Render("● not running")
	case p.Registration.NodeID == "":
		return yellow.Render("● not registered (R)")
	case p.Router.Connected:
		return green.Render("● connected") + dim.Render(" · "+m.st.NodeName())
	default:
		return red.Render("● offline") + dim.Render(" · "+m.st.NodeName())
	}
}

func kv(k, v string) string {
	return "  " + dim.Render(fmt.Sprintf("%-15s", k)) + " " + v + "\n"
}

func (m Model) overview() string {
	if !m.loaded {
		return dim.Render("Reading the agent…")
	}
	if errors.Is(m.stErr, agent.ErrNotInstalled) {
		return "The noBGP agent is not installed.\n\n" +
			"  curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh\n\n" + dim.Render("Then press R to join a network.")
	}
	if errors.Is(m.stErr, agent.ErrNeedsAuth) {
		return "Reading this machine's agent needs root.\n\n" +
			"  Press " + bold.Render("A") + " to enter your sudo password once for this session,\n" +
			"  or run " + bold.Render("nobgp-omarchy setup") + " to let the bar and this app read status without one."
	}
	if m.stErr != nil {
		return red.Render(m.stErr.Error())
	}
	p := m.profile()
	var b strings.Builder
	b.WriteString(kv("Machine", m.st.Hostname))
	if n := m.st.NodeName(); n != m.st.Hostname {
		b.WriteString(kv("Node name", n))
	}
	b.WriteString(kv("Agent", m.st.Version+" · service "+m.st.Service))
	if p == nil {
		b.WriteString("\n" + yellow.Render("No agent profile is running. Press R to join a network."))
		return b.String()
	}
	if p.Registration.NodeID == "" {
		b.WriteString("\n" + yellow.Render("This machine is not registered. Press R to join a network."))
		return b.String()
	}
	b.WriteString(kv("Node id", p.Registration.NodeID))
	link := red.Render("offline")
	if p.Router.Connected {
		link = green.Render("connected")
		if p.Router.Transport != "" {
			link += " over " + strings.ToUpper(p.Router.Transport)
		}
	}
	b.WriteString(kv("Router", p.Router.URL+"  "+link))
	b.WriteString(kv("Address", strings.TrimSpace(p.Network.LocalIP+"  "+dim.Render(p.Network.TunDevice))))
	b.WriteString(kv("Name", p.Network.Domain))
	b.WriteString(kv("Peers", fmt.Sprint(len(p.Peers()))))
	if p.FS != nil {
		drive := p.FS.Mount + " (" + p.FS.Type + ")"
		if !p.FS.Mounted {
			drive += " " + yellow.Render("not mounted")
		}
		b.WriteString(kv("Network drive", drive))
	}
	mcp := "off"
	if p.MCP.Running {
		mcp = fmt.Sprintf("running on port %d", p.MCP.Port)
	} else if p.MCP.Granted {
		mcp = "granted, not running (local-mcp " + p.MCP.LocalMCP + ")"
	}
	b.WriteString(kv("Local MCP", mcp))
	b.WriteString(kv("Signed in", m.signedIn()))
	return b.String()
}

func (m Model) signedIn() string {
	switch {
	case errors.Is(m.dirErr, agent.ErrNotLoggedIn):
		return yellow.Render("no — press L to log in")
	case m.dirErr != nil:
		return red.Render(m.dirErr.Error())
	case m.st != nil && m.st.User != nil && m.st.User.Email != "":
		return m.st.User.Email
	case m.dir != nil:
		return "yes"
	}
	return dim.Render("…")
}

func (m Model) peerView() string {
	if m.stErr != nil || m.profile() == nil {
		return m.overview()
	}
	ps := m.peers()
	if len(ps) == 0 {
		return dim.Render("No peers yet. Add machines from the console (w) or run `nobgp register` on them.")
	}
	var b strings.Builder
	b.WriteString(dim.Render(fmt.Sprintf("  %-24s %-18s %s", "NAME", "ADDRESS", "NODE ID")) + "\n")
	for i, p := range ps {
		line := fmt.Sprintf("  %-24s %-18s %s", clip(p.Name, 24), p.Address, dim.Render(p.NodeID))
		if p.Reachable != nil && !*p.Reachable {
			line += " " + red.Render("unreachable")
		}
		if i == m.cursor[tabPeers] {
			line = selRow.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) serviceView() string {
	if errors.Is(m.dirErr, agent.ErrNotLoggedIn) {
		return "Services need you to be signed in to noBGP.\n\n  Press " + bold.Render("L") + " to log in."
	}
	if m.dirErr != nil {
		return red.Render(m.dirErr.Error())
	}
	if m.dir == nil {
		return dim.Render("Loading services…")
	}
	ss := m.services()
	if len(ss) == 0 {
		return dim.Render("No published services. Press a to publish one from this machine.")
	}
	self := ""
	if p := m.profile(); p != nil {
		self = p.Registration.NodeID
	}
	var b strings.Builder
	b.WriteString(dim.Render(fmt.Sprintf("  %-26s %-18s %s", "TITLE", "NODE", "URL")) + "\n")
	for i, s := range ss {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		node := s.NodeName
		if s.NodeID == self {
			node = "★ " + node
		}
		line := fmt.Sprintf("  %-26s %-18s %s", clip(title, 26), clip(node, 18), s.PublicURL)
		if i == m.cursor[tabServices] {
			line = selRow.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) deviceView() string {
	if m.stErr != nil && !errors.Is(m.stErr, agent.ErrNeedsAuth) {
		return m.overview()
	}
	p := m.profile()
	var b strings.Builder
	b.WriteString(dim.Render("What this machine shares with your network. Changes run `sudo nobgp config`.") + "\n\n")
	for i, s := range agent.Settings {
		cur := s.Current(p)
		if cur == "" {
			cur = dim.Render("—")
		}
		line := fmt.Sprintf("  %-22s %s", s.Label, cur)
		if i == m.cursor[tabDevice] {
			line = selRow.Render(line)
		}
		b.WriteString(line + "\n")
	}
	s := agent.Settings[m.cursor[tabDevice]]
	b.WriteString("\n" + dim.Render("  "+s.Help))
	return b.String()
}

func (m Model) formView() string {
	f := m.form
	var b strings.Builder
	b.WriteString(bold.Render(f.title) + "\n\n")
	for i, in := range f.inputs {
		b.WriteString(fmt.Sprintf("%-22s %s\n", dim.Render(f.labels[i]), in.View()))
	}
	if f.err != "" {
		b.WriteString("\n" + red.Render(f.err))
	}
	b.WriteString("\n" + dim.Render("enter: next/submit · tab: switch field · esc: cancel"))
	return boxed.Render(b.String())
}

func (m Model) footer() string {
	common := "1-4/tab: switch · j/k: move · r: refresh · w: web console · q: quit"
	switch {
	case m.form != nil || m.confirm != nil:
		return ""
	case m.tab == tabPeers:
		return "enter/c: copy address · n: copy name · i: copy node id · s: ssh\n" + common
	case m.tab == tabServices:
		return "o: open · c: copy URL · a: publish · S: share · d: delete · L: log in\n" + common
	case m.tab == tabDevice:
		return "enter: toggle/edit · e: edit value · X: restart agent\n" + common
	default:
		return "R: join a network · L: log in · A: sudo · X: restart agent\n" + common
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
