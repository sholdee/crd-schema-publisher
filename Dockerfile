FROM --platform=$BUILDPLATFORM golang:1.26.3@sha256:257c1f60c465aa5d22b4d81f9ae73643a12f228a10165c658ec77bd6ff791f34 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /crd-schema-publisher ./cmd/

FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39
LABEL org.opencontainers.image.title="crd-schema-publisher"
LABEL org.opencontainers.image.description="Extracts CRD JSON schemas from Kubernetes and publishes to Cloudflare Pages"
LABEL org.opencontainers.image.url="https://kube-schemas.shold.io"
LABEL org.opencontainers.image.source="https://github.com/sholdee/crd-schema-publisher"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /crd-schema-publisher /crd-schema-publisher
ENTRYPOINT ["/crd-schema-publisher"]
