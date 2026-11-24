// Package desktop holds the files the app installs into an Omarchy desktop:
// the bar widget plugin and the menu entries.
package desktop

import "embed"

// Plugin is the `com.nobgp.status` bar-widget plugin, installed into
// ~/.config/omarchy/plugins/com.nobgp.status/ (or added there from git by
// the Omarchy plugin marketplace).
//
//go:embed plugin/manifest.json plugin/Status.qml plugin/install.sh
var Plugin embed.FS

// PluginID is the plugin's manifest id.
const PluginID = "com.nobgp.status"

// LegacyPluginID is the id 0.1.0 used; setup removes a plugin left under it.
const LegacyPluginID = "nobgp.status"

// MenuEntries are the rows added to the Omarchy menu (SUPER+SPACE), as
// JSONC object members. Ids are dotted: everything sits under one "noBGP"
// row on the root menu.
const MenuEntries = `  "nobgp": {"icon":"󰛳","label":"noBGP","description":"Private network for your machines"},
  "nobgp.open": {"icon":"󰛳","label":"Open noBGP","action":"omarchy-launch-or-focus-tui nobgp-omarchy"},
  "nobgp.peers": {"icon":"󰒍","label":"Peers","description":"Machines on your noBGP network","action":"omarchy-launch-tui --app-id=org.omarchy.nobgp-peers nobgp-omarchy peers"},
  "nobgp.services": {"icon":"󰖟","label":"Services","description":"Publish and share services from this machine","action":"omarchy-launch-tui --app-id=org.omarchy.nobgp-services nobgp-omarchy services"},
  "nobgp.device": {"icon":"󰒓","label":"This Device","description":"What this machine shares with your network","action":"omarchy-launch-tui --app-id=org.omarchy.nobgp-device nobgp-omarchy device"},
  "nobgp.console": {"icon":"󰖟","label":"Web Console","description":"Network map, LLMs and sharing","action":"omarchy-launch-or-focus-webapp app.nobgp.com https://app.nobgp.com"},
  "nobgp.login": {"icon":"󰍂","label":"Log In","action":"omarchy-launch-floating-terminal-with-presentation nobgp login"},
  "nobgp.restart": {"icon":"󰜉","label":"Restart Agent","action":"omarchy-launch-floating-terminal-with-presentation sudo nobgp service restart"},
`
