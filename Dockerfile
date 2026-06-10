FROM lucas42/lucos_navbar:2.1.73 AS navbar

FROM golang:1.26 AS builder

WORKDIR /go/src/lucos_root

# Download module dependencies before copying source (better layer caching)
COPY go.mod .
RUN go mod download

# Copy source and static assets
COPY main.go .
COPY templates/ templates/
COPY public/ public/
# Inject lucos_navbar.js into public/ before embedding (so it is bundled into the binary)
COPY --from=navbar lucos_navbar.js public/lucos_navbar.js

RUN CGO_ENABLED=0 go build -o lucos_root .

# Minimal runtime image: no shell, just the binary (all assets embedded)
FROM gcr.io/distroless/static-debian12
ARG VERSION
ENV VERSION=$VERSION

COPY --from=builder /go/src/lucos_root/lucos_root /lucos_root

HEALTHCHECK CMD ["/lucos_root", "--healthcheck"]
CMD ["/lucos_root"]
