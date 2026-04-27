# Multi-stage build for the team-app Go server.
# Story 1.1 AC4 / Task 4.1.

FROM golang:1.26-alpine AS builder
WORKDIR /src

# Copy dep manifests first so layer cache survives source-only edits.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a fully static binary so the distroless runtime can exec it directly.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/team-app-server \
    ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/team-app-server /team-app-server

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/team-app-server"]
