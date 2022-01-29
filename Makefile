# We use ifeq instead of ?= so that we set variables
# also when they are defined, but empty.
ifeq ($(VERSION),)
 VERSION = `git describe --tags --always --dirty=+`
endif
ifeq ($(BUILD_TIMESTAMP),)
 BUILD_TIMESTAMP = `date -u +%FT%TZ`
endif
ifeq ($(REVISION),)
 REVISION = `git rev-parse HEAD`
endif

.PHONY: build build-static test test-ci lint lint-ci fmt fmt-ci clean lint-docs audit

build:
	go build -ldflags "-X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o wikidata gitlab.com/peerdb/search/cmd/wikidata
	go build -ldflags "-X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o wikipedia gitlab.com/peerdb/search/cmd/wikipedia
	go build -ldflags "-X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o prepare gitlab.com/peerdb/search/cmd/prepare
	go build -ldflags "-X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o search gitlab.com/peerdb/search/cmd/search

build-static:
	go build -ldflags "-linkmode external -extldflags '-static' -X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o wikidata gitlab.com/peerdb/search/cmd/wikidata
	go build -ldflags "-linkmode external -extldflags '-static' -X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o wikipedia gitlab.com/peerdb/search/cmd/wikipedia
	go build -ldflags "-linkmode external -extldflags '-static' -X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o prepare gitlab.com/peerdb/search/cmd/prepare
	go build -ldflags "-linkmode external -extldflags '-static' -X main.version=${VERSION} -X main.buildTimestamp=${BUILD_TIMESTAMP} -X main.revision=${REVISION}" -o search gitlab.com/peerdb/search/cmd/search

test:
	gotestsum --format pkgname --packages ./... -- -race -timeout 10m -cover -covermode atomic

test-ci:
	gotestsum --format pkgname --packages ./... --junitfile tests.xml -- -race -timeout 10m -coverprofile=coverage.txt -covermode atomic
	gocover-cobertura < coverage.txt > coverage.xml
	go tool cover -html=coverage.txt -o coverage.html

lint:
	golangci-lint run --timeout 4m --color always

# TODO: Output both formats at the same time, once it is supported.
# See: https://github.com/golangci/golangci-lint/issues/481
lint-ci:
	-golangci-lint run --timeout 4m --color always
	golangci-lint run --timeout 4m --out-format code-climate > codeclimate.json

fmt:
	go mod tidy
	git ls-files --cached --modified --other --exclude-standard -z | grep -z -Z '.go$$' | xargs -0 gofumpt -w
	git ls-files --cached --modified --other --exclude-standard -z | grep -z -Z '.go$$' | xargs -0 goimports -w -local gitlab.com/peerdb/search

fmt-ci: fmt
	git diff --exit-code --color=always

clean:
	rm -f coverage.* codeclimate.json tests.xml wikidata wikipedia prepare search

lint-docs:
	npx --yes --package 'markdownlint-cli@~0.30.0' -- markdownlint --ignore-path .gitignore --ignore testdata/ '**/*.md'

audit:
	go list -json -deps | nancy sleuth --skip-update-check
