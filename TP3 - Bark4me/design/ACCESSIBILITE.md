# Accessibilité de Bark4me

Règles pour les tickets front. Les ratios ci-dessous sont calculés avec la formule WCAG 2.1 sur les valeurs de `tokens.css` ; la page `index.html` (section 9) les recalcule en direct.

## Contrastes (WCAG 2.1 AA)

Seuils : 4,5:1 pour le texte, 3:1 pour les icônes et bordures de composants.

| Paire | Seuil | Normal | Daltochien |
|---|---|---|---|
| Texte sur fond (`ink` / `bg`) | 4.5:1 | 16.10:1 | 15.71:1 |
| Texte sur carte (`ink` / `surface`) | 4.5:1 | 17.17:1 | 16.87:1 |
| Texte sur fond alterné (`ink` / `surface-2`) | 4.5:1 | 14.39:1 | 13.95:1 |
| Texte secondaire sur fond (`ink-soft` / `bg`) | 4.5:1 | 6.91:1 | 7.15:1 |
| Texte secondaire sur carte (`ink-soft` / `surface`) | 4.5:1 | 7.37:1 | 7.68:1 |
| Texte secondaire sur fond alterné (`ink-soft` / `surface-2`) | 4.5:1 | 6.18:1 | 6.35:1 |
| Texte sur couleur de marque (`on-brand` / `brand`) | 4.5:1 | 5.36:1 | 7.38:1 |
| Lien de marque sur fond (`brand` / `bg`) | 4.5:1 | 5.02:1 | 6.88:1 |
| Icône Patte survolée (`on-patte` / `patte`) | 4.5:1 | 5.89:1 | 7.38:1 |
| Icône Truffe survolée (`on-truffe` / `truffe`) | 4.5:1 | 6.29:1 | 10.27:1 |
| Icône Panneau survolée (`on-panneau` / `panneau`) | 4.5:1 | 6.29:1 | 12.41:1 |
| Icône Patte normale (`patte-line` / `surface`) | 3:1 | 5.89:1 | 7.38:1 |
| Icône Truffe normale (`truffe-line` / `surface`) | 3:1 | 6.29:1 | 6.45:1 |
| Icône Panneau normale (`panneau-line` / `surface`) | 3:1 | 6.29:1 | 12.41:1 |
| Texte du badge Pédigrée (`badge-ink` / `badge-bg`) | 4.5:1 | 8.41:1 | 11.69:1 |
| Bordure du badge (`badge-line` / `badge-bg`) | 3:1 | 4.87:1 | 5.76:1 |
| Texte du Match, début du dégradé (`on-match` / `match-a`) | 4.5:1 | 5.36:1 | 7.38:1 |
| Texte du Match, fin du dégradé (`on-match` / `match-b`) | 4.5:1 | 9.72:1 | 16.87:1 |
| Bordure de composant (`line` / `bg`) | 3:1 | 3.52:1 | 3.72:1 |
| Anneau de focus sur fond (`focus` / `bg`) | 3:1 | 5.90:1 | 6.88:1 |
| Anneau de focus sur carte (`focus` / `surface`) | 3:1 | 6.29:1 | 7.38:1 |

Tous les ratios atteignent leur seuil. Le texte posé sur les photos (nom, race, accroche) l'est sur un dégradé sombre en bas de carte, au moins à 0,55 d'opacité noire sous le texte.

## Mode daltochien

- Aucun rouge ni vert : uniquement des bleus, des jaunes et des neutres bleutés.
- Les trois actions ne reposent jamais sur la teinte seule : Patte est un bleu moyen, Truffe un jaune clair, Panneau une ardoise sombre (trois clartés distinctes), et les icônes ont trois formes différentes. Chaque bouton porte aussi un libellé accessible.
- Les noms de variables sont identiques dans les deux modes. Activation : `data-mode="daltochien"` sur `<html>`, choix stocké dans `localStorage` (`bark4me-mode`).

## Cibles tactiles

- Boutons Patte, Truffe, Panneau : 64 px (`--target-action`).
- Tout autre contrôle (interrupteur, liens de navigation, boutons) : 44 px minimum (`--target-min`). Espace d'au moins 24 px entre les trois actions.

## Focus

- Anneau de 3 px (`--c-focus`, au moins 3:1 sur fond et sur carte) avec décalage de 2 px et halo clair, visible sur photo comme sur aplat. Jamais `outline: none` sans remplaçant.
- Ordre de tabulation : en-tête, interrupteur, carte, Panneau, Patte, Truffe. Au clavier, flèche gauche = Panneau, droite = Patte, haut = Truffe (en plus des boutons).

## Sémantique

- Boutons natifs avec `aria-label` (« Patte », « Truffe », « Panneau »), jamais de `div` cliquable.
- Interrupteur : `<button role="switch" aria-checked>`.
- Photos : `alt` = nom, race, accroche. Icônes décoratives : `aria-hidden`.
- Résultat d'un Swipe et fin du Paquet annoncés dans une région `role="status"`.
- Pages Éleveurs et Handicaps canins : encart « page humoristique » visible, sans portée médicale.

## Mouvement

- Retours animés (vol de la carte, tampon, Match) : 520 ms maximum.
- `prefers-reduced-motion: reduce` : transitions et animations réduites à 0,01 ms, la carte disparaît sans déplacement, aucun effet de survol animé. La logique JS saute l'animation et passe directement au résultat.
- `forced-colors: active` : bordures rendues en `ButtonText` sur les boutons, badges et interrupteur.

## Ressources

Aucune police ni ressource externe : piles système (`--font-display`, `--font-body`, `--font-mono`), icônes en SVG inline, photos servies par `/photos`.
