// Package setup wires the app into an Omarchy desktop: menu rows, the bar
// widget, and a web-app launcher for the graphical console. Everything goes
// into the user's own ~/.config and ~/.local, never /usr/share/omarchy, and
// every step is safe to run again.
package setup

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nobgp/omarchy-nobgp/app/desktop"
	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
)

// Paths are resolved from HOME so tests can point them at a temp dir.
type Paths struct {
	Home string
}

func (p Paths) MenuFile() string {
	return filepath.Join(p.Home, ".config/omarchy/extensions/omarchy-menu.jsonc")
}

func (p Paths) PluginDir() string {
	return filepath.Join(p.Home, ".config/omarchy/plugins", desktop.PluginID)
}

// LegacyPluginDir is where 0.1.0 installed the widget, as nobgp.status.
func (p Paths) LegacyPluginDir() string {
	return filepath.Join(p.Home, ".config/omarchy/plugins", desktop.LegacyPluginID)
}

// gitManaged reports whether the plugin folder is a git checkout, which is how
// `omarchy plugin add` (and so the plugin marketplace) installs it. Setup then
// leaves its files to `omarchy plugin update`.
func (p Paths) gitManaged() bool {
	_, err := os.Stat(filepath.Join(p.PluginDir(), ".git"))
	return err == nil
}

func (p Paths) WebAppDesktop() string {
	return filepath.Join(p.Home, ".local/share/applications/noBGP.desktop")
}

// WebAppName is the launcher name of the web console. It differs from the
// terminal app's "noBGP" so the app launcher does not list two "noBGP" rows.
const WebAppName = "noBGP Console"

// IconPath is installed by the package.
const IconPath = "/usr/share/icons/hicolor/scalable/apps/nobgp.svg"

// run is replaceable in tests.
var run = func(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout, c.Stderr = io.Discard, io.Discard
	return c.Run()
}

