# Run any go target with GOCACHE=/tmp/oa-gocache if the default build cache is read-only.

.PHONY: build test ci fuzz clean

build:
	go build -o bin/oa ./cmd/oa

test:
	go test ./...

ci:
	@test -z "$$(gofmt -s -l .)" || { echo "gofmt -s needed on:"; gofmt -s -l .; exit 1; }
	go vet ./...
	go test ./...

fuzz:
	go test -run '^$$' -fuzz FuzzParse -fuzztime 10s ./internal/org

clean:
	rm -rf bin
