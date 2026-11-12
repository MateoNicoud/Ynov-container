FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /photos .
RUN mkdir -p /data/photos

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /photos /photos
# Photos livrées : copiées vers le volume partagé par `photos init`.
COPY images/ /seed/
# /data appartient à l'utilisateur non-root : un volume nommé neuf en hérite.
COPY --from=build --chown=65532:65532 /data /data
EXPOSE 8080
ENTRYPOINT ["/photos"]
