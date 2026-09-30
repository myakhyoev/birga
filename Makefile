CURRENT_DIR=$(shell pwd)

APP=birga_backend

APP_CMD_DIR=${CURRENT_DIR}/cmd

IMG_NAME=${APP}
REGISTRY?=registry.gitlab.com/loyihalar/birga
TAG?=latest

SWAG_VERSION=v1.16.6
MIGRATIONS_DIR=${CURRENT_DIR}/migrations
POSTGRES_URL?=postgres://birga:birga@localhost:5432/birga?sslmode=disable

.PHONY: all build build-image push-image swag-init run up down migrate-up migrate-down migrate-create \
	lint-go test test-integration race coverage coverhtml dep clean help

all: build

build: ## build binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ${CURRENT_DIR}/bin/${APP} ${APP_CMD_DIR}/server/main.go

build-image: ## build docker image
	docker build --rm -t ${REGISTRY}/${IMG_NAME}:${TAG} .

push-image: ## push docker image
	docker push ${REGISTRY}/${IMG_NAME}:${TAG}

swag-init: ## regenerate swagger docs into api/docs
	go run github.com/swaggo/swag/cmd/swag@${SWAG_VERSION} init -g cmd/server/main.go -o api/docs --parseInternal

run: ## run application (reads .env if present)
	set -a; [ -f .env ] && . ./.env; set +a; go run cmd/server/main.go

up: ## start postgres + migrations + api with docker compose
	docker compose up -d --build

down: ## stop docker compose stack
	docker compose down

migrate-up: ## apply all migrations
	migrate -path ${MIGRATIONS_DIR} -database "${POSTGRES_URL}" up

migrate-down: ## roll back the last migration
	migrate -path ${MIGRATIONS_DIR} -database "${POSTGRES_URL}" down 1

migrate-create: ## create a migration: make migrate-create name=create_users_table
	migrate create -ext sql -dir ${MIGRATIONS_DIR} -seq -digits 6 ${name}

lint-go: ## lint go files
	golangci-lint run -c .golangci.yml ./...

test: ## run unit tests
	go test -short ./...

test-integration: ## run all tests incl. DB ones (needs a migrated database)
	TEST_POSTGRES_URL="${POSTGRES_URL}" go test -count=1 ./...

race: ## run data race detector
	go test -race -short ./...

coverage: ## print global code coverage
	go test -short -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

coverhtml: coverage ## open coverage report in browser
	go tool cover -html=coverage.out

dep: ## download dependencies
	go mod download

clean: ## remove build output
	@rm -rf ${CURRENT_DIR}/bin coverage.out

help: ## display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
