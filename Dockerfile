FROM lucas42/lucos_navbar:2.2.0 AS navbar

FROM golang:1.26 AS builder

WORKDIR /go/src/lucos_root

# Download module dependencies before copying source (better layer caching)
COPY go.mod .
RUN go mod download

# Copy source and embedded assets (public/ and templates/ live under src/)
COPY src/ src/
# Inject lucos_navbar.js into public/ before embedding (so it is bundled into the binary)
COPY --from=navbar lucos_navbar.js src/public/lucos_navbar.js

RUN CGO_ENABLED=0 go build -o lucos_root ./src/

# distroless/static-debian12 rather than scratch: includes the CA certificate
# bundle, which is required for the outbound HTTPS calls to configy and
# every service's /_info endpoint. scratch has no CA certs, so TLS would
# fail with "certificate signed by unknown authority".
FROM gcr.io/distroless/static-debian12
ARG VERSION
ENV VERSION=$VERSION

COPY --from=builder /go/src/lucos_root/lucos_root /lucos_root

HEALTHCHECK CMD ["/lucos_root", "--healthcheck"]
CMD ["/lucos_root"]
