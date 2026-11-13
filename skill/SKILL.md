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

<1 ou 2 lignes : l'objectif>

## Approche
- **Problème** : les admins ne voient pas le modèle d'un freelance dans l'export, ils le recopient à la main.
- **Fini quand** :
  - l'export a une colonne « Modèle » avec le libellé ;
  - la cellule est vide si le modèle manque ;
  - l'import CSV marche toujours.
- **Existant** : `exportFreelances` (`lib/export/freelances.ts:42`) construit les lignes ; `formatModel` (`lib/labels.ts:12`) donne déjà le libellé sur la fiche.
- **Choix** : ajouter le champ au select et une colonne au mapper, avec `formatModel`.
- **Écarté** : le libellé calculé en SQL, qui dupliquerait `formatModel`.
- **Risque** : aucun ; l'import ignore les colonnes inconnues (`lib/import/csv.ts:30`), vérifié.
- **Hors périmètre** : l'export des missions (YAK-905).

- [x] une tâche faite
- [~] la tâche en cours
- [ ] une tâche vérifiable à faire

## Questions
- Une question ouverte qui attend une réponse humaine ?
- **Ciblage** : que fait-on pour un deal sans métier ?
  - (a) Garder le ciblage actuel : le compteur affiche 0.
  - (b) Élargir l'envoi aux expertises de la spé : change aussi le premier envoi.
  - Recommandation : (a), parce qu'il ne change pas qui reçoit l'email.
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
- capture : ~/.cairn/proofs/YAK-12/apres-redimension.png
```

cairn affiche tout ce fichier :
- le paragraphe avant le premier `##` devient l'**objectif** ;
- `## Approche` garde ce que le plan a établi (voir « Construire le plan ») : des puces simples, sans case à cocher,
  sinon elles seraient comptées comme des tâches. Un point qui a plusieurs éléments (les critères de **Fini quand**,
  les fichiers de **Existant**) les met en sous-puces indentées, un par ligne, jamais en un paragraphe numéroté :
  l'humain lit l'approche point par point ;
- `- [~]` marque la tâche **en cours** : il n'y en a qu'une à la fois, et c'est celle sur laquelle tu travailles maintenant ;
- sous `## Questions`, les questions sans `[x]` sont affichées comme **ouvertes**. Une question à choix
  s'affiche comme dans ton chat : l'humain sélectionne une option, ou répond avec ses mots. Pour ça :
  - une puce par question, sans case `[ ]` tant qu'elle est ouverte, et le contexte dans son texte ou dans des
    sous-puces sans clé ;
  - les choix en sous-puces `(a)`, `(b)`, `(c)`… : un choix par sous-puce, avec sa conséquence après « : ». Ne
    mets pas une option en puce de premier niveau, ce serait une autre question ;
  - puis une sous-puce `Recommandation : (x), parce que…`. cairn la met en avant ;
  - une fois tranchée : `- [x] La question → la réponse retenue`.
- ce que tu écris sous un élément (sous-puces, paragraphe indenté, bloc de code) s'affiche sous lui, à toutes les
  sections : une tâche peut détailler ses étapes ou ses cas d'erreur en sous-puces ;
- `## Effets de bord` est suivi à part (« 2/3 vérifiés ») ;
- sous `## Preuves`, chaque élément de liste est une preuve. Une capture y est un chemin absolu vers un fichier
  image, et cairn la montre en vignette. Enregistre tes captures dans `~/.cairn/proofs/<TICKET>/` (crée le dossier),
  jamais dans un dossier temporaire ou le scratchpad de ta session : ils sont effacés, et l'humain ne verrait plus
  qu'une capture introuvable ;
- `## Comment ça marche` s'ouvre en plein écran (`i`) : c'est là que l'humain lit comment le ticket est construit.
- `## Tester à la main` s'affiche comme une liste d'étapes que l'humain coche dans cairn au fur et à mesure. Quand
  une étape ne marche pas, cairn t'envoie l'étape et ce que l'humain voit à la place : corrige le code (ou l'étape si
  c'est elle qui est fausse) et laisse-la décochée, il la refera.
