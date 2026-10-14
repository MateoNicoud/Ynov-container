# TP 4 : intégration continue avec GitHub Actions

## Point de départ

Au TP 3, vous avez construit, scanné et poussé l'image de l'API depuis votre poste. Ça marche, mais rien ne garantit que l'image publiée correspond à un commit précis, que les tests ont tourné, ni que le scan a été lu. Le cours le dit : une image de production ne se construit pas en local.

À la fin du TP, chaque push déclenche une chaîne automatique :

```text
checkout -> tests -> build -> scan Trivy -> SBOM -> push GHCR (main et tags v*.*.* seulement)
```

et chaque étape bloque la suivante si elle échoue.

### Prérequis

- Le TP 3 : `app/Dockerfile` construit une image qui passe le scan.
- Le dépôt est sur GitHub, et vous pouvez modifier ses réglages.
- La solution est dans `.github/workflows/ci.yml` et `.github/dependabot.yml`. Renommez le dossier `.github` pour faire le TP sans la voir.

---

## Cours

### CI, livraison continue, déploiement continu

- **Intégration continue (CI)** : à chaque push, une machine neutre récupère le code, le compile et lance les tests. Le but : savoir en quelques minutes si le commit casse quelque chose, avant de le fusionner.
- **Livraison continue (continuous delivery)** : la CI produit en plus un **artefact** prêt à déployer (ici l'image Docker), versionné et stocké. Le déploiement reste une décision humaine.
- **Déploiement continu (continuous deployment)** : l'artefact part en production sans intervention, si toutes les étapes passent.

Ce TP fait de la livraison continue. Le TP 9 ajoute un déploiement sur Azure, déclenché à la main.

### Le vocabulaire de GitHub Actions

| Terme | Ce que c'est |
|---|---|
| *workflow* | un fichier YAML dans `.github/workflows/` |
| *événement* (`on:`) | ce qui déclenche le workflow : `push`, `pull_request`, `workflow_dispatch` (bouton manuel), `schedule` (cron)... |
| *job* | un ensemble d'étapes exécutées sur une même machine. Les jobs tournent en parallèle sauf si l'un dépend d'un autre (`needs:`) |
| *runner* | la machine qui exécute un job. `ubuntu-latest` est une VM neuve fournie par GitHub, détruite après le job |
| *step* | une commande shell (`run:`) ou une *action* (`uses:`) |
| *action* | un composant réutilisable publié dans un dépôt GitHub (`docker/build-push-action`) |

Chaque job démarre sur une VM vide : rien ne passe d'un job à l'autre sauf ce qu'on transmet explicitement (artefacts, cache, sorties). C'est ce qui rend le résultat reproductible : il ne dépend pas de l'état d'un poste.

### Des étapes qui servent de barrières

Une étape qui renvoie un code de sortie non nul fait échouer le job, et les étapes suivantes ne tournent pas. On place donc les contrôles **avant** la publication :

1. les tests échouent : pas d'image ;
2. le scan trouve une CVE critique corrigeable : pas de push ;
3. c'est une pull request : on construit et on scanne, mais on ne pousse rien (le code n'est pas encore validé).

Combinée à une **règle de protection de branche** (« la CI doit être verte pour fusionner dans `main` »), cette chaîne empêche de publier du code non testé.

### Les droits du workflow

GitHub fournit à chaque exécution un jeton `GITHUB_TOKEN`, valable le temps du workflow, dont les droits se règlent avec `permissions:`. Le principe de moindre privilège s'applique ici aussi : `contents: read` pour tout le workflow, et `packages: write` seulement pour le job qui pousse l'image. Une action compromise ne peut faire que ce que le jeton autorise.

Pour les identifiants externes (un registry tiers, Azure), on utilise les *secrets* du dépôt, ou mieux, une **fédération OIDC** : GitHub signe un jeton qui dit « je suis le workflow X du dépôt Y sur la branche Z », et le fournisseur cloud l'échange contre un accès temporaire. Aucun mot de passe n'est stocké (TP 9).

### Nommer les images

Une stratégie de tags qui marche bien :

