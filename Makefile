APP=family-app

.PHONY: run
run:
	HTTP_PORT=8080 go run ./cmd/family-app

.PHONY: test
test:
	go test ./...

.PHONY: e2e
e2e:
	@if [ -z "$$E2E_DB_DSN" ]; then \
		echo "E2E_DB_DSN is not set"; \
		exit 1; \
	fi
	go test -tags e2e ./e2e/...
