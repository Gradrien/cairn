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
# Go's bin folder is often missing from PATH: link cairn into ~/.local/bin instead.
case ":$PATH:" in
  *":$bindir:"*) ;;
  *)
    mkdir -p ~/.local/bin
    ln -sfn "$cairn_bin" ~/.local/bin/cairn
    case ":$PATH:" in
      *":$HOME/.local/bin:"*) echo "✓ cairn lié dans ~/.local/bin (déjà dans ton PATH)" ;;
      *) echo "! ajoute ~/.local/bin à ton PATH (ex. dans ~/.zshrc : export PATH=\"\$HOME/.local/bin:\$PATH\")" ;;
    esac ;;
esac

# First install: ask for a first project. More later with « cairn add » in any repository.
conf="${CAIRN_HOME:-$HOME/.cairn}/config.json"
if [[ ! -f "$conf" && -t 0 ]]; then
  read -r -p "Premier dépôt à suivre (vide pour plus tard) : " repo
  repo="${repo/#\~/$HOME}"
  if [[ -n "$repo" ]]; then
    read -r -p "Tracker [local / linear / sprintosaur] (local) : " tracker; tracker="${tracker:-local}"
    key=""
    if [[ "$tracker" != "local" ]]; then
      read -r -p "Clé d'équipe dans $tracker (ex. ENG, YAK) : " key
    else
      read -r -p "Préfixe des tickets (vide : le nom du dossier) : " key
    fi
    "$cairn_bin" add "$repo" --tracker "$tracker" ${key:+--key "$key"} || echo "! projet non ajouté : relance « cairn add » dans le dépôt"
  else
    echo "• plus tard : « cairn add » dans un dépôt (--tracker linear|sprintosaur --key ENG pour un tracker)"
  fi
fi

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

# A cairn already open in herdr keeps running the old binary: restart it.
if command -v herdr >/dev/null && pkill -fx '(.*/)?cairn' 2>/dev/null; then
  sleep 0.5
  "$cairn_bin" ensure && echo "✓ cairn relancé dans herdr avec la nouvelle version"
fi

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
