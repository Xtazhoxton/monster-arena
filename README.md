# monster-arena

Plateforme de combats de créatures au tour par tour, **event-driven et serverless sur AWS**.
Projet d'apprentissage : system design, microservices, event-driven, IaC, CI/CD, puis IA générative et ML.

> Projet personnel, non commercial, sans affiliation avec Nintendo, Game Freak ou The Pokémon Company.
> Aucun asset officiel n'est versionné dans ce dépôt.

## Stack

- **Langages** : Go (services), Python (ML, scripts)
- **Cloud** : AWS, région `ap-northeast-1`, serverless-first
- **IaC** : Terraform
- **CI/CD** : GitHub Actions (auth AWS via OIDC)

## Organisation du dépôt

```
services/     un dossier par microservice
internal/     code Go partagé entre services
infra/        Terraform (bootstrap, modules, environnements)
```

## Roadmap

- [ ] **Phase 0** : fondations (compte sécurisé, Terraform, CI/CD, hello world)
- [ ] **Phase 1** : Catalog (CRUD créatures, attaques, types)
- [ ] **Phase 2** : Battle engine
- [ ] **Phase 3** : Matchs temps réel (WebSocket, orchestration, timeouts)
- [ ] **Phase 4** : Ranking, ligue, saisons
- [ ] **Phase 5** : Draft
- [ ] **Phase 6** : Bots IA
- [ ] **Phase 7** : Génération de créatures par LLM
- [ ] **Phase 8** : ML maison (prédiction de victoire, fine-tuning)
- [ ] **Phase 9** : Assets 3D et VFX générés
