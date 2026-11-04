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

## Boucler avec les agents

- **Retour de review → agent** : dans la vue Commentaires, `[→ agent]` en bout d'en-tête envoie un commentaire (review,
  inline avec son `fichier:ligne`, bot) à l'agent du chantier. L'agent l'ajoute comme tâche, corrige en local, puis
  demande le push.
- **Corriger la CI** : quand un check échoue, le bouton `f` (ou le lien dans le détail) récupère les logs des étapes
  en échec (`gh run view --log-failed`) dans `~/.cairn/cache/ci/` et les passe à l'agent.
- **Rattacher un agent** : un agent « hors chantier » (par exemple dans le dépôt principal) se rattache d'un clic à un
  chantier. cairn le renomme d'après le ticket, ce qui le place aussi au bon endroit dans herdr.

## Suivi

- **Prêt à merger** : pour une PR ouverte, cairn vérifie les tâches, les effets de bord, la CI, la review, les conflits et
  le statut brouillon. La liste affiche `✓ prêt` ou `✗ CI`.
- **Historique** : chaque changement de phase et chaque décision (plan validé, push autorisé, retour envoyé…) est noté
  dans `~/.cairn/chantiers/<TICKET>.log`. La ligne Phase indique depuis quand le chantier est dans sa phase actuelle.
- **Notifications herdr** quand un chantier attend ta décision, quand la CI casse, quand une review demande des
  changements ou approuve, et quand un chantier est livré. Seule l'instance qui tourne dans l'espace `cairn` notifie.
  Les PR ouvertes sont relues toutes les 2 minutes.
- **Sprintosaur suit** : le ticket passe en In Progress quand le plan est lancé ou que le chantier entre en Build.
  Les passages en Pull Request puis en Done restent automatiques, via les PR liées.
- **Ranger** (`x`, deux clics) : une fois le chantier livré, cairn supprime son worktree et son espace herdr et archive
  son plan dans `~/.cairn/archive/`. Il refuse s'il reste des changements non commités.

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

Il te faut : Go, `gh` connecté (`gh auth login`), herdr, et le skill sprintosaur avec ses identifiants.

```sh
git clone https://github.com/Gradrien/cairn.git && cd cairn
./install.sh --herdr
```

Au premier passage, l'installeur pose deux questions : où est ton checkout de Yak-App et ton handle Sprintosaur
(proposé d'après ton email git). Il installe ensuite le binaire, le skill `cairn` pour les agents, le hook
Claude Code qui bloque les push non autorisés, et avec `--herdr` le bouton `cairn` dans la barre latérale et `prefix+a`.
Relance `./install.sh` après chaque `git pull`.

Config : `~/.cairn/config.json`. Seuls `repo` et `assignee` sont propres à chacun ; les autres réglages prennent
les valeurs de l'équipe (équipe `YAK`, base `origin/main`, branches `<handle>/yak-123-…`, agent `claude`) et
peuvent y être surchargés : `team`, `branch_prefix`, `base`, `worktree_parent`, `agent_kind`,
`sprintosaur_credentials`, `sprintosaur_web`. Les identifiants Sprintosaur viennent du skill sprintosaur
(`~/.claude/skills/sprintosaur/credentials.env`) ou des variables `SPRINTOSAUR_API` et `SPRINTOSAUR_TOKEN`.
Si `repo` est faux, cairn le dit au démarrage et indique le fichier à corriger.

Les Claude déjà ouverts avant l'installation ne voient le hook qu'après un redémarrage.
