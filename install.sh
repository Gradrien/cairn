#!/usr/bin/env bash
# Build and install cairn, link its agent skill, and (with --herdr) add the herdr
# popup key and sidebar row.
set -euo pipefail
cd "$(dirname "$0")"

go vet ./...
go test ./...
go install .
echo "✓ cairn installé dans $(go env GOBIN || true)$( [ -z "$(go env GOBIN)" ] && echo "$(go env GOPATH)/bin")"

mkdir -p ~/.claude/skills
ln -sfn "$PWD/skill" ~/.claude/skills/cairn
echo "✓ skill cairn lié dans ~/.claude/skills/cairn"

# Claude Code hook: no push / PR on a cairn chantier without a human's go-ahead.
python3 - "$(go env GOPATH)/bin/cairn" <<'PY'
import json, os, sys
path = os.path.expanduser("~/.claude/settings.json")
d = json.load(open(path)) if os.path.exists(path) else {}
cmd = sys.argv[1] + " guard"
pre = d.setdefault("hooks", {}).setdefault("PreToolUse", [])
if not any(h.get("command", "").endswith("cairn guard") for e in pre for h in e.get("hooks", [])):
    pre.append({"matcher": "Bash", "hooks": [{"type": "command", "command": cmd, "timeout": 10}]})
    json.dump(d, open(path, "w"), indent=2, ensure_ascii=False)
    print("✓ hook Claude Code « cairn guard » ajouté dans ~/.claude/settings.json")
else:
    print("✓ hook Claude Code « cairn guard » déjà présent")
PY

if [[ "${1:-}" == "--herdr" ]]; then
  cfg="${HERDR_CONFIG_PATH:-$HOME/.config/herdr/config.toml}"
  cp "$cfg" "$cfg.bak-cairn"
  # Idempotent edit: drop any older cairn key block, bind prefix+a to the plugin
  # toggle, and add $cairn to the sidebar space rows if it is missing.
  python3 - "$cfg" <<'PY'
import re, sys
path = sys.argv[1]
s = open(path).read()
s = re.sub(r'\n*(# cairn[^\n]*\n)*\[\[keys\.command\]\]\n(?:(?!\[)[^\n]*\n?)*?command = "[^"]*cairn[^"]*"\n(?:(?!\[)[^\n]*\n?)*', '\n', s)
s = s.rstrip('\n') + '\n'
s += '\n# cairn : prefix+a bascule entre cairn et l\'espace d\'où tu viens.\n[[keys.command]]\nkey = "prefix+a"\ntype = "plugin_action"\ncommand = "cairn.toggle"\ndescription = "cairn"\n'
if '$cairn' not in s:
    m = re.search(r'^\[ui\.sidebar\.spaces\]\n((?:(?!\[)[^\n]*\n?)*)', s, re.M)
    if m and re.search(r'^rows\s*=', m.group(1), re.M):
        s = re.sub(r'(^\[ui\.sidebar\.spaces\]\n(?:(?!\[)[^\n]*\n)*?rows\s*=\s*\[.*)\]\s*$', r'\1, ["$cairn"]]', s, count=1, flags=re.M)
    else:
        s += '\n[ui.sidebar.spaces]\nrows = [["state_icon", "workspace"], ["branch", "git_status"], ["$cairn"]]\n'
open(path, 'w').write(s)
PY
  herdr plugin list 2>/dev/null | grep -q '(cairn)' || herdr plugin link "$PWD/herdr-plugin"
  herdr server reload-config >/dev/null
  "$(go env GOPATH)/bin/cairn" ensure || true
  echo "✓ herdr : l'espace « cairn » est dans la barre latérale (un clic l'ouvre), prefix+a bascule (sauvegarde : $cfg.bak-cairn)"
fi
