#!/bin/bash
# Installs what the noBGP bar widget needs: the noBGP agent and the noBGP app
# for Omarchy (nobgp-omarchy). The bar icon runs this in a terminal when the
# app is missing; you can also run it yourself:
#
#   bash ~/.config/omarchy/plugins/com.nobgp.status/install.sh
#
# It never downloads and runs code on your behalf. The agent comes from
# noBGP's own installer, which you run yourself; the app comes from the AUR
# or is built from the source in this repository. Nothing runs when the
# plugin is added: Omarchy plugins have no install hook.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\n\033[31mnoBGP: %s\033[0m\n' "$*" >&2; read -r -p "Press Enter to close. " _ || true; exit 1; }
have_agent() { command -v nobgp >/dev/null || [ -x /usr/local/bin/nobgp ]; }

[ "$(id -u)" -ne 0 ] || die "Run this as your own user, not as root."
command -v pacman >/dev/null || die "This installer is for Omarchy (Arch Linux)."

# 1. The agent: you run noBGP's official installer yourself. It shows a
#    sign-in link to add this machine to your noBGP account.
if have_agent; then
	say "noBGP agent: already installed"
else
	agent_cmd='curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh'
	say "The noBGP agent isn't installed yet"
	cat <<EOF
This plugin doesn't run downloaded code for you. Install the agent with
noBGP's official installer: open a new terminal (SUPER + ENTER) and run

    $agent_cmd

It's on your clipboard. The installer shows a sign-in link that adds this
machine to your noBGP account. When it has finished, come back here.
EOF
	printf '%s' "$agent_cmd" | wl-copy 2>/dev/null || true
	while ! have_agent; do
		read -r -p $'\nPress Enter when the agent is installed (or Ctrl-C to stop). ' _ || exit 1
		have_agent || printf 'The agent is not installed yet.\n'
	done
	say "noBGP agent: installed"
fi

# 2. The app: from the AUR when it's there, else built from the source that
#    ships with this plugin (app/).
if command -v nobgp-omarchy >/dev/null; then
	say "noBGP app: already installed"
elif command -v yay >/dev/null && yay -Si nobgp-omarchy >/dev/null 2>&1; then
	say "Installing nobgp-omarchy from the AUR"
	yay -S --needed nobgp-omarchy || die "Installing nobgp-omarchy from the AUR failed."
elif [ -f "$here/app/Makefile" ]; then
	say "Building nobgp-omarchy from the source in this plugin (needs go and base-devel)"
	if ! command -v go >/dev/null || ! command -v makepkg >/dev/null; then
		sudo pacman -S --needed go base-devel || die "Couldn't install go and base-devel."
	fi
	make -C "$here/app" clean pkg || die "The build failed (see above)."
	sudo pacman -U --noconfirm "$here"/app/packaging/archlinux/nobgp-omarchy-*.pkg.tar.zst || die "Installing the package failed."
	make -C "$here/app" clean >/dev/null
else
	die "nobgp-omarchy isn't in the AUR yet and this copy has no source. See https://github.com/nobgp/omarchy-nobgp"
fi

# 3. Menu rows and the web console launcher. The bar widget is this plugin.
say "Adding noBGP to the Omarchy menu"
nobgp-omarchy setup || true

say "Done. Open noBGP from the bar icon or SUPER + SPACE → noBGP."
read -r -p "Press Enter to close. " _ || true
