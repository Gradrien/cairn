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
# The clone's path lets cairn update itself from it (U), and its commit tells
# whether main has moved on. Go's own stamp counts untracked files as local
# edits: only edits to tracked files leave the commit out.
rev=
[ -n "$(git status --porcelain --untracked-files=no)" ] || rev=$(git rev-parse HEAD)
go install -ldflags "-X 'main.sourceDir=$(pwd -P)' -X 'main.buildRev=$rev'" .
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

# Claude Code hooks: no push / PR on a cairn chantier without a human's go-ahead,
# given in cairn or, for a push, asked in the chat.
python3 - "$cairn_bin" <<'PY'
import json, os, sys
path = os.path.expanduser("~/.claude/settings.json")
d = json.load(open(path)) if os.path.exists(path) else {}
hooks = d.setdefault("hooks", {})
changed = False
for event, suffix, entry in [
    ("PreToolUse", "cairn guard", {"matcher": "Bash"}),
    ("UserPromptSubmit", "cairn guard prompt", {}),
]:
    lst = hooks.setdefault(event, [])
    if any(h.get("command", "").endswith(suffix) for e in lst for h in e.get("hooks", [])):
        print(f"✓ hook Claude Code « {suffix} » déjà présent")
        continue
    lst.append({**entry, "hooks": [{"type": "command", "command": sys.argv[1] + suffix[len("cairn"):], "timeout": 10}]})
    changed = True
    print(f"✓ hook Claude Code « {suffix} » ajouté dans ~/.claude/settings.json")
if changed:
    json.dump(d, open(path, "w"), indent=2, ensure_ascii=False)
PY

# A cairn already open in herdr keeps running the old binary: restart it. The
# cairn updating itself (U) is left alone: it starts the new build on its own.
if command -v herdr >/dev/null; then
  others=$(pgrep -fx '(.*/)?cairn' | grep -vx "${CAIRN_UPDATING_PID:-0}" || true)
  if [[ -n "$others" ]]; then
    kill $others 2>/dev/null || true
    sleep 0.5
    "$cairn_bin" ensure && echo "✓ cairn relancé dans herdr avec la nouvelle version"
  fi
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
