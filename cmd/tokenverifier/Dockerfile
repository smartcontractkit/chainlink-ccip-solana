FROM golang:1.26.10-alpine AS builder
WORKDIR /build

COPY go.mod go.sum ./
COPY contracts/gobindings/go.mod contracts/gobindings/go.sum contracts/gobindings/
# Root go.mod replaces this module with ./deployment; it is not published as its own GitHub module.
# Copy it before go mod download so we never fetch github.com/.../deployment (which fails in Docker).
COPY deployment/ ./deployment/

RUN --mount=type=cache,target=/go/pkg/mod,id=ccv-go-mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build,id=ccv-go-build \
    --mount=type=cache,target=/go/pkg/mod,id=ccv-go-mod \
    CGO_ENABLED=0 go build -ldflags='-s -w' -o /bin/token-verifier ./cmd/tokenverifier

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/token-verifier /bin/token-verifier
CMD ["/bin/token-verifier"]
