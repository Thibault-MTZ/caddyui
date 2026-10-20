# Atelier personnel — CaddyUI en français

Cette branche `i18n-personal` est ma version de développement et de test. Le
travail initial n'a pas été suivi avec un historique Git rigoureux dès le début.
Les commits récents suivent les essais et corrections au fil du travail.

Je m’aide d’outils d’IA pour explorer des idées, préparer des tests et organiser
mes contributions. Je relis et corrige le résultat, qui reste perfectible.

Une version antérieure est utilisée dans mon installation. Cette branche mise
à jour n'y a pas été déployée et tous les parcours ne sont pas vérifiés. Ce fork
n'est pas une version officielle ni une recommandation de mise à jour automatique.

## Organisation

- Version complète : `i18n-personal`, commits et suivi en français.
- Extraction propre : `i18n-settings-security`, basée sur l'amont `e96a090`
  (après v2.66.1), sans ajout de traduction française.
- Français : `fr-settings-security`, basée sur la branche d'extraction ; son
  diff propre porte uniquement sur `web/i18n/fr.json`.
- Les anciennes branches de travail sont conservées.

La séparation suit la demande du mainteneur dans
[PR144](https://github.com/X4Applegate/caddyui/pull/144#issuecomment-6299054728).
Les futurs lots avanceront par zone : autres paramètres, listes, Analytics,
petits formulaires, puis formulaire des hôtes proxy. Les branches sont publiées
sur mon fork pour préparer la relecture ; aucune nouvelle PR n'est encore ouverte.

## Travail repris

Fusion de l'amont récent et conservation du catalogue russe. Résolution des
conflits Analytics en gardant le nouvel explorateur, les liens de filtrage et
les percentiles p50/p95/p99. Ajout du français pour les nouveaux textes,
regroupement des phrases contenant des compteurs et extraction séparée du lot
Sécurité avec des clés stables `settings.security.*`.

Le lot Sécurité réutilise les clés partagées disponibles. Noms de champs,
valeurs techniques, exemples `group|role`, URLs et code de contrôle CAPTCHA
restent inchangés. Les langues non complétées utilisent le repli anglais.

## Validation

Les vérifications et leurs limites sont consignées dans
[le bilan des essais](docs/contributions/BILAN-20261020.md).
Une compilation ou une suite de tests réussie ne garantit pas tous les parcours
SSO/CAPTCHA réels. Ils nécessitent encore une relecture et des essais adaptés.
