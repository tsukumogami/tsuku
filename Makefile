.PHONY: build test clean build-test test-functional test-functional-critical

# Build with dev defaults (.tsuku-dev as home directory)
build:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-dev" -o tsuku ./cmd/tsuku

test:
	go test ./...

clean:
	rm -f tsuku tsuku-test
	rm -rf .tsuku-dev .tsuku-test

# Build test binary with isolated home directory
build-test:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku

# Run functional tests (builds test binary first)
test-functional: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test go test -v ./test/functional/...
	rm -rf .tsuku-test

# Run only critical functional tests
test-functional-critical: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test go test -v ./test/functional/... -godog.tags=@critical
	rm -rf .tsuku-test
