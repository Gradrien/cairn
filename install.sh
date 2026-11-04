#!/usr/bin/env bash
# Build and install cairn, link its agent skill, add the Claude Code push guard,
# and (with --herdr) the herdr plugin, sidebar button and prefix+a key.
set -euo pipefail
cd "$(dirname "$0")"

for t in go gh git python3; do
  command -v "$t" >/dev/null || { echo "✗ $t est introuvable : installe-le d'abord" >&2; exit 1; }
done
command -v herdr >/dev/null || echo "! herdr est introuvable : cairn en a besoin pour tourner"
gh auth status >/dev/null 2>&1 || echo "! gh n'est pas connecté : lance « gh auth login » (PR, CI, images)"

go vet ./...
go test ./...
go install .
bindir="$(go env GOBIN)"; [ -n "$bindir" ] || bindir="$(go env GOPATH)/bin"
cairn_bin="$bindir/cairn"
echo "✓ cairn installé dans $bindir"

# First install: ask for the two personal settings, the rest has team defaults.
conf="${CAIRN_HOME:-$HOME/.cairn}/config.json"
if [[ ! -f "$conf" && -t 0 ]]; then
  repo=""
  for p in ~/Applications/Yak-App ~/Yak-App ~/code/Yak-App ~/dev/Yak-App ~/Projects/Yak-App; do
    [[ -d "$p/.git" ]] && { repo="$p"; break; }
  done
  read -r -p "Checkout principal de Yak-App [${repo:-chemin}] : " ans; repo="${ans:-$repo}"
  repo="${repo/#\~/$HOME}"
  [[ -d "$repo" ]] && git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || { echo "✗ $repo n'est pas un dépôt git" >&2; exit 1; }
  handle="$(git -C "$repo" config user.email | cut -d@ -f1 | cut -d. -f1 | tr '[:upper:]' '[:lower:]')"
  read -r -p "Ton handle Sprintosaur [$handle] : " ans; handle="${ans:-$handle}"
  mkdir -p "$(dirname "$conf")"
  python3 - "$conf" "$repo" "$handle" <<'PY2'
import json, sys
conf, repo, handle = sys.argv[1:]
json.dump({"repo": repo, "assignee": handle}, open(conf, "w"), indent=2)
PY2
  echo "✓ config écrite dans $conf (les autres réglages prennent les valeurs de l'équipe)"
fi
creds="$HOME/.claude/skills/sprintosaur/credentials.env"
[[ -f "$creds" || -n "${SPRINTOSAUR_TOKEN:-}" ]] || echo "! identifiants Sprintosaur introuvables ($creds) : installe le skill sprintosaur ou renseigne « sprintosaur_credentials » dans $conf"

mkdir -p ~/.claude/skills
ln -sfn "$PWD/skill" ~/.claude/skills/cairn
echo "✓ skill cairn lié dans ~/.claude/skills/cairn"

# Claude Code hook: no push / PR on a cairn chantier without a human's go-ahead.
python3 - "$cairn_bin" <<'PY'
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
  "$cairn_bin" ensure || true
  echo "✓ herdr : l'espace « cairn » est dans la barre latérale (un clic l'ouvre), prefix+a bascule (sauvegarde : $cfg.bak-cairn)"
fi
