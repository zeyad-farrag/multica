.PHONY: help sqlc sqlc-verify build test vet check

help: ## Show this help.
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-15s %s\n", $$1, $$2}'

sqlc: ## Regenerate sqlc code under pkg/db/generated.
	sqlc generate -f sqlc.yaml

sqlc-verify: ## Verify sqlc generated code is in sync (CI guard).
	sqlc diff -f sqlc.yaml

build: ## Build the team-app server binary.
	go build ./...

test: ## Run all team-app Go tests. Requires DATABASE_URL pointing at a reachable Postgres.
	go test ./... -race

vet: ## Run go vet across the module.
	go vet ./...

check: vet test ## Run vet + tests — the team-app pre-push gate.
