# syntax=docker/dockerfile:1

# --- Étape 1 : compilation. Image lourde (~300 Mo), jetée à la fin. ---
ARG GO_VERSION=1.27
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Dépendances d'abord : cette couche reste en cache tant que go.mod/go.sum ne bougent pas.
COPY go.mod go.sum ./
RUN go mod download

# Puis le code, qui change à chaque commit.
COPY . .
ARG VERSION=dev
# CGO_ENABLED=0 : binaire statique, pas besoin de libc dans l'image finale.
# -trimpath et -s -w : pas de chemins locaux ni de symboles de debug dans le binaire.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/api .

# --- Étape 2 : exécution. Ni shell, ni gestionnaire de paquets, utilisateur non-root. ---
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/api /api
# nonroot = UID 65532. Redit ici pour que ce soit visible à la lecture.
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/api", "healthcheck"]
ENTRYPOINT ["/api"]