| Événement | Tags produits | Usage |
|---|---|---|
| push sur `main` | `sha-1a2b3c4` | chaque commit a son image, retrouvable par son SHA |
| tag git `v1.2.3` | `1.2.3`, `1.2`, `sha-1a2b3c4` | version publiée ; `1.2` suit le dernier correctif de la 1.2 |
| pull request | `pr-42`, jamais poussé | sert au scan |

Pas de `latest` : il ne dit pas quelle version tourne, et deux déploiements « de latest » à une heure d'intervalle peuvent installer deux images différentes.

### La chaîne d'approvisionnement de la CI

Un workflow exécute du code tiers : chaque `uses:` télécharge et lance le code d'un autre dépôt, avec les droits du jeton et l'accès aux secrets. Si ce dépôt est compromis, votre CI l'est aussi.

Les 19 et 20 mars 2026, des attaquants ont redirigé les tags de `aquasecurity/trivy-action` (l'action de scan que ce TP utilise) vers des commits malveillants ([CVE-2026-33634](https://nvd.nist.gov/vuln/detail/CVE-2026-33634)). Tous les workflows qui utilisaient `@v0.x` ont exécuté ce code. Un tag git se déplace d'une commande ; un SHA de commit désigne un contenu et ne peut pas changer. D'où la règle : **épingler chaque action tierce par SHA complet**, avec la version en commentaire pour la lisibilité.

L'épinglage fige aussi les failles corrigées plus tard. Il faut donc un outil qui propose les mises à jour : **Dependabot** (intégré à GitHub) ou **Renovate** ouvrent une pull request quand une action, une image de base ou une dépendance a une nouvelle version. La PR passe par la CI comme n'importe quel changement.

---

## Partie 1 : un premier workflow de test

**Consignes.**

1. Créez `.github/workflows/ci.yml`, déclenché par les push sur `main` et par les pull requests.
2. Un job `test` sur `ubuntu-latest` : récupère le code (`actions/checkout`), puis vérifie le formatage (`gofmt -l`), lance `go vet` et `go test`. Pas besoin d'installer Go sur le runner : utilisez l'image `golang:1.27-alpine` avec `docker run`.
3. Poussez sur une branche, ouvrez une pull request, regardez l'onglet *Actions*.
4. Cassez volontairement un test dans `app/main_test.go`, poussez, constatez l'échec, réparez.

**Questions.**

1. Pourquoi tester dans la même image Go que celle du Dockerfile ?
2. `test -z "$(gofmt -l .)"` : que fait cette commande et pourquoi échoue-t-elle quand un fichier est mal formaté ?

<details>
<summary>Solution</summary>

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:          # toute PR, quelle que soit la branche cible

permissions:
  contents: read         # le jeton du workflow ne peut que lire le dépôt

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1

      - name: gofmt, vet, tests
        run: |
          docker run --rm -v "$PWD/app:/src" -w /src golang:1.27-alpine \
            sh -c 'test -z "$(gofmt -l .)" && go vet ./... && go test ./...'
```

1. Pour qu'un test vert garantisse un build vert : même version du compilateur, mêmes règles. Sinon on peut avoir des tests qui passent avec Go 1.26 sur le runner et un build qui casse avec Go 1.27 dans le Dockerfile.
2. `gofmt -l` liste les fichiers dont le formatage diffère du format standard. `test -z` réussit si la chaîne est vide. Un fichier listé rend la chaîne non vide, donc `test` renvoie 1, et l'étape échoue.

</details>

---

## Partie 2 : construire l'image et choisir ses tags

**Consignes.**

1. Ajoutez un job `image` qui ne démarre que si `test` a réussi.
2. Utilisez `docker/metadata-action` pour calculer les tags du tableau du cours (`sha-...`, semver, `pr-...`), pour l'image `ghcr.io/<owner>/tp-api`.
3. Construisez l'image avec `docker/build-push-action`, **sans la pousser**, en la chargeant dans le Docker du runner sous le nom `tp-api:scan`. Passez la version calculée en `build-args`.
4. Activez le cache de build GitHub Actions (`type=gha`) et comparez la durée de deux exécutions successives.

**Questions.**

1. Que contient `steps.meta.outputs.version` pour un push sur `main` ? Pour le tag `v1.2.3` ?
2. Sans cache, chaque build retélécharge l'image Go et les modules. Pourquoi ?

<details>
<summary>Solution</summary>

```yaml
  image:
    needs: test                         # ne démarre que si "test" a réussi
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1

      # BuildKit "complet" : nécessaire pour le cache GitHub Actions.
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1

      - id: meta
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ghcr.io/${{ github.repository_owner }}/tp-api   # passé en minuscules automatiquement
          tags: |
            type=sha,prefix=sha-
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=ref,event=pr

      - name: build (local)
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: app
          load: true                      # l'image reste sur le runner, pour le scan
          tags: tp-api:scan
          build-args: VERSION=${{ steps.meta.outputs.version }}
          cache-from: type=gha            # relit les couches des exécutions précédentes
          cache-to: type=gha,mode=max     # sauvegarde toutes les couches, y compris l'étape de build
```

Il faut aussi déclencher le workflow sur les tags : `push: tags: ["v*.*.*"]`.

1. Pour un push sur `main` : `sha-1a2b3c4` (le premier tag de la liste qui s'applique). Pour le tag git `v1.2.3` : `1.2.3`. Le binaire affichera cette valeur dans `/version`.
2. Le runner est une VM neuve à chaque job : son Docker ne contient aucune image ni couche. Le cache `gha` stocke les couches dans le cache GitHub entre deux exécutions ; avec `mode=max`, même les couches de l'étape `build` (le `go mod download`) sont gardées.

</details>

---

## Partie 3 : le scan comme barrière

**Consignes.**

1. Après le build, scannez `tp-api:scan` avec `aquasecurity/trivy-action` : sévérités HIGH et CRITICAL, uniquement les vulnérabilités corrigeables, et un code de sortie 1 si Trivy en trouve.
2. Sur une branche, remplacez temporairement l'image finale du Dockerfile par `golang:1.27` (ou `debian:12`). Poussez : la CI doit échouer au scan.
3. Remettez l'image distroless.

**Questions.**

1. Le scan passe aujourd'hui. Que faire pour être prévenu si une CVE apparaît la semaine prochaine dans une image déjà publiée ?
2. Un correctif urgent doit partir, mais le scan bloque sur une CVE d'une bibliothèque qui n'est pas utilisée par le code. Que faites-vous ?

<details>
<summary>Solution</summary>

```yaml
      - name: scan Trivy
        uses: aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25 # v0.36.0
        with:
          image-ref: tp-api:scan     # l'image chargée par le build précédent
          severity: HIGH,CRITICAL
          ignore-unfixed: true       # on ne bloque que sur ce qu'une mise à jour peut corriger
          exit-code: "1"             # Trivy échoue s'il trouve quelque chose : le job s'arrête ici
```

1. Ajouter un workflow planifié (`on: schedule: - cron: "0 6 * * 1"`) qui rescanne les images publiées (par digest) et ouvre une issue ou échoue en cas de nouvelle CVE. Dependabot, de son côté, proposera la nouvelle image de base.
2. On ne désactive pas le scan. On ajoute la CVE dans un fichier `.trivyignore` à la racine, avec un commentaire qui dit pourquoi elle ne s'applique pas, qui l'a décidé et jusqu'à quand. La décision est tracée dans Git et relue en PR comme le reste.

</details>

---

## Partie 4 : SBOM et publication

**Consignes.**

1. Générez un SBOM SPDX de `tp-api:scan` avec `anchore/sbom-action` et gardez-le comme artefact du job.
2. Connectez-vous à GHCR avec `docker/login-action` et `GITHUB_TOKEN`, **sauf** sur les pull requests.
3. Poussez l'image avec les tags calculés à la partie 2, toujours sauf sur les PR. Donnez au job le droit `packages: write`.
4. Affichez les tags et le digest dans le résumé du job (`$GITHUB_STEP_SUMMARY`).
5. Créez un tag `v0.2.0`, poussez-le, et vérifiez dans l'onglet *Packages* du dépôt les tags `0.2.0`, `0.2` et `sha-...`.

**Questions.**

1. Le job reconstruit l'image pour la pousser. Est-ce une perte de temps ? L'image poussée est-elle bien celle qui a été scannée ?
2. Pourquoi ne pas pousser depuis les pull requests, même sous un tag `pr-42` ?

<details>
<summary>Solution</summary>

```yaml
  image:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write             # uniquement pour ce job, qui pousse sur GHCR
    steps:
      # ... checkout, buildx, meta, build (local), scan ...

      - name: SBOM
        uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          image: tp-api:scan
          format: spdx-json
          artifact-name: sbom-tp-api.spdx.json   # téléchargeable depuis la page de l'exécution

      - name: login GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}   # jeton éphémère, aucun secret à créer

      - name: push
        if: github.event_name != 'pull_request'
        id: push
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: app
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}   # source, commit, date : visibles sur GHCR
          build-args: VERSION=${{ steps.meta.outputs.version }}
          cache-from: type=gha

      - name: résumé
        if: github.event_name != 'pull_request'
        run: |
          {
            echo "### Image publiée"
            echo '```'
            echo "${{ steps.meta.outputs.tags }}"
            echo "digest: ${{ steps.push.outputs.digest }}"
            echo '```'
          } >> "$GITHUB_STEP_SUMMARY"