var have = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Install runs every step and reports each one to w.
func Install(w io.Writer, p Paths) error {
	var failed []string
	step := func(name string, err error) {
		if err != nil {
			fmt.Fprintf(w, "  ✗ %s: %v\n", name, err)
			failed = append(failed, name)
		} else {
			fmt.Fprintf(w, "  ✓ %s\n", name)
		}
	}

	if !have("omarchy-shell") {
		fmt.Fprintln(w, "This does not look like an Omarchy desktop (no omarchy-shell); installing the files anyway.")
	}

	step("Menu entries (SUPER+SPACE → noBGP)", installMenu(p))
	step("Bar widget ("+desktop.PluginID+")", installPlugin(p))
	step("Web console launcher (noBGP Console)", installWebApp(p))

	fmt.Fprintln(w)
	checkAgent(w)

	if len(failed) > 0 {
		return fmt.Errorf("%d step(s) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// Remove undoes Install.
func Remove(w io.Writer, p Paths) error {
	if b, err := os.ReadFile(p.MenuFile()); err == nil {
		if out, changed := RemoveMenu(string(b)); changed {
			if err := os.WriteFile(p.MenuFile(), []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Fprintln(w, "  ✓ Removed menu entries")
		}
	}
	if p.gitManaged() {
		fmt.Fprintf(w, "  · Bar widget left in place: it was added with `omarchy plugin add`; remove it with `omarchy plugin remove %s`\n", desktop.PluginID)
	} else if _, err := os.Stat(p.PluginDir()); err == nil {
		if have("omarchy-plugin-disable") {
			_ = run("omarchy-plugin-disable", desktop.PluginID)
		}
		if err := os.RemoveAll(p.PluginDir()); err != nil {
			return err
		}
		if have("omarchy-shell") {
			_ = run("omarchy-shell", "shell", "rescanPlugins")
		}
		fmt.Fprintln(w, "  ✓ Removed bar widget")
	}
	if _, err := os.Stat(p.WebAppDesktop()); err == nil && have("omarchy-webapp-remove") {
		if err := run("omarchy-webapp-remove", WebAppName); err == nil {
			fmt.Fprintln(w, "  ✓ Removed web console launcher")
		}
	}
	return nil
}

func installMenu(p Paths) error {
	path := p.MenuFile()
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}
	out, err := MergeMenu(existing, desktop.MenuEntries)
	if err != nil {
		return fmt.Errorf("%s: %w (fix the file or move it aside, then run setup again)", path, err)
	}
	if out == existing {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if existing != "" {
		if err := os.WriteFile(path+".bak-nobgp", []byte(existing), 0o644); err != nil {
			return err
		}
	}
	return writeAtomic(path, []byte(out), 0o644)
}

func installPlugin(p Paths) error {
	removeLegacyPlugin(p)
	dir := p.PluginDir()
	if p.gitManaged() {
		// Added from git by `omarchy plugin add`: its files are git's to update.
		return enablePlugin()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	err := fs.WalkDir(desktop.Plugin, "plugin", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := desktop.Plugin.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		return writeAtomic(filepath.Join(dir, filepath.Base(path)), b, mode)
	})
	if err != nil {
		return err
	}
	return enablePlugin()
}

// removeLegacyPlugin takes away a widget 0.1.0 installed as nobgp.status, so
// the bar does not show two noBGP icons. Best effort: a failure leaves it.
func removeLegacyPlugin(p Paths) {
	dir := p.LegacyPluginDir()
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if have("omarchy-plugin-disable") {
		_ = run("omarchy-plugin-disable", desktop.LegacyPluginID)
	}
	_ = os.RemoveAll(dir)
}

func enablePlugin() error {
	if !have("omarchy-shell") {
		return nil
	}
	// The shell hot-reloads plugin code; a rescan picks up a new plugin.
	if err := run("omarchy-shell", "shell", "rescanPlugins"); err != nil {
		return fmt.Errorf("installed, but omarchy-shell did not rescan: %w (run `omarchy plugin enable %s` later)", err, desktop.PluginID)
	}
	if err := run("omarchy-plugin-enable", desktop.PluginID); err != nil {
		return fmt.Errorf("installed, but enabling failed: %w (run `omarchy plugin enable %s`)", err, desktop.PluginID)
	}
	return nil
}

func installWebApp(p Paths) error {
	if _, err := os.Stat(p.WebAppDesktop()); err == nil {
		return nil
	}
	if !have("omarchy-webapp-install") {
		return fmt.Errorf("omarchy-webapp-install not found")
	}
	icon := IconPath
	if _, err := os.Stat(icon); err != nil {
		icon = "https://app.nobgp.com/favicon.ico"
	}
	return run("omarchy-webapp-install", WebAppName, "https://app.nobgp.com", icon)
}

func checkAgent(w io.Writer) {
	c, err := agent.New()
	if err != nil {
		fmt.Fprintln(w, "The noBGP agent is not installed. Install it with:")
		fmt.Fprintln(w, "    curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh")
		fmt.Fprintln(w, "then open noBGP from the menu (SUPER+SPACE → noBGP) to join a network.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := c.Status(ctx)
	switch {
	case err == agent.ErrNeedsAuth:
		fmt.Fprintln(w, "The bar cannot read the agent status without a password.")
		fmt.Fprintln(w, "The nobgp-omarchy package installs /etc/sudoers.d/nobgp-omarchy, which allows")
		fmt.Fprintf(w, "exactly `%s status --json` for the wheel group. Check that you are in wheel\n", c.Bin)
		fmt.Fprintln(w, "and that the agent lives at /usr/local/bin/nobgp or /usr/bin/nobgp.")
	case err != nil:
		fmt.Fprintf(w, "Agent: %v\n", err)
	default:
		p := st.Main()
		switch {
		case p == nil || p.Registration.NodeID == "":
			fmt.Fprintln(w, "Agent installed, not registered yet: open noBGP and press R to join a network.")
		case p.Router.Connected:
			fmt.Fprintf(w, "Agent connected as %s.\n", st.NodeName())
		default:
			fmt.Fprintf(w, "Agent registered as %s, currently offline.\n", st.NodeName())
		}
	}
}

func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nobgp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