- Les clés et titres restent en français même si tu écris le plan en anglais. cairn accepte aussi `waiting:`,
  `## Side effects` et `## Proofs`.

L'avancement du code (commits, fichiers) et de la PR (CI, review) est lu directement dans git et GitHub : ne le recopie pas.

## Construire le plan

Le plan décide de la façon dont le ticket sera construit, et c'est un **agent neuf** qui le construira, avec ce seul
fichier : ce que tu as compris en explorant et que tu n'écris pas est perdu à la passation. Suis ces étapes dans
l'ordre. Leur profondeur suit la taille du ticket : pour un XS, chacune tient en une ligne ; un L ou un XL les déroule
toutes. Ce qu'elles établissent va sous `## Approche`, les tâches en dessous.

1. **Comprendre le besoin.** Lis le ticket et ses commentaires. Écris le problème en une phrase : qui est gêné, par
   quoi (**Problème**). Puis les critères d'acceptation : ce qui sera vrai et vérifiable quand le ticket sera fini,
   cas limites compris (vide, erreur, droits) et ce qui ne doit pas changer (**Fini quand**). Ce que le ticket ne
   tranche pas devient tout de suite une question sous `## Questions` ; continue sur le reste en notant l'hypothèse
   que tu fais.
2. **Adapter la démarche au type de ticket.**
   - **Bug** : reproduis-le (commande, test ou étapes), trouve la cause et pas seulement l'endroit où elle se voit,
     et prévois un test qui échoue avant le correctif.
   - **Feature** : suis le parcours de l'utilisateur, de l'écran ou de l'appel jusqu'aux données.
   - **Refacto** : liste les comportements qui ne doivent pas bouger et ce qui le garantit (des tests existants, ou à
     écrire d'abord).
3. **Partir de l'existant.** Avant d'imaginer une solution, regarde comment le code fait aujourd'hui, ce qui existe
   déjà de semblable dans le dépôt (pattern, composant, helper, endpoint) et quels tests couvrent la zone
   (**Existant**, avec des `fichier:ligne`). Confie ces recherches à `cairn-explorer`, plusieurs en parallèle : un pour
   le code touché, un pour les cas semblables, un pour les tests. La bonne solution ressemble le plus souvent à ce qui
   existe déjà.
4. **Choisir l'approche.** S'il y a plus d'une façon de faire, compare-les en une ligne chacune et garde la plus simple
   qui remplit les critères (**Choix**, **Écarté** avec la raison). Si le choix engage le produit ou l'architecture
   (comportement visible, modèle de données, nouvelle dépendance), ne tranche pas seul : pose la question sous
   `## Questions`, avec les options en `(a)`, `(b)`…, la conséquence de chacune et ta recommandation (voir « Le
   fichier »). Écris ce que tu ne fais pas (**Hors périmètre**).
5. **Lever le risque d'abord.** Repère ce qui peut faire échouer le plan : une API ou une bibliothèque mal connue, un
   volume de données, un comportement que tu supposes sans l'avoir vu (**Risque**). Vérifie-le pendant le plan (lis le
   code, lance une commande courte). Si ce n'est pas possible, fais-en la première tâche, en disant ce qu'on fait si
   l'hypothèse tombe.
6. **Recenser les effets de bord**, toujours : ce que le changement peut casser ou modifier ailleurs.
   - les autres appelants et écrans du code touché ;
   - les autres parties du dépôt (autres apps d'un monorepo, paquets partagés) ;
   - les données existantes et leur rattrapage ;
   - la compatibilité avec ce qui tourne déjà (clients, jobs, liens, formats de fichier) ;
   - les permissions ;
   - les migrations, et comment revenir en arrière si le changement casse en production ;
   - les notifications et les jobs ;
   - les performances.

   Chaque effet de bord devient une tâche qui le vérifie, sous `## Effets de bord`. Si tu n'en trouves aucun, écris
   pourquoi en une ligne.
7. **Découper en tâches.** Ordonne-les selon leurs dépendances, la plus risquée d'abord. Chaque tâche dit **où** elle
   agit (fichier ou fonction, en `code`) et **comment on sait qu'elle est finie** (un test, une commande, un
   comportement), et laisse le code dans un état qui compile et passe les tests. Préfère des tranches qui traversent le
   ticket de bout en bout à des couches (« tout le back, puis tout le front »). Les tests sont des tâches comme les
   autres : lesquels ajouter ou modifier, à quel niveau (unitaire, intégration), et la commande qui servira de preuve.
   « Gérer le cas vide » ne suffit pas ; « `exportFreelances` : 0 freelance → fichier avec les seuls en-têtes, test
   unitaire » suffit.
