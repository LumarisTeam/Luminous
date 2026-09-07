.PHONY: run build clean test

run:
	go run ./cmd/server/

build:
	go build -o bin/luminous ./cmd/server/

clean:
	-rm -rf bin/

test:
	go test ./...

build-mcp:
	go build -o bin/luminous ./cmd/server/
