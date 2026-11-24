#!/bin/bash
# Installs what the noBGP bar widget needs: the noBGP agent and the noBGP app
# for Omarchy (nobgp-omarchy). The bar icon runs this in a terminal when the
# app is missing; you can also run it yourself:
#
#   bash ~/.config/omarchy/plugins/com.nobgp.status/install.sh
#
# It asks for your sudo password itself and never stores it. Nothing runs
# when the plugin is added: Omarchy plugins have no install hook.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\n\033[31mnoBGP: %s\033[0m\n' "$*" >&2; read -r -p "Press Enter to close. " _ || true; exit 1; }

[ "$(id -u)" -ne 0 ] || die "Run this as your own user, not as root."
command -v pacman >/dev/null || die "This installer is for Omarchy (Arch Linux)."

# 1. The agent: noBGP's own installer, which shows a sign-in link to add this
#    machine to your account.
if command -v nobgp >/dev/null || [ -x /usr/local/bin/nobgp ]; then
	say "noBGP agent: already installed"
else
	say "Installing the noBGP agent (downloads.nobgp.com)"
	curl -fsSL https://downloads.nobgp.com/agent/install.sh | sh || die "The agent didn't install (see above)."
fi

# 2. The app: from the AUR when it's there, else built from the source that
#    ships with this plugin (app/).
if command -v nobgp-omarchy >/dev/null; then
	say "noBGP app: already installed"
elif command -v yay >/dev/null && yay -Si nobgp-omarchy >/dev/null 2>&1; then
	say "Installing nobgp-omarchy from the AUR"
	yay -S --needed nobgp-omarchy || die "Installing nobgp-omarchy from the AUR failed."
elif [ -f "$here/app/Makefile" ]; then
	say "Building nobgp-omarchy from source (needs go and base-devel)"
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
