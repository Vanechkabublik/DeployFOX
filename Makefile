.PHONY: local down test lint build
local:
	docker compose -f docker-compose.local.yml up -d --build
down:
	docker compose -f docker-compose.local.yml down
test:
	docker run --rm -v "$(CURDIR)/backend:/src" -w /src golang:1.27.3-alpine sh -c 'go test ./... && go vet ./...'
	cd frontend && npm run typecheck && npm run lint
lint:
	cd frontend && npm run lint
build:
	docker compose -f docker-compose.local.yml build
