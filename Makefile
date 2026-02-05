APP=family-app

.PHONY: run
run:
	HTTP_PORT=8080 go run ./cmd/family-app

.PHONY: test
test:
	go test ./...
