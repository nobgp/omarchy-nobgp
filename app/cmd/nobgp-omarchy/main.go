// nobgp-omarchy is the noBGP app for Omarchy.
//
//	nobgp-omarchy                 open the TUI
//	nobgp-omarchy peers|services|device
//	                              open the TUI on that tab
//	nobgp-omarchy bar             one line of JSON for the bar widget
//	nobgp-omarchy setup [--remove]
//	                              add (or remove) the menu rows, bar widget
//	                              and web console launcher
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nobgp/omarchy-nobgp/app/internal/agent"
	"github.com/nobgp/omarchy-nobgp/app/internal/bar"
	"github.com/nobgp/omarchy-nobgp/app/internal/setup"
	"github.com/nobgp/omarchy-nobgp/app/internal/tui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `noBGP for Omarchy

Usage:
  nobgp-omarchy [peers|services|device]   open the app (on a tab)
  nobgp-omarchy bar                       print the bar widget's JSON
  nobgp-omarchy setup [--remove]          add the menu, bar widget and web console
  nobgp-omarchy version

The app drives the noBGP agent's own CLI (nobgp). Set NOBGP_BIN to use a
different binary.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "", "tui", "peers", "services", "device", "settings":
		start := cmd
		if cmd == "tui" && len(args) > 1 {
			start = args[1]
		}
		return runTUI(start)
	case "bar":
		return runBar()
	case "setup":
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		p := setup.Paths{Home: home}
		if len(args) > 1 && args[1] == "--remove" {
			err = setup.Remove(os.Stdout, p)
		} else {
			fmt.Println("Setting up noBGP for Omarchy:")
			err = setup.Install(os.Stdout, p)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Println("nobgp-omarchy", version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

func runTUI(start string) int {
	c, err := agent.New()
	var be tui.Backend
	if err != nil {
		be = missingAgent{}
	} else {
		be = c
	}
	p := tea.NewProgram(tui.New(be, start), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runBar() int {
	var s bar.State
	c, err := agent.New()
	if err != nil {
		s = bar.State{Text: bar.GlyphStopped, Class: "missing",
			Tooltip: "noBGP agent is not installed\nOpen noBGP from the menu to set it up"}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s = bar.Summarize(ctx, c, time.Now())
	}
	b, _ := json.Marshal(s)
	fmt.Println(string(b))
	return 0
}

// missingAgent lets the TUI open and explain how to install the agent.
type missingAgent struct{}

func (missingAgent) Status(context.Context) (*agent.Status, error) { return nil, agent.ErrNotInstalled }
func (missingAgent) Directory(context.Context) (*agent.Directory, error) {
	return nil, agent.ErrNotInstalled
}
func (missingAgent) DeleteService(context.Context, string) error { return agent.ErrNotInstalled }
func (missingAgent) ShareService(context.Context, string, []string) error {
	return agent.ErrNotInstalled
}
func (missingAgent) LoginArgs() []string                       { return notInstalled() }
func (missingAgent) RegisterArgs(string, string) []string      { return notInstalled() }
func (missingAgent) ConfigArgs(agent.Setting, string) []string { return notInstalled() }
func (missingAgent) RestartArgs() []string                     { return notInstalled() }
func (missingAgent) PublishArgs(agent.Publish) ([]string, error) {
	return nil, errors.New("the noBGP agent is not installed")
}

func notInstalled() []string {
	return []string{"sh", "-c", "echo 'The noBGP agent is not installed. Install it with:'; echo '  curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh'; exit 1"}
}
