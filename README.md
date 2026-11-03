# cairn

Où en est chaque chantier, à côté de [herdr](https://herdr.dev).

herdr te dit quel agent travaille, attend ou dort. cairn te dit **où en est le travail** : la phase du ticket,
les tâches faites, les preuves, la PR, et ce qui attend ta décision. Une seule commande à retenir, tout le reste
est au clavier, affiché à l'écran.

```
cairn  YAK · adrien · cycle 86
Attend ta décision                       │ YAK-900  Export PDF des missions
⚠ YAK-900 Export PDF des missions        │ Sprintosaur  In Progress · High · M
    Plan 0/5 · ◐                         │ Phase   Plan ● → Build → Review → Livré
Chantiers                                │ Attente ⚠ Valider le plan  (v valider · enter voir l'agent)
  YAK-788 Supprimer « Définir comme… »   │ Tâches 0/5
    Review · ○ · PR #3323                │   ○ …
```

## Dans herdr

cairn vit dans **son propre espace herdr**, nommé `cairn`, en plein écran. Tu gardes herdr tel quel (barre latérale, panes,
agents) :
- **un clic sur `cairn` dans la barre latérale** l'ouvre. Sous son nom, la barre latérale indique « ⚠ N attendent ta décision » ;
- `prefix+a` bascule entre cairn et l'espace d'où tu viens. Dans cairn, `q` te ramène aussi à cet espace ;
- `enter` sur un chantier t'emmène dans l'espace de son agent.

Le plugin herdr (`herdr-plugin/`) recrée l'espace et relance cairn à chaque démarrage de herdr : le bouton ne disparaît pas.

## Un chantier

C'est un ticket Sprintosaur, du cycle en cours et assigné à toi, ou dont le nom de branche contient la clé. On lui associe :
- son **worktree**, s'il en a un (sa branche contient `yak-123`) ;
- ses **agents herdr**, ceux qui travaillent dans ce worktree, ou dont le nom ou le titre cite le ticket ;
- son **fichier** `~/.cairn/chantiers/YAK-123.md` : phase, tâches, preuves, attente. Ce sont les agents qui le
  tiennent à jour, via le skill `cairn` (`skill/SKILL.md`).

Les phases sont **Plan → Build → Review → Livré**. « Livré » est détecté quand les PR du ticket sont mergées.

## Commentaires et images de PR

Le bouton **Commentaires** (`c`, ou un clic sur « 💬 Commentaires et images » dans le détail) ouvre la conversation de la
PR en plein écran : la description, les commentaires, les reviews, et les commentaires inline avec leur `fichier:ligne`.
Le HTML des bots est nettoyé et leurs longs messages sont repliés (bouton **Bots**).

Les images sont marquées `🖼 image N`. Un clic sur l'une d'elles, ou **Images** (`i`) pour les voir toutes, les
télécharge et les affiche **dans le terminal**, via le protocole d'images de Kitty que herdr transmet à Ghostty, Kitty
ou WezTerm :
- `←` `→` pour passer d'une image à l'autre ;
- `o` pour ouvrir l'image dans Aperçu ;
- `q` pour revenir.

Les pièces jointes GitHub privées passent par le token `gh`, les liens signés (Linear…) sont téléchargés tels quels.
Tout est mis en cache dans `~/.cairn/cache/images`. Les captures citées sous `## Preuves` d'un plan sont cliquables de
la même façon.

## Souris et clavier

Tout se fait à la souris :
- un clic sur un chantier le sélectionne, un second clic t'emmène à son agent ;
- la molette fait défiler la liste ;
- un clic sur un agent dans le détail t'amène dans son pane ;
- les actions sont des boutons en bas de l'écran, grisés quand ils ne s'appliquent pas au chantier sélectionné.

Chaque bouton a aussi sa touche :

## Touches

| Touche | Action |
|---|---|
| `enter` | aller à l'agent du chantier (celui qui attend d'abord) |
| `d` | browsediff de la branche (`origin/main...HEAD`) |
| `p` | planifier le ticket : crée le worktree et l'espace herdr, lance Claude et lui demande le plan |
| `v` | valider ce qui attend : plan validé → Build, et l'agent est relancé |
| `o` | ouvrir le ticket dans Sprintosaur |
| `r` | rafraîchir · `q` retour à herdr (quitter, hors de herdr) |

## Installer

```sh
./install.sh            # vet, test, go install, lien du skill dans ~/.claude/skills/cairn
./install.sh --herdr    # en plus : plugin herdr (bouton cairn dans la barre latérale), prefix+a, phase dans la barre latérale
```

Config : `~/.cairn/config.json`, écrit au premier lancement (dépôt, équipe, handle Sprintosaur, base, type d'agent).
Les identifiants Sprintosaur sont lus depuis le skill sprintosaur.
