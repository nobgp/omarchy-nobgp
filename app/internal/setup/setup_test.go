package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nobgp/omarchy-nobgp/app/desktop"
)

// stubShell replaces the commands setup runs, recording them.
func stubShell(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	oldRun, oldHave := run, have
	run = func(name string, args ...string) error {
		calls = append(calls, name)
		return nil
	}
	have = func(string) bool { return true }
	t.Cleanup(func() { run, have = oldRun, oldHave })
	return &calls
}

func TestInstallPluginWritesFiles(t *testing.T) {
	stubShell(t)
	p := Paths{Home: t.TempDir()}
	if err := installPlugin(p); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"manifest.json", "Status.qml", "install.sh"} {
		if _, err := os.Stat(filepath.Join(p.PluginDir(), f)); err != nil {
			t.Errorf("%s not installed: %v", f, err)
		}
	}
	st, err := os.Stat(filepath.Join(p.PluginDir(), "install.sh"))
	if err == nil && st.Mode().Perm()&0o100 == 0 {
		t.Errorf("install.sh is not executable: %v", st.Mode())
	}
}

// A plugin added with `omarchy plugin add` is a git checkout; setup must not
// write over its files, only enable it.
func TestInstallPluginLeavesGitCheckout(t *testing.T) {
	calls := stubShell(t)
	p := Paths{Home: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(p.PluginDir(), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(p.PluginDir(), "Status.qml")
	if err := os.WriteFile(marker, []byte("from git"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installPlugin(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "from git" {
		t.Errorf("setup overwrote the git checkout's Status.qml: %q", b)
	}
	if len(*calls) == 0 || (*calls)[len(*calls)-1] != "omarchy-plugin-enable" {
		t.Errorf("plugin not enabled; calls: %v", *calls)
	}
}

func TestInstallPluginRemovesLegacyID(t *testing.T) {
	stubShell(t)
	p := Paths{Home: t.TempDir()}
	if err := os.MkdirAll(p.LegacyPluginDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installPlugin(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.LegacyPluginDir()); !os.IsNotExist(err) {
		t.Errorf("legacy %s plugin still present", desktop.LegacyPluginID)
	}
}