```

1. Le second build ne recompile rien : toutes les couches viennent du cache `gha` rempli par le premier, avec les mêmes entrées. On obtient les mêmes couches. Il y a une différence : le premier build charge une image dans le Docker local (format Docker), le second pousse au format OCI avec des métadonnées (labels), donc le digest final peut différer de celui de `tp-api:scan` alors que le contenu des couches est identique. Pour une garantie stricte, on pousse d'abord, puis on scanne l'image du registry par son digest. C'est un bon exercice d'amélioration.
2. Une pull request peut venir d'un fork, avec du code que personne n'a relu. Lui donner le droit d'écrire dans le registry permettrait de publier n'importe quoi sous le nom du projet. GitHub limite d'ailleurs le `GITHUB_TOKEN` des PR de forks à la lecture.

</details>

---

## Partie 5 : épingler et maintenir à jour

**Consignes.**

1. Remplacez chaque `uses: owner/action@vX` par `uses: owner/action@<SHA complet> # vX.Y.Z`. Pour trouver le SHA d'une version :

   ```bash
   gh api repos/aquasecurity/trivy-action/commits/v0.36.0 --jq .sha
   ```

2. Créez `.github/dependabot.yml` qui surveille chaque semaine : les actions GitHub, l'image de base du Dockerfile, les modules Go.
3. Dans *Settings > Branches*, protégez `main` : PR obligatoire, jobs `test` et `image` verts obligatoires.

