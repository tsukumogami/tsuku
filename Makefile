.PHONY: build test clean build-test test-functional test-functional-critical

# Build with dev defaults (.tsuku-dev as home directory)
# CGO_ENABLED=0 produces a static binary that works in all Linux containers
# (including Alpine/musl). CI and .goreleaser.yaml already use this setting.
build:
	go generate ./internal/containerimages/...
	CGO_ENABLED=0 go build -ldflags "-X main.defaultHomeOverride=.tsuku-dev" -o tsuku ./cmd/tsuku

test:
	go test ./...

clean:
	rm -f tsuku tsuku-test
	rm -rf .tsuku-dev .tsuku-test

# Build test binary with isolated home directory
build-test:
	CGO_ENABLED=0 go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku

# Run functional tests (builds test binary first)
test-functional: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test go test -v ./test/functional/...
	rm -rf .tsuku-test

# Run only critical functional tests
test-functional-critical: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test TSUKU_TEST_TAGS=@critical go test -v ./test/functional/...
	rm -rf .tsuku-test
