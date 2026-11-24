package tui

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// execDoneMsg arrives when an interactive command handed to the terminal
// returns.
type execDoneMsg struct {
	what string
	err  error
}

// pauseScript runs the command, then waits for a key, so the owner can read
// what sudo or the agent printed before the TUI takes the screen back.
const pauseScript = `"$@"; rc=$?; echo; if [ $rc -eq 0 ]; then printf '\033[32mDone.\033[0m'; else printf '\033[31mExited with %s.\033[0m' "$rc"; fi; printf ' Press any key to return to noBGP… '; read -rsn1; exit $rc`

// runInTerminal suspends the TUI and runs argv in the same terminal.
func runInTerminal(what string, argv []string) tea.Cmd {
	args := append([]string{"-c", pauseScript, "nobgp-omarchy"}, argv...)
	c := exec.Command("bash", args...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{what: what, err: err} })
}

// copyToClipboard uses wl-copy, which Omarchy ships.
func copyToClipboard(s string) error {
	c := exec.Command("wl-copy")
	c.Stdin = strings.NewReader(s)
	return c.Run()
}

// openURL prefers Omarchy's web-app launcher, so a service opens as an app
// window like the rest of the desktop, and falls back to xdg-open.
func openURL(url string) error {
	if p, err := exec.LookPath("omarchy-launch-webapp"); err == nil {
		return exec.Command(p, url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}