**Questions.**

1. Dependabot propose de passer `golang:1.27-alpine` à une nouvelle version. Qu'est-ce qui garantit que cette mise à jour ne casse rien ?
2. Pourquoi le commentaire `# v0.36.0` après le SHA ?

<details>
<summary>Solution</summary>

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: docker          # lignes FROM de app/Dockerfile
    directory: /app
    schedule: { interval: weekly }
  - package-ecosystem: gomod           # go.mod / go.sum
    directory: /app
    schedule: { interval: weekly }
  - package-ecosystem: github-actions  # uses: dans .github/workflows/
    directory: /
    schedule: { interval: weekly }
```

1. La PR de Dependabot passe par la même CI : tests, build, scan. Avec la protection de branche, elle ne peut être fusionnée que si tout est vert. C'est le couple « épingler + mettre à jour automatiquement + tester » qui rend l'épinglage sûr.
2. Un SHA est illisible pour un humain. Le commentaire dit quelle version il représente ; Dependabot le met à jour en même temps que le SHA.

</details>

---

## Bilan

1. Dessinez la chaîne complète, de `git push` à l'image sur GHCR, en indiquant à chaque étape ce qui l'arrête.
2. Quelles informations permettent, devant un conteneur qui tourne, de retrouver le commit exact dont il vient ? (Indices : tags, labels OCI, `/version`.)
3. Une personne obtient un accès en écriture à votre dépôt. Que peut-elle publier, et qu'est-ce qui l'en empêcherait ?
