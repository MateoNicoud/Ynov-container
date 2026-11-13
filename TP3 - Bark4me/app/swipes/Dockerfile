FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /swipes .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /swipes /swipes
EXPOSE 8080
ENTRYPOINT ["/swipes"]
