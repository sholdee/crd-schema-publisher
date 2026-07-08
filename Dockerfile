FROM --platform=$BUILDPLATFORM golang:1.26.5@sha256:63f132d58c1f589f0dcda584933a9bb44bfda1150f1506377f5a902f34d86033 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /crd-schema-publisher ./cmd/

FROM gcr.io/distroless/static:nonroot@sha256:d29e660cc75a5b6b1334e03c5c81ccf9bc0884a002c6000dbf0fb96034814478
LABEL org.opencontainers.image.title="crd-schema-publisher"
LABEL org.opencontainers.image.description="Extracts CRD JSON schemas from Kubernetes and publishes to Cloudflare Pages"
LABEL org.opencontainers.image.url="https://kube-schemas.shold.io"
LABEL org.opencontainers.image.source="https://github.com/sholdee/crd-schema-publisher"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /crd-schema-publisher /crd-schema-publisher
ENTRYPOINT ["/crd-schema-publisher"]