8. **Relire le plan comme le constructeur le lira.** Avant de t'arrêter, vérifie que :
   - chaque critère de **Fini quand** est couvert par une tâche ;
   - un agent qui n'a que ce fichier peut construire sans refaire l'exploration ni choisir à ta place ;
   - chaque nom propre au projet est expliqué ou pointé par un `fichier:ligne` ;
   - les questions ouvertes sont celles qui bloquent vraiment, pas des points que tu peux vérifier toi-même.

   Puis mets `attente: Valider le plan`, lance `cairn sync` et arrête-toi, sans coder.

## Les règles

- **La taille suit le ticket.** XS ou S : une `## Approche` de quelques lignes et quelques tâches. L ou XL : un plan plus détaillé, mais toujours en tâches vérifiables. Pas d'autre fichier de plan.
- **Les effets de bord font partie du plan, toujours** (voir « Construire le plan », étape 6). L'humain peut cocher
  un effet de bord dans cairn quand il a vérifié le comportement lui-même : ne décoche jamais une case qu'il a cochée.
- **Les phases** :
  - `plan` : tu suis « Construire le plan ». Tu termines par `attente: Valider le plan`, puis tu **t'arrêtes**, sans coder.
  - `build` : l'humain a validé. Passe une tâche en `[~]` quand tu la commences, puis en `[x]` dès qu'elle est finie. Ajoute le travail découvert en cours de route et retire ce qui s'est révélé inutile. Si tu changes d'approche, corrige `## Approche` et dis pourquoi en une ligne.
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
- **Images dans une PR : jamais de chemin relatif.** `![avant](./before.png)` est résolu par GitHub en `/pull/before.png`, c'est un lien mort. N'utilise que des URLs d'images réellement hébergées, par exemple des pièces jointes GitHub (`https://github.com/user-attachments/assets/…`) téléversées via le navigateur, puis vérifie qu'elles s'affichent. Si tu ne peux pas téléverser, n'insère pas d'image : garde les captures dans `~/.cairn/proofs/<TICKET>/` et cite leur chemin sous `## Preuves`, cairn les affiche.
- **`attente:`** n'apparaît que quand une décision humaine bloque. Écris-la en quelques mots (« Choisir entre migration et script »), pose la question dans ta réponse, puis attends. Retire la ligne une fois la réponse obtenue.
- **`## Comment ça marche` explique le code à l'humain.** Il doit pouvoir répondre seul aux questions sur le ticket
  (« où est sauvée la taille ? », « quand tourne la synchro ? ») sans lire le diff. Écris-la pendant le build, dès
  qu'un mécanisme existe, et garde-la vraie jusqu'à la PR (après un retour de review aussi). Le format :
  - 2 ou 3 lignes sur le parcours d'ensemble ;
  - un `###` par mécanisme, avec les puces qui s'appliquent : **Ce qu'on voit**, **Où** (fichiers, fonctions, hooks,
    composants, en `code`), **Quand** (ce qui déclenche : montage, clic, sauvegarde, job, webhook…), **Données** (où vit
    l'état, persisté ou non, envoyé au serveur ou non), **Pas fait / limites** ;
  - des faits vérifiés dans le code, pas des intentions ni la liste des fichiers touchés.

  La taille suit le ticket : un XS tient en 2 lignes. Les questions que l'humain pose depuis cairn sont traitées
  par un agent à part, qui lit le code ; cairn écrit sa réponse sous `### Questions posées`. Garde ces entrées, et
  corrige-les si ton travail les rend fausses.
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
- **Après chaque modification du fichier**, lance `cairn sync` (depuis le worktree du ticket, ou `cairn sync <TICKET>`).
  Il met à jour la barre latérale de herdr et **vérifie le plan** : s'il liste des points (« ligne 134 : question
  « … » : pas de recommandation… »), c'est que cairn ne peut pas afficher le fichier comme tu l'as voulu. Corrige-les
  et relance-le. S'il échoue pour une autre raison (herdr absent), ce n'est pas grave : continue.
