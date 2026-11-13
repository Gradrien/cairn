// cairn: shows cairn's space, or goes back to the one you came from.
// Written by cairn's install.sh --herdr-app; run it again to update it.
// Opened by a key (mod.open:cairn-toggle), it runs `cairn open` and closes.
import { View, div } from "gpui-kit";
import { exec, notify, props, closeOverlay } from "herdr";

const CAIRN = "@CAIRN@";
const PATH = "@PATH@";

export default class CairnToggle extends View {
  init(_props, cx) {
    const overlay = props()?.overlay;
    if (overlay == null) return; // herdr-app check renders it once: run nothing
    cx.spawn(async () => {
      try {
        const r = await exec("/usr/bin/env", ["PATH=" + PATH, "CAIRN_MUX=herdr-app", CAIRN, "open"]);
        if (r.code !== 0) notify({ title: "cairn", body: (r.stderr || r.stdout || "cairn open").trim(), level: "error" });
      } catch (e) {
        notify({ title: "cairn", body: String(e), level: "error" });
      }
      closeOverlay(overlay);
    });
  }

  render() {
    return div();
  }
}
