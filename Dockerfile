FROM golang:1.25 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/ppiankov/tote/internal/version.Version=${VERSION}" \
    -o /tote ./cmd/tote/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /tote /tote
ENTRYPOINT ["/tote"]
