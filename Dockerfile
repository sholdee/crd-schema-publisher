FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:0c050feb47802ae436b9030833694a0b80160d08e3b3049ad30bf7c9e724e09d AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /crd-schema-publisher ./cmd/

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
LABEL org.opencontainers.image.title="crd-schema-publisher"
LABEL org.opencontainers.image.description="Extracts CRD JSON schemas from Kubernetes and publishes to Cloudflare Pages"
LABEL org.opencontainers.image.url="https://kube-schemas.shold.io"
LABEL org.opencontainers.image.source="https://github.com/sholdee/crd-schema-publisher"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /crd-schema-publisher /crd-schema-publisher
ENTRYPOINT ["/crd-schema-publisher"]
