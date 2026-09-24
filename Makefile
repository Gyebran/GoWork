GO ?= go

.PHONY: run build test fmt fmt-check vet check
run:
	$(GO) run ./cmd/api
build:
	$(GO) build -trimpath -o bin/gowork-api ./cmd/api
	$(GO) build -trimpath -o bin/gowork-bootstrap ./cmd/bootstrap
	$(GO) build -trimpath -o bin/gowork-seed ./cmd/seed
	$(GO) build -trimpath -o bin/gowork-migrate ./cmd/migrate
	$(GO) build -trimpath -o bin/gowork-probe ./cmd/probe
test:
	$(GO) test -race ./...
fmt:
	$(GO) fmt ./...
fmt-check:
	@test -z "$$(gofmt -l cmd internal tests)" || (gofmt -l cmd internal tests; exit 1)
vet:
	$(GO) vet ./...
check: fmt-check vet test build

.PHONY: sqlc test-integration migrate-up migrate-down db-up db-down db-grants
sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
test-integration:
	$(GO) test -race -tags=integration -count=1 ./tests/integration/...
migrate-up:
	$(GO) run ./cmd/migrate up
migrate-down:
	$(GO) run ./cmd/migrate down
db-up:
	docker compose up -d --wait postgres
db-down:
	docker compose down
db-grants:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U gowork_owner -d gowork < db/local/grants.sql

.PHONY: bootstrap seed-demo
bootstrap:
	$(GO) run ./cmd/bootstrap
seed-demo:
	$(GO) run ./cmd/seed

.PHONY: docker-up docker-down docker-seed docker-smoke
docker-up:
	docker compose up --build -d --wait
docker-down:
	docker compose down
docker-seed:
	docker compose run --rm --no-deps --entrypoint /app/gowork-seed api
docker-smoke:
	python3 scripts/docker-smoke.py
