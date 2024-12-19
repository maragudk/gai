.PHONY: benchmark
benchmark:
	go test -bench=.

.PHONY: cover
cover:
	go tool cover -html=cover.out

.PHONY: evaluate
evaluate:
	go test -json -run TestEval ./... | jq 'select(.Test != null and .Action == "output" and (.Output | contains("score"))) | del(.Action)'

.PHONY: lint
lint:
	golangci-lint run

.PHONY: test
test:
	go test -coverprofile=cover.out -shuffle on ./...

.PHONY: test-down
test-down:
	docker compose down

.PHONY: test-up
test-up:
	docker compose up -d
