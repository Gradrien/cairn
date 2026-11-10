---
name: cairn
description: Keep a ticket's chantier file (~/.cairn/chantiers/<TICKET>.md) true while you plan, build and review it, so the human sees where the work stands in cairn and herdr's sidebar. Use whenever you work on a ticket (YAK-123, ENG-42, EIDO-7…: Linear, Sprintosaur or a cairn-only ticket) and that file exists, or when asked to plan a ticket "avec le workflow cairn".
---

# cairn

Le fichier du chantier sert à afficher où en est le travail : ce que l'humain lit dans cairn, c'est ce fichier. Il doit rester **court et vrai**. Ce n'est pas un rapport.

## Le fichier

`~/.cairn/chantiers/<TICKET>.md`, par exemple `~/.cairn/chantiers/YAK-788.md`. La clé dépend du projet : celle du tracker
(Linear, Sprintosaur), ou celle que cairn a donnée à un ticket créé sans tracker (`EIDO-7`) :

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

## Comment ça marche
La largeur se règle à la souris et n'est sauvée nulle part : elle vit en mémoire jusqu'au rechargement.

### Largeur d'une colonne
- **Ce qu'on voit** : glisser le bord d'un en-tête, double-clic pour revenir à la largeur automatique.
- **Où** : hook `usePointerResize` (geste), `useColumnWidths` dans `AdvDataTable` (état).
- **Quand** : pendant le geste, seule une variable CSS change ; l'état React est écrit au relâchement.
- **Données** : état React local, ni URL, ni localStorage, ni serveur. Perdu au changement d'onglet.
- **Pas fait** : la mémoire des largeurs (YAK-851).

## Tester à la main
Lance `pnpm dev`, connecte-toi en admin, ouvre /freelances.

- [ ] Glisse le bord droit de l'en-tête « Nom » vers la droite → la colonne s'élargit, les autres ne bougent pas.
- [ ] Double-clique sur ce bord → la colonne revient à sa largeur d'avant.
- [ ] Recharge la page → toutes les colonnes sont revenues à leur largeur d'origine (rien n'est sauvé, c'est voulu).

## Preuves
- `pnpm test --filter yak-query` ✓ 42 tests (2026-11-03 14:20)
- capture : /chemin/vers/capture.png
```

cairn affiche tout ce fichier :
- le paragraphe avant le premier `##` devient l'**objectif** ;
- `- [~]` marque la tâche **en cours** : il n'y en a qu'une à la fois, et c'est celle sur laquelle tu travailles maintenant ;
- sous `## Questions`, les questions sans `[x]` sont affichées comme **ouvertes** ;
- `## Effets de bord` est suivi à part (« 2/3 vérifiés ») ;
- sous `## Preuves`, chaque élément de liste est une preuve ;
- `## Comment ça marche` s'ouvre en plein écran (`i`) : c'est là que l'humain lit comment le ticket est construit.
- `## Tester à la main` s'affiche comme une liste d'étapes que l'humain coche dans cairn au fur et à mesure.
- Les clés et titres restent en français même si tu écris le plan en anglais. cairn accepte aussi `waiting:`,
  `## Side effects` et `## Proofs`.

L'avancement du code (commits, fichiers) et de la PR (CI, review) est lu directement dans git et GitHub : ne le recopie pas.

## Les règles

- **La taille suit le ticket.** XS ou S : quelques tâches, pas de prose. L ou XL : un plan plus détaillé, mais toujours en tâches vérifiables. Pas d'autre fichier de plan.
- **Les effets de bord font partie du plan, toujours.** Avant de valider, cherche ce que le changement peut casser ou modifier ailleurs :
  - les autres appelants et écrans du code touché ;
  - les autres parties du dépôt (autres apps d'un monorepo, paquets partagés) ;
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
- **Jamais de push ni de PR sans feu vert humain, et deux feux verts distincts.** Ne lance pas `git push`,
  `gh pr create` ni `gh pr ready` sans autorisation, et **jamais** `gh pr merge` : le merge est à l'humain. Un hook
  bloque ce qui n'est pas autorisé : si tu le rencontres, ne le contourne pas, demande.
  - **Le push.** Quand le travail est prêt à partir sur sa branche, mets `attente: Prêt à pousser — <ce qui partira, en
    une ligne>`, lance `cairn sync` et arrête-toi. L'humain clique sur « Autoriser le push » (cairn écrit
    `push: autorisé <commit>`), ou te demande simplement de pousser dans le chat (« push », « pousse ») : un hook l'écrit
    alors pour toi, sans commit, et ton premier push le fixe. L'autorisation vaut pour **ce commit** et ses nouvelles
    tentatives ; un nouveau commit redemande. **Pousser n'ouvre pas la PR.**
  - **La PR.** Ne l'ouvre que si le plan porte `pr: autorisée`. Quand tu juges le travail prêt pour la relecture, mets
    `attente: Prêt à ouvrir la PR — <ce qu'elle contient>`, lance `cairn sync` et arrête-toi : l'humain clique sur
    « Ouvrir la PR » et te relance. Ouvre-la alors, lie-la au ticket et passe en `phase: review`.
  - **Une fois la PR ouverte**, tes pushes suivants (correctifs de CI, retours de review) passent sans redemander : la
    relecture se fait sur la PR.
