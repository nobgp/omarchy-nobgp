package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
)

type fake struct {
	st      *agent.Status
	stErr   error
	dir     *agent.Directory
	dirErr  error
	deleted []string
	shared  map[string][]string
}

func (f *fake) Status(context.Context) (*agent.Status, error)       { return f.st, f.stErr }
func (f *fake) Directory(context.Context) (*agent.Directory, error) { return f.dir, f.dirErr }
func (f *fake) DeleteService(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fake) ShareService(_ context.Context, id string, e []string) error {
	if f.shared == nil {
		f.shared = map[string][]string{}
	}
	f.shared[id] = e
	return nil
}
func (f *fake) LoginArgs() []string                  { return []string{"nobgp", "login"} }
func (f *fake) RegisterArgs(n, name string) []string { return []string{"sudo", "nobgp", "register"} }
func (f *fake) ConfigArgs(s agent.Setting, v string) []string {
	return []string{"sudo", "nobgp", "config", "--" + s.Key + "=" + v}
}
func (f *fake) RestartArgs() []string { return []string{"sudo", "nobgp", "service", "restart"} }
func (f *fake) PublishArgs(p agent.Publish) ([]string, error) {
	return (&agent.Client{Bin: "nobgp"}).PublishArgs(p)
}

func loaded(t *testing.T) (*fake, Model) {
	t.Helper()
	b, err := os.ReadFile("../agent/testdata/status.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := agent.ParseStatus(b)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := os.ReadFile("../agent/testdata/directory-envelope.json")
	dir, err := agent.ParseDirectory(db)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{st: st, dir: dir}
	m := New(f, "")
	m = step(m, statusMsg{st, nil})
	m = step(m, dirMsg{dir, nil})
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return f, m
}

func step(m Model, msg tea.Msg) Model {
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func key(m Model, k string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	nm, cmd := m.Update(msg)
	return nm.(Model), cmd
}

func TestOverviewShowsAgentState(t *testing.T) {
	_, m := loaded(t)
	v := m.View()
	for _, want := range []string{"connected", "workstation", "11111111-1111-4111-8111-111111111111", "router.example.net", "/mnt/nobgp (fuse)", "running on port 35945"} {
		if !strings.Contains(v, want) {
			t.Errorf("overview lacks %q:\n%s", want, v)
		}
	}
}

func TestTabsAndPeers(t *testing.T) {
	_, m := loaded(t)
	m, _ = key(m, "2")
	v := m.View()
	if !strings.Contains(v, "laptop") || !strings.Contains(v, "100.64.0.7") {
		t.Fatalf("peers view:\n%s", v)
	}
	if strings.Contains(v, "192.168.1.50") {
		t.Error("this machine must not be listed as its own peer")
	}
	m, _ = key(m, "j")
	if m.cursor[tabPeers] != 1 {
		t.Errorf("cursor = %d", m.cursor[tabPeers])
	}
	m, _ = key(m, "tab")
	if m.tab != tabServices {
		t.Errorf("tab = %d", m.tab)
	}
}

func TestServicesListThisMachineFirst(t *testing.T) {
	_, m := loaded(t)
	m, _ = key(m, "3")
	ss := m.services()
	if len(ss) != 2 || ss[0].NodeName != "workstation" {
		t.Fatalf("services = %+v", ss)
	}
	if !strings.Contains(m.View(), "★ workstation") {
		t.Error("this machine's services are marked")
	}
}

func TestDeleteNeedsConfirmation(t *testing.T) {
	f, m := loaded(t)
	m, _ = key(m, "3")
	m, _ = key(m, "d")
	if m.confirm == nil {
		t.Fatal("delete must ask first")
	}
	m, cmd := key(m, "n")
	if cmd != nil {
		cmd() // only a flash
	}
	if len(f.deleted) != 0 {
		t.Fatal("deleted without a yes")
	}
	m, _ = key(m, "d")
	_, cmd = key(m, "y")
	if cmd == nil {
		t.Fatal("no command after yes")
	}
	if _, ok := cmd().(execDoneMsg); !ok || len(f.deleted) != 1 || f.deleted[0] != "s1" {
		t.Fatalf("deleted = %v", f.deleted)
	}
}

func TestPublishFormValidates(t *testing.T) {
	_, m := loaded(t)
	m, _ = key(m, "3")
	m, _ = key(m, "a")
	if m.form == nil {
		t.Fatal("no form")
	}
	// Submit with no URL or command: the form stays open with an error.
	m, _ = key(m, "enter")
	m, _ = key(m, "enter")
	m, _ = key(m, "enter")
	if m.form == nil || m.form.err == "" {
		t.Fatalf("form should show an error, got %+v", m.form)
	}
	m, _ = key(m, "esc")
	if m.form != nil {
		t.Fatal("esc closes the form")
	}
}

func TestDeviceToggleAsksThenRunsSudoConfig(t *testing.T) {
	_, m := loaded(t)
	m, _ = key(m, "4")
	v := m.View()
	if !strings.Contains(v, "Allow root") || !strings.Contains(v, "fs,command,logs") {
		t.Fatalf("device view:\n%s", v)
	}
	m, _ = key(m, "enter") // Allow root: true -> false
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "Allow root to false") {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	// A list setting opens an editor prefilled with the current value.
	m, _ = key(m, "n")
	m.cursor[tabDevice] = 3 // Shared folders
	m, _ = key(m, "enter")
	if m.form == nil || m.form.inputs[0].Value() != "/" {
		t.Fatalf("form = %+v", m.form)
	}
	m.form.inputs[0].SetValue("relative/path")
	m, _ = key(m, "enter")
	if m.form == nil || !strings.Contains(m.form.err, "absolute") {
		t.Fatalf("bad path must be refused before sudo: %+v", m.form)
	}
}

func TestNeedsAuthAndNotLoggedIn(t *testing.T) {
	m := New(&fake{}, "")
	m = step(m, statusMsg{nil, agent.ErrNeedsAuth})
	m = step(m, dirMsg{nil, agent.ErrNotLoggedIn})
	if v := m.View(); !strings.Contains(v, "needs sudo") || !strings.Contains(v, "Press A") {
		t.Errorf("auth view:\n%s", v)
	}
	m, _ = key(m, "3")
	if v := m.View(); !strings.Contains(v, "Press L to log in") {
		t.Errorf("services view:\n%s", v)
	}
}

func TestMissingAgent(t *testing.T) {
	m := New(&fake{}, "")
	m = step(m, statusMsg{nil, agent.ErrNotInstalled})
	if v := m.View(); !strings.Contains(v, "install.sh") {
		t.Errorf("view:\n%s", v)
	}
}
