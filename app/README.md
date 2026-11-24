# noBGP for Omarchy

This is the Go source of the noBGP app for Omarchy. Run the commands below from `app/`.

The noBGP app for [Omarchy](https://omarchy.org) 4.x (Arch + Hyprland + the Quickshell desktop). It has three parts:

- **`nobgp-omarchy`**: a keyboard-driven terminal app in the lazygit style. It shows status and peers, lets you publish and share services, and has a **This device** tab for what this machine shares with your network.
- **A bar widget** (`nobgp.status`): connection state and peer count in the Omarchy bar. Left click opens the app, right click opens the web console and middle click refreshes.
- **Menu rows**: `SUPER + SPACE → noBGP` has entries for Open, Peers, Services, This Device, Web Console, Log In and Restart Agent.

The graphical parts of noBGP (network map, LLM chat, sharing) stay in the web console at app.nobgp.com. Setup installs it as an Omarchy web app (`noBGP Console` in the app launcher, beside `noBGP` for the terminal app).

The app drives the agent's own CLI (`nobgp … --json`) and never reads the agent's files or socket itself, so it keeps working across agent upgrades.

## Install

```bash
# 1. The agent, if this machine doesn't have it yet
curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh

# 2. This package (from omarchy/ in a checkout of nobgp/app)
make pkg
sudo pacman -U packaging/archlinux/nobgp-omarchy-*.pkg.tar.zst

# 3. As your own user: menu rows, bar widget, web console launcher
nobgp-omarchy setup
```

`nobgp-omarchy setup --remove` undoes step 3. Setup only writes to `~/.config/omarchy/` and `~/.local/share/`. It keeps your own menu entries, saves a `.bak-nobgp` copy of the menu file before it changes it, and can be run again safely.

A suggested hotkey for `~/.config/hypr/bindings.lua`. Apps go on the `Super + Shift` plane, and `U` is unused there in a default Omarchy 4 install:

```lua
o.bind("SUPER + SHIFT + U", "noBGP", { launch = "omarchy-launch-or-focus-tui nobgp-omarchy" })
```

## The app

```text
nobgp-omarchy                 Overview
nobgp-omarchy peers           Peers: copy address/name/id, ssh
nobgp-omarchy services        Services: open, copy URL, publish from this machine, share, delete
nobgp-omarchy device          This device: allow-root, run-as account, allowed tools,
                              shared folders, local MCP, loopback reach, host services, mount
nobgp-omarchy bar             the bar widget's JSON (also works as a Waybar-style module)
```

Keys: `1–4`/`tab` switch tabs, `j/k` move, `r` refresh, `w` web console, `R` join a network, `L` log in, `A` sudo, `X` restart agent, `q` quit.

Any change that needs root or a browser (host settings, `register`, `login`, `publish`) runs in the same terminal, so you see sudo's prompt and the agent's own confirmations. The app never passes `--yes` for you.

## Privileges

`nobgp status` needs root, because the agent's API socket is root's. The package installs `/etc/sudoers.d/nobgp-omarchy`, which lets the `wheel` group run exactly `nobgp status --json` without a password. That rule is what lets the bar widget show the connection state. Without it, the bar only shows whether the agent's service is running, and the app asks you to press `A` to authenticate. Every other privileged action asks for your password as usual.

Services and networks use your own `nobgp login`, not root.

## Develop

```bash
make test        # unit tests: real agent status fixture, menu merge, TUI flows
make build       # build/nobgp-omarchy
NOBGP_BIN=/path/to/fake-nobgp build/nobgp-omarchy   # run against a stub CLI
```

Layout: `internal/agent` (CLI wrapper and JSON types), `internal/tui` (Bubble Tea), `internal/bar` (bar state), `internal/setup` (Omarchy integration), `desktop/plugin` (the QML bar widget), `packaging/` (PKGBUILD, sudoers, desktop file, icon).

## Not yet

- Desktop notifications when the node goes offline or a peer joins. This needs a change signal from the agent; Omarchy asks its shell components not to poll for this.
- Approving access requests: the agent CLI has no command for it yet, so the app sends you to the web console for now.
