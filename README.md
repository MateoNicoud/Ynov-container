# TP Ynov Docker

Un wordpress + mariadb + une petite API Go en docker compose, openbao a coté pour les secrets, puis la même stack sur Kubernetes (minikube) et sur Azure.

![](./docs/img/schema.png)

## Lancer

```
cp .env.example .env
docker compose up -d --build
```

Wordpress sur http://localhost:80, l'API sur http://localhost:8081/api/posts, OpenBao sur http://localhost:8200.
Adminer (debug) avec `docker compose --profile debug up -d`, sur http://localhost:8082.

Pour se connecter à OpenBao (interface web ou CLI), le token root est dans le volume `bao_keys` :

```
docker compose run --rm --no-deps --entrypoint cat bao-init /bao-keys/root_token
```

## Les TP

Ce que couvre le dépôt par rapport au cours : [écart avec le cours](./docs/ecart-cours.md).

1. [OpenBao : sortir les secrets de la stack WordPress](./docs/tp-openbao.md)
2. [HashiCorp Vault : identifiants MariaDB dynamiques](./docs/tp-vault.md) (fait ici avec OpenBao, voir `openbao/bootstrap.sh`)
3. [Construire, durcir et publier sa propre image](./docs/tp3-image.md) (`app/`, `app.compose.yml`)
4. [Intégration continue avec GitHub Actions](./docs/tp4-ci.md) (`.github/`)
5. [Premiers pas dans Kubernetes avec minikube](./docs/tp5-kubernetes-bases.md) (`k8s/tp5/`)
6. [La stack WordPress dans Kubernetes](./docs/tp6-kubernetes-stack.md) (`k8s/base/`, `k8s/minikube/`, `k8s/gateway/`)
7. [Résilience, montée en charge et sécurité dans le cluster](./docs/tp7-kubernetes-production.md)
8. [OpenBao dans Kubernetes](./docs/tp8-openbao-kubernetes.md) (`k8s/openbao/`)
9. [Mise en production sur Azure](./docs/tp9-azure.md) (`k8s/azure/`, `.github/workflows/deploy-azure.yml`)

## Kubernetes en local

```
minikube start --driver=docker --cni=calico --cpus=4 --memory=6g
minikube addons enable ingress
minikube addons enable metrics-server
docker build -t tp-api:dev app && minikube image load tp-api:dev

kubectl apply -f k8s/base/namespace.yaml
kubectl -n wordpress create secret generic db \
  --from-literal=root-password="$(openssl rand -base64 24)" \
  --from-literal=wordpress-password="$(openssl rand -base64 24)" \
  --from-literal=api-password="$(openssl rand -base64 24)"
kubectl apply -k k8s/minikube

kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 8080:80
```

Wordpress sur http://wordpress.localhost:8080, l'API sur http://api.localhost:8080/api/posts.

[Références](./docs/references.md)
