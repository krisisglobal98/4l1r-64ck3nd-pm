-include .env
export

.PHONY: run build test test-integration test-e2e migrate-up migrate-down migrate-test seed-sample db-roles

run:            ## start the service (reads .env)
	go run ./cmd/pm-service

build:
	go build -o bin/pm-service ./cmd/pm-service

test:           ## unit tests (no database)
	go test ./...

test-integration: migrate-test ## unit + Postgres tests against TEST_DATABASE_URL
	go test ./... -count=1

E2E_PORT ?= 8082

# Black-box suite (positive, negative, stress) over HTTP: starts the service on E2E_PORT against the TEST database,
# in the Asia/Jakarta zone on purpose (times must still be UTC), runs test/e2e, stops it. The suite resets the test
# database to the migration seed before and after.
test-e2e: build migrate-test
	@PORT=$(E2E_PORT) PM_AUTH_MODE=default-user DATABASE_URL="$(TEST_APP_DATABASE_URL)" TZ=Asia/Jakarta PGTZ=Asia/Jakarta LOG_LEVEL=warn ./bin/pm-service & pid=$$!; \
	trap "kill $$pid" EXIT; \
	curl -s -o /dev/null --retry 20 --retry-connrefused --retry-delay 1 http://localhost:$(E2E_PORT)/readyz; \
	E2E_BASE_URL=http://localhost:$(E2E_PORT) go test -tags e2e -count=1 -v ./test/e2e/

# Migrations run as the owner; afterwards the application role gets access to the (new) tables.
migrate-up:
	migrate -path migrations -database "$(MIGRATE_DATABASE_URL)" up
	psql "$(MIGRATE_DATABASE_URL)" -v ON_ERROR_STOP=1 -q -f deploy/grants.sql

migrate-down:   ## roll back the last migration
	migrate -path migrations -database "$(MIGRATE_DATABASE_URL)" down 1

migrate-test:
	migrate -path migrations -database "$(TEST_DATABASE_URL)" up
	psql "$(TEST_DATABASE_URL)" -v ON_ERROR_STOP=1 -q -f deploy/grants.sql

db-roles:       ## create/update the application role alir_pm_app (once per cluster; PM_APP_PASSWORD)
	PGOPTIONS="-c pm.app_password=$(PM_APP_PASSWORD)" psql "$(MIGRATE_DATABASE_URL)" -v ON_ERROR_STOP=1 -q -f deploy/roles.sql

seed-sample:    ## sample organization tree (development/demo only; safe to run again)
	psql "$(MIGRATE_DATABASE_URL)" -v ON_ERROR_STOP=1 -q -f seeds/sample_org.sql
