# Multi-stage build. modernc.org/sqlite is pure Go, so no CGO and no glibc.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src

# Better layer caching: copy module manifests first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -trimpath -ldflags="-s -w" -o /out/unimatrix ./cmd/unimatrix

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/unimatrix /app/unimatrix
ENV UNIMATRIX_ADDR=:8080
ENV UNIMATRIX_DB=/data/unimatrix.db
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/unimatrix"]
