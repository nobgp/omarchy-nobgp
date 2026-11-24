import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

// noBGP bar widget. All state comes from `nobgp-omarchy bar`, which prints
// {"text","tooltip","class","peers"}; this file only renders it, so the
// agent-facing logic stays in the Go binary where it is tested.
BarWidget {
  id: root
  moduleName: "com.nobgp.status"

  property string glyph: "󰛵"
  property string tip: "noBGP"
  property string connState: "unknown"
  property bool refreshPending: false

  readonly property int intervalSec: Math.max(10, Number(root.setting("refreshIntervalSec", 30)) || 30)

  function refresh() {
    if (statusProc.running) refreshPending = true
    else statusProc.running = true
  }

  // install.sh ships in this plugin's own folder. It runs in a terminal the
  // user can see, so sudo prompts and the agent's sign-in link are theirs.
  function install() {
    const script = String(Qt.resolvedUrl("install.sh")).replace(/^file:\/\//, "")
    Quickshell.execDetached(["omarchy-launch-floating-terminal-with-presentation", "bash", script])
  }

  function apply(text) {
    try {
      const s = JSON.parse(String(text || "").trim())
      root.glyph = s.text || "󰛵"
      root.tip = s.tooltip || "noBGP"
      root.connState = s["class"] || "unknown"
    } catch (e) {
      root.glyph = "󰛵"
      root.tip = "noBGP is not installed yet. Click to install it."
      root.connState = "missing"
    }
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  ShellIpc {
    target: "com.nobgp.status"

    function refresh(): void {
      root.broadcast("refresh")
    }
  }

  Process {
    id: statusProc
    command: ["nobgp-omarchy", "bar"]
    stdout: StdioCollector {
      id: statusOut
      waitForEnd: true
    }
    onExited: function(exitCode) {
      root.apply(statusOut.text)
      if (root.refreshPending) {
        root.refreshPending = false
        Qt.callLater(root.refresh)
      }
    }
  }

  Timer {
    interval: root.intervalSec * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.glyph
    slotSize: Style.bar.statusSlot
    active: root.connState === "offline"
    dimmed: root.connState !== "connected" && root.connState !== "offline"
    tooltipText: root.tip
    onPressed: function(b) {
      // execDetached rather than bar.run: a third-party widget gets a
      // facade of the bar, and launching is all this needs.
      if (b === Qt.RightButton) Quickshell.execDetached(["omarchy-launch-or-focus-webapp", "app.nobgp.com", "https://app.nobgp.com"])
      else if (b === Qt.MiddleButton) root.refresh()
      else if (root.connState === "missing") root.install()
      else Quickshell.execDetached(["omarchy-launch-or-focus-tui", "nobgp-omarchy"])
    }
  }
}
