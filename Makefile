.PHONY: build test clean

# Build with dev defaults (.tsuku-dev as home directory)
build:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-dev" -o tsuku ./cmd/tsuku

test:
	go test ./...

clean:
	rm -f tsuku
	rm -rf .tsuku-dev