- **Images dans une PR : jamais de chemin relatif.** `![avant](./before.png)` est résolu par GitHub en `/pull/before.png`, c'est un lien mort. N'utilise que des URLs d'images réellement hébergées, par exemple des pièces jointes GitHub (`https://github.com/user-attachments/assets/…`) téléversées via le navigateur, puis vérifie qu'elles s'affichent. Si tu ne peux pas téléverser, n'insère pas d'image : garde les captures en local et cite leur chemin absolu sous `## Preuves`, cairn les affiche.
- **`attente:`** n'apparaît que quand une décision humaine bloque. Écris-la en quelques mots (« Choisir entre migration et script »), pose la question dans ta réponse, puis attends. Retire la ligne une fois la réponse obtenue.
- **`## Comment ça marche` explique le code à l'humain.** Il doit pouvoir répondre seul aux questions sur le ticket
  (« où est sauvée la taille ? », « quand tourne la synchro ? ») sans lire le diff. Écris-la pendant le build, dès
  qu'un mécanisme existe, et garde-la vraie jusqu'à la PR (après un retour de review aussi). Le format :
  - 2 ou 3 lignes sur le parcours d'ensemble ;
  - un `###` par mécanisme, avec les puces qui s'appliquent : **Ce qu'on voit**, **Où** (fichiers, fonctions, hooks,
    composants, en `code`), **Quand** (ce qui déclenche : montage, clic, sauvegarde, job, webhook…), **Données** (où vit
    l'état, persisté ou non, envoyé au serveur ou non), **Pas fait / limites** ;
  - des faits vérifiés dans le code, pas des intentions ni la liste des fichiers touchés.

  La taille suit le ticket : un XS tient en 2 lignes. Quand l'humain pose une question depuis cairn, vérifie dans le
  code, complète le mécanisme concerné et note la question avec sa réponse courte sous `### Questions posées`.
- **`## Tester à la main` permet à l'humain d'essayer le changement lui-même.** Écris-la pendant le build dès que le
  changement se voit ou se déclenche à la main (une page, une commande, une API qu'on peut appeler), et garde-la vraie
  jusqu'à la PR. Le format :
  - d'abord, en lignes simples, ce qu'il faut pour commencer : la commande à lancer, la page à ouvrir, le compte ou la
    donnée à avoir ;
  - puis une case `- [ ]` par étape : **une action, une flèche `→`, ce qu'on doit voir**. « Clique sur Exporter → un
    fichier `freelances.csv` se télécharge, avec une colonne Modèle. » Pas « vérifier que l'export fonctionne » ;
  - des mots simples et précis : le nom exact du bouton, de la page, de la commande, la valeur attendue. Pas de jargon,
    pas de formules du type « s'assurer de la bonne prise en compte » ;
  - le cas normal d'abord, puis les cas limites qui comptent (vide, erreur, droits) et ce qui ne doit **pas** changer ;
  - 3 à 8 étapes en général. L'humain coche les étapes qui marchent : ne coche jamais à sa place, ne décoche pas les
    siennes.

  Si le changement ne peut pas s'essayer à la main (refacto interne, script de migration sans effet visible), écris
  juste une ligne sous le titre qui dit pourquoi et ce qui le couvre à la place (les tests, la CI).
- **Les preuves** sont des choses réellement exécutées : la commande, son résultat et la date. Jamais une affirmation sans exécution.
- **Après chaque modification du fichier**, lance `cairn sync` pour mettre à jour la barre latérale de herdr. S'il échoue, ce n'est pas grave : continue.
- **Le ticket vit là où le projet le range.** Avec Linear ou Sprintosaur, ses règles s'appliquent toujours (PR liée au
  ticket, commentaire de fin) : lis-le avec l'outil du tracker (MCP Linear, skill sprintosaur). Sans tracker, le ticket
  n'est que ce fichier : son titre dit tout, ce qui manque se demande sous `## Questions`. Dans tous les cas, mets la clé
  du ticket dans le nom de la branche et dans le titre de la PR : c'est par la branche que cairn retrouve la PR.
