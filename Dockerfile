FROM golang:1.27.1-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY docs/serve.go docs/openapi.yaml docs/swagger.html ./docs/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gowork-api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gowork-migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gowork-bootstrap ./cmd/bootstrap && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gowork-seed ./cmd/seed && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gowork-probe ./cmd/probe

FROM scratch AS runtime
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/ /app/
COPY db/migrations /app/db/migrations
USER 65532:65532
ENV PORT=8080
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 CMD ["/app/gowork-probe", "/health"]
ENTRYPOINT ["/app/gowork-api"]
