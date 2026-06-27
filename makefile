# ==============================================================================
# Tetra — a scalable async payload-processing service.
#
# Quick start:
#   make compose/up        # full local stack (db, minio, otel, jaeger, prom, grafana, app)
#   make compose/logs
#   make compose/down
#
# Local (without docker), needs Postgres + MinIO + collector reachable:
#   make db/migrations/up && make minio/bootstrap
#   make run/mockproc &  make run/worker &  make run/api
# ==============================================================================

SHELL := /bin/bash

VERSION        := 0.0.1
TETRA_IMAGE    := tetra:$(VERSION)
WORKER_IMAGE   := tetra-worker:$(VERSION)
MOCKPROC_IMAGE := tetra-mockproc:$(VERSION)

KIND_CLUSTER := tetra
MIGRATIONS   := ./migrations
COMPOSE      := docker compose -f zarf/compose/docker-compose.yaml

# Local DSN used by the golang-migrate CLI (force/goto recovery commands).
DB_DSN := postgres://postgres:postgres@localhost:5432/tetra?sslmode=disable

.DEFAULT_GOAL := help

# ==============================================================================
# Help

help: ## Show this help.
	@grep -E '^[a-zA-Z0-9_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ==============================================================================
# Build & run

build/tetra: ## Build the tetra API binary.
	go build -ldflags "-X main.tag=$(VERSION)" -o bin/tetra ./api/services/tetra

build/worker: ## Build the worker binary.
	go build -ldflags "-X main.tag=$(VERSION)" -o bin/worker ./api/services/worker

build/mockproc: ## Build the mock processor binary.
	go build -ldflags "-X main.tag=$(VERSION)" -o bin/mockproc ./api/services/mockproc

build/admin: ## Build the admin tool.
	go build -ldflags "-X main.tag=$(VERSION)" -o bin/admin ./api/tooling/admin

run/api: ## Run the API service locally.
	go run ./api/services/tetra

run/worker: ## Run the worker locally.
	go run ./api/services/worker

run/mockproc: ## Run the mock processor locally.
	go run ./api/services/mockproc

# ==============================================================================
# Database migrations (golang-migrate)

db/migrations/new: ## Scaffold a new migration: make db/migrations/new name=create_widgets
	migrate create -seq -ext .sql -dir $(MIGRATIONS) $(name)

db/migrations/up: ## Apply all pending migrations.
	TETRA_DB_HOST=localhost:5432 go run ./api/tooling/admin migrate-up

db/migrations/down: ## Roll back the most recent migration.
	TETRA_DB_HOST=localhost:5432 go run ./api/tooling/admin migrate-down

db/migrations/version: ## Print the current migration version.
	TETRA_DB_HOST=localhost:5432 go run ./api/tooling/admin migrate-version

db/migrations/force: ## Force the version (drift recovery): make db/migrations/force version=1
	migrate -path $(MIGRATIONS) -database "$(DB_DSN)" force $(version)

db/migrations/goto: ## Migrate to a specific version: make db/migrations/goto version=1
	migrate -path $(MIGRATIONS) -database "$(DB_DSN)" goto $(version)

# ==============================================================================
# Object storage

minio/bootstrap: ## Create the MinIO bucket (idempotent).
	go run ./api/tooling/admin minio-bootstrap

# ==============================================================================
# Docker & Compose

docker/build: ## Build all service images.
	docker build -f zarf/docker/dockerfile.tetra    -t $(TETRA_IMAGE)    --build-arg BUILD_REF=$(VERSION) .
	docker build -f zarf/docker/dockerfile.worker   -t $(WORKER_IMAGE)   --build-arg BUILD_REF=$(VERSION) .
	docker build -f zarf/docker/dockerfile.mockproc -t $(MOCKPROC_IMAGE) --build-arg BUILD_REF=$(VERSION) .

compose/up: ## Start the full local stack.
	$(COMPOSE) up --build -d

compose/down: ## Stop the local stack and remove volumes.
	$(COMPOSE) down -v

compose/logs: ## Follow the local stack logs.
	$(COMPOSE) logs -f

# ==============================================================================
# Kubernetes (KIND)

kind/up: ## Create the KIND cluster and install metrics-server (needed by HPA).
	kind create cluster --name $(KIND_CLUSTER) --config zarf/k8s/dev/kind-config.yaml
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	kubectl patch -n kube-system deployment metrics-server --type=json \
		-p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

kind/down: ## Delete the KIND cluster.
	kind delete cluster --name $(KIND_CLUSTER)

dev/load: docker/build ## Load the images into the KIND cluster.
	kind load docker-image $(TETRA_IMAGE) $(WORKER_IMAGE) $(MOCKPROC_IMAGE) --name $(KIND_CLUSTER)

dev/apply: ## Apply the dev kustomization.
	kubectl apply -k zarf/k8s/dev

dev/restart: ## Restart the app deployments.
	kubectl rollout restart deployment tetra worker mockproc -n tetra-system

dev/status: ## Show pods and HPAs.
	kubectl get pods,hpa,svc -n tetra-system

dev/logs: ## Tail the tetra logs.
	kubectl logs -n tetra-system -l app=tetra -f

# ==============================================================================
# Quality

test: ## Run the unit tests.
	go test ./...

lint: ## Run staticcheck.
	staticcheck ./...

tidy: ## Tidy and format.
	go mod tidy
	go fmt ./...

audit: ## Run vet + govulncheck + staticcheck.
	go vet ./...
	govulncheck ./...
	staticcheck ./...

dev/tools: ## Install dev tooling (migrate, staticcheck, govulncheck, hey).
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/rakyll/hey@latest

# ==============================================================================
# Load testing (hey) — see zarf/loadtest/loadtest.sh

load/text: ## Load test: JSON text ingestion (vars: N, C, HOST).
	./zarf/loadtest/loadtest.sh text

load/upload: ## Load test: multipart file upload to MinIO (vars: N, C, SIZE, HOST).
	./zarf/loadtest/loadtest.sh upload

load/watch: ## Watch the queue drain (payload counts over time).
	./zarf/loadtest/loadtest.sh watch

load/all: ## Run text + upload bursts, then watch the drain.
	./zarf/loadtest/loadtest.sh all

# ==============================================================================
# Observability shortcuts (macOS `open`)

obs/open: ## Open the observability UIs (compose).
	open http://localhost:16686   # Jaeger
	open http://localhost:9090    # Prometheus
	open http://localhost:3002    # Grafana
	open http://localhost:9101    # MinIO console

.PHONY: help build/tetra build/worker build/mockproc build/admin run/api run/worker run/mockproc \
	db/migrations/new db/migrations/up db/migrations/down db/migrations/version db/migrations/force \
	db/migrations/goto minio/bootstrap docker/build compose/up compose/down compose/logs \
	kind/up kind/down dev/load dev/apply dev/restart dev/status dev/logs test lint tidy audit \
	dev/tools load/text load/upload load/watch load/all obs/open