- **Le ticket vit là où le projet le range.** Avec Linear ou Sprintosaur, ses règles s'appliquent toujours (PR liée au
  ticket, commentaire de fin) : lis-le avec l'outil du tracker (MCP Linear, skill sprintosaur). Sans tracker, le ticket
  n'est que ce fichier : son titre dit tout, ce qui manque se demande sous `## Questions`. Dans tous les cas, mets la clé
  du ticket dans le nom de la branche et dans le titre de la PR : c'est par la branche que cairn retrouve la PR.

## Déléguer pour garder ton contexte petit

Tout ce que tu lis reste dans ton contexte et **est relu à chaque tour** jusqu'à la fin du ticket : c'est ce
qui coûte le plus cher sur un chantier, bien plus que ce que tu écris. Deux sous-agents, sur un modèle léger,
lisent à ta place et ne te rendent que l'essentiel :

- **`cairn-explorer`** (lecture seule) : toute recherche qui demande de parcourir plus de trois ou quatre
  fichiers. Au plan, c'est lui qui recense les effets de bord et les appelants ; pendant le build, il répond à
  « où est… », « qui appelle… », « comment marche… ». Pose-lui une question précise, avec ce que tu sais déjà et
  ce que tu cherches ; il rend des `fichier:ligne`. Lance-en plusieurs en parallèle pour des questions
  indépendantes (un par app, un par type d'effet de bord). Ensuite, ne lis toi-même que les lignes que tu
  vas modifier.
- **`cairn-runner`** : les tests, le build, le lint, le typecheck, et les logs d'une CI en échec, dès que la
  sortie peut être longue. Donne-lui la commande et le dossier (ou le fichier de logs). Il rend le verdict,
  les échecs avec `fichier:ligne` et une ligne `Preuve` à reporter telle quelle sous `## Preuves`. Il ne
  corrige rien : c'est toi qui corriges, puis tu le relances.

**Ne délègue pas** ce qui demande ton jugement ou ce que tu as déjà en tête : le plan, les questions, les
décisions, le code, `## Comment ça marche` et `## Tester à la main`. Ne délègue pas non plus ce qui est plus
court à faire qu'à demander : lire un fichier que tu connais, `git status`, une commande à la sortie courte.
Si ces sous-agents ne sont pas installés, fais le travail toi-même, en lisant le moins possible.

## Passer la main à un agent neuf

Un chantier change d'agent pour alléger le contexte : à la validation du plan (un agent neuf construit), et quand
l'humain clique sur « Repartir à neuf » en cours de route. Le plan est la mémoire du chantier, git garde le code.

- **Quand on te demande ta reprise**, mets le plan à jour, puis écris une section `## Reprise` de 5 à 15 lignes pour
  ton successeur : où tu en es exactement, ce qui est en cours, ce que tu as essayé sans succès et pourquoi, les pièges
  du code, la prochaine étape. Pas ce qui est déjà dans le plan ou dans git. Lance `cairn sync` et arrête-toi.
- **Quand tu reprends un chantier**, lis le plan en entier (la reprise d'abord), puis `git log`, `git diff --stat`
  depuis la base et `git status`. Ne refais pas l'exploration : vérifie un point précis avec `cairn-explorer`, et
  continue là où ton prédécesseur s'est arrêté. Laisse la section `## Reprise` : la prochaine passation la réécrira.
