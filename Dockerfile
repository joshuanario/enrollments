FROM golang:1.27-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /mybuild
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY pkg ./pkg
RUN CGO_ENABLED=0 GOOS=linux go build -o /mybuild/main ./cmd/enrollments

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /mybuild/main /
# Scenario Player playlists; served from FIXTURES_DIR (default test/fixtures, relative to /)
COPY test/fixtures /test/fixtures

USER 1000
EXPOSE 8080
STOPSIGNAL SIGINT
ENTRYPOINT ["/main"]
