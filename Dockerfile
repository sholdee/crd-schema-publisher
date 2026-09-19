FROM --platform=$BUILDPLATFORM golang:1.27.1@sha256:1cfcdb11f37fce9429f617100f39e0251748bbaba454bd431275155701765058 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /crd-schema-publisher ./cmd/

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.title="crd-schema-publisher"
LABEL org.opencontainers.image.description="Extracts CRD JSON schemas from Kubernetes and publishes to Cloudflare Pages"
LABEL org.opencontainers.image.url="https://kube-schemas.shold.io"
LABEL org.opencontainers.image.source="https://github.com/sholdee/crd-schema-publisher"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /crd-schema-publisher /crd-schema-publisher
ENTRYPOINT ["/crd-schema-publisher"]
