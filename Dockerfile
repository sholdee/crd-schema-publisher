FROM --platform=$BUILDPLATFORM golang:1.26.4@sha256:11fd8f7f63db3b6fb198797042ba4c40a4a34dc83325d3328ca3bc4bb7726786 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /crd-schema-publisher ./cmd/

FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240
LABEL org.opencontainers.image.title="crd-schema-publisher"
LABEL org.opencontainers.image.description="Extracts CRD JSON schemas from Kubernetes and publishes to Cloudflare Pages"
LABEL org.opencontainers.image.url="https://kube-schemas.shold.io"
LABEL org.opencontainers.image.source="https://github.com/sholdee/crd-schema-publisher"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /crd-schema-publisher /crd-schema-publisher
ENTRYPOINT ["/crd-schema-publisher"]
