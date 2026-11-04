---
name: cairn
description: Keep a ticket's chantier file (~/.cairn/chantiers/<TICKET>.md) true while you plan, build and review it, so the human sees where the work stands in cairn and herdr's sidebar. Use whenever you work on a Sprintosaur ticket (YAK-123…) and that file exists, or when asked to plan a ticket "avec le workflow cairn".
---

# cairn

Le fichier du chantier sert à afficher où en est le travail : ce que l'humain lit dans cairn, c'est ce fichier. Il doit rester **court et vrai**. Ce n'est pas un rapport.

## Le fichier

`~/.cairn/chantiers/<TICKET>.md`, par exemple `~/.cairn/chantiers/YAK-788.md` :

```markdown
---
ticket: YAK-788
phase: plan            # plan | build | review
attente: Valider le plan   # seulement quand tu as besoin d'une décision humaine
---
# YAK-788 · <titre>

<2 à 5 lignes : l'objectif et les décisions prises>

- [x] une tâche faite
- [~] la tâche en cours
- [ ] une tâche vérifiable à faire

## Questions
- Une question qui attend une réponse humaine ?
- [x] Une question tranchée → la réponse retenue

## Effets de bord
- [ ] l'export CSV lit aussi `freelance.model` : vérifier qu'il ne change pas

## Preuves
- `pnpm test --filter yak-query` ✓ 42 tests (2026-11-03 14:20)
- capture : /chemin/vers/capture.png
```

cairn affiche tout ce fichier :
- le paragraphe avant le premier `##` devient l'**objectif** ;
- `- [~]` marque la tâche **en cours** : il n'y en a qu'une à la fois, et c'est celle sur laquelle tu travailles maintenant ;
- sous `## Questions`, les questions sans `[x]` sont affichées comme **ouvertes** ;
- `## Effets de bord` est suivi à part (« 2/3 vérifiés ») ;
- sous `## Preuves`, chaque élément de liste est une preuve.
- Les clés et titres restent en français même si tu écris le plan en anglais. cairn accepte aussi `waiting:`,
  `## Side effects` et `## Proofs`.

L'avancement du code (commits, fichiers) et de la PR (CI, review) est lu directement dans git et GitHub : ne le recopie pas.

## Les règles

- **La taille suit le ticket.** XS ou S : quelques tâches, pas de prose. L ou XL : un plan plus détaillé, mais toujours en tâches vérifiables. Pas d'autre fichier de plan.
- **Les effets de bord font partie du plan, toujours.** Avant de valider, cherche ce que le changement peut casser ou modifier ailleurs :
  - les autres appelants et écrans du code touché ;
  - les autres apps du monorepo ;
  - les données existantes et leur rattrapage ;
  - les permissions ;
  - les migrations ;
  - les notifications et les jobs ;
  - les performances.

  Chaque effet de bord devient une tâche qui le vérifie, sous `## Effets de bord`. Si tu n'en trouves aucun, écris pourquoi en une ligne.
- **Les phases** :
  - `plan` : tu explores et tu écris les tâches. Tu termines par `attente: Valider le plan`, puis tu **t'arrêtes**, sans coder.
  - `build` : l'humain a validé. Passe une tâche en `[~]` quand tu la commences, puis en `[x]` dès qu'elle est finie. Ajoute le travail découvert en cours de route et retire ce qui s'est révélé inutile.
  - `review` : la PR est ouverte. Si on te demande des changements, fais-les en local, puis redemande le push comme ci-dessous.
  - « Livré » est détecté seul quand la PR est mergée : ne l'écris pas.
- **Jamais de push ni de PR sans feu vert humain.** Ne lance pas `git push`, `gh pr create`, `gh pr ready` ni `gh pr merge` de toi-même. Quand le travail est prêt, dans les deux cas suivants :
  - avant la première PR ;
  - avant chaque nouvelle série de changements sur une PR existante.

  Mets `attente: Prêt à pousser — <ce qui partira, en une ligne>`, lance `cairn sync` et arrête-toi. L'humain relit le diff et clique sur « Autoriser le push » : cairn écrit alors `push: autorisé` et te relance. Une autorisation vaut pour **un seul** push. Un hook bloque tout push non autorisé : si tu le rencontres, ne le contourne pas, demande l'autorisation. Le but est de ne jamais envoyer de travail inachevé dans la pipeline de la PR.
- **Images dans une PR : jamais de chemin relatif.** `![avant](./before.png)` est résolu par GitHub en `/pull/before.png`, c'est un lien mort. N'utilise que des URLs d'images réellement hébergées, par exemple des pièces jointes GitHub (`https://github.com/user-attachments/assets/…`) téléversées via le navigateur, puis vérifie qu'elles s'affichent. Si tu ne peux pas téléverser, n'insère pas d'image : garde les captures en local et cite leur chemin absolu sous `## Preuves`, cairn les affiche.
- **`attente:`** n'apparaît que quand une décision humaine bloque. Écris-la en quelques mots (« Choisir entre migration et script »), pose la question dans ta réponse, puis attends. Retire la ligne une fois la réponse obtenue.
- **Les preuves** sont des choses réellement exécutées : la commande, son résultat et la date. Jamais une affirmation sans exécution.
- **Après chaque modification du fichier**, lance `cairn sync` pour mettre à jour la barre latérale de herdr. S'il échoue, ce n'est pas grave : continue.
- Les règles du skill sprintosaur (PR liée au ticket, commentaire de fin) s'appliquent toujours.
