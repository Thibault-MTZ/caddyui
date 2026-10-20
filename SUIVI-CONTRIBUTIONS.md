# Atelier personnel — CaddyUI en français

La branche `homelab` est désormais la référence de ma version personnelle.
Elle reprend le travail de `i18n-personal`, conservée uniquement en local. Le
travail initial n'a pas été suivi avec un historique Git rigoureux dès le début.
Les commits récents suivent les essais et corrections au fil du travail.

Je m’aide d’outils d’IA pour explorer des idées, préparer des tests et organiser
mes contributions. Je relis et corrige le résultat, qui reste perfectible.

Une version antérieure est utilisée dans mon installation. Cette branche mise
à jour n'y a pas été déployée et tous les parcours ne sont pas vérifiés. Ce fork
n'est pas une version officielle ni une recommandation de mise à jour automatique.

## Organisation

- Référence personnelle : `homelab` ; déploiement réel à vérifier séparément.
- Ancien atelier : `i18n-personal`, conservé en local ; travail courant sur `homelab`.
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

## Prochaines tâches — suivi vivant

Objectif : avancer par petits lots utiles, avec des commits locaux en français
et une documentation courte. Les brouillons peuvent être imparfaits ; avant
une PR, le diff doit être ciblé, compréhensible et testé. Pas de grand nettoyage
préalable ni de réécriture des anciens commits pour embellir l'historique.

- [x] Créer et publier `homelab`, avec une présentation FR/EN du fork.
- [x] Vérifier le retour du mainteneur : PR #144 fusionnée le 20 octobre 2026 ;
  extraction anglaise et traduction française demandées séparément.
- [x] Repérer les lots Sécurité existants : `i18n-settings-security` et
  `fr-settings-security`. Ne pas refaire ce travail ni toucher aux worktrees occupés.
- [ ] Identifier le conteneur utilisé chez moi et relever sa version, sa source
  de lancement et, si disponible, son commit. Comparer à `homelab` avant de
  décider d'une mise à jour ; ne pas marquer la branche comme déployée sans preuve.
- [ ] Rafraîchir l'amont et vérifier les PR en cours avant chaque nouveau lot.
- [ ] Relire le diff d'extraction Sécurité seul : templates + `en.json`, sans
  français ni adaptations homelab. Rejouer `go test ./internal/server/ -run I18n`
  et les tests ciblés pertinents ; noter le commit et le résultat.
- [ ] Vérifier Sécurité en navigateur dans la VM : anglais, français, repli
  anglais d'une langue partielle, mise en page et valeurs des formulaires.
  Tester les protections réelles uniquement dans une instance jetable.
- [ ] Préparer la PR d'extraction Sécurité puis la PR française séparée.
  Tant que l'extraction n'est pas fusionnée, garder explicite la dépendance de
  la branche française ; après fusion, préparer son diff sur l'amont actualisé.
- [ ] Continuer les paramètres, une zone à la fois : Notifications, DNS,
  Analytics des paramètres, Intégrations, IA et Sauvegarde. Réutiliser `status.*`,
  garder les clés stables et les placeholders nommés identiques à l'anglais.
- [ ] Traiter les listes : hôtes proxy, redirections, routes avancées, certificats.
- [ ] Reprendre Analytics sur l'amont récent, y compris `analytics_requests.html`.
  Utiliser le prototype local comme matériau, sans envoyer son gros diff en bloc.
- [ ] Passer aux petits formulaires ; réserver le formulaire des hôtes proxy
  pour la fin et une contribution dédiée.
- [ ] Après validation de chaque lot, intégrer ce qui est utile à `homelab`.
  Consigner en quelques lignes : changement, essai effectué, limite connue et
  prochaine étape. Le déploiement reste une opération distincte.

Les résultats du bilan précédent restent des preuves historiques, pas des tests
rejoués à chaque modification de cette liste. Aucune nouvelle PR ni aucun
déploiement ne sont créés par cette mise à jour documentaire.


## Étape locale — résumé de période Analytics

Le résumé de période de l’explorateur utilisait encore une chaîne anglaise
produite dans le gestionnaire Go, même avec un profil français. Il utilise
maintenant le catalogue existant, avec deux clés supplémentaires pour 1 h et 6 h.
Les valeurs des filtres et leurs alias restent techniques et inchangés.

Le test de régression a d’abord reproduit le défaut : les huit cas français
échouaient. Après correction, les tests ciblés Analytics et i18n passent
(serveur : 9,733 s), les 4 869 clés françaises sont renseignées et la compilation
`go build -p 1 ./...` passe sous Go 1.27.2, Linux amd64, CGO désactivé.
Ces essais utilisent des bases temporaires et un faux Caddy dans le laboratoire ;
ils ne constituent pas une validation navigateur ni un déploiement.

Ce correctif reste dans le fork personnel. Le prochain lot à proposer reste
Sécurité, suivant l’ordre demandé par le mainteneur : extraction anglaise puis
français séparé. Les phrases fragmentées autour du balisage doivent encore être
relues avant soumission. Aucune nouvelle PR ou publication sur le dépôt officiel
sans accord préalable.
