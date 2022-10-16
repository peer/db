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

.PHONY: build search wikipedia mapping moma build-static test test-ci lint lint-ci fmt fmt-ci clean release lint-docs audit serve watch

build: search wikipedia mapping moma

# dist is build only if it is missing. Use "make clean" to remove it to build it again.
search wikipedia mapping moma: dist
	go build -ldflags "-X gitlab.com/peerdb/search/internal/cli.Version=${VERSION} -X gitlab.com/peerdb/search/internal/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/peerdb/search/internal/cli.Revision=${REVISION}" -o $@ gitlab.com/peerdb/search/cmd/$@

# dist is build only if it is missing. Use "make clean" to remove it to build it again.
build-static: dist
	go build -ldflags "-linkmode external -extldflags '-static' -X gitlab.com/peerdb/search/internal/cli.Version=${VERSION} -X gitlab.com/peerdb/search/internal/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/peerdb/search/internal/cli.Revision=${REVISION}" -o search gitlab.com/peerdb/search/cmd/search
	go build -ldflags "-linkmode external -extldflags '-static' -X gitlab.com/peerdb/search/internal/cli.Version=${VERSION} -X gitlab.com/peerdb/search/internal/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/peerdb/search/internal/cli.Revision=${REVISION}" -o wikipedia gitlab.com/peerdb/search/cmd/wikipedia
	go build -ldflags "-linkmode external -extldflags '-static' -X gitlab.com/peerdb/search/internal/cli.Version=${VERSION} -X gitlab.com/peerdb/search/internal/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/peerdb/search/internal/cli.Revision=${REVISION}" -o mapping gitlab.com/peerdb/search/cmd/mapping
	go build -ldflags "-linkmode external -extldflags '-static' -X gitlab.com/peerdb/search/internal/cli.Version=${VERSION} -X gitlab.com/peerdb/search/internal/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/peerdb/search/internal/cli.Revision=${REVISION}" -o moma gitlab.com/peerdb/search/cmd/moma

dist:
	npm run build

test:
	gotestsum --format pkgname --packages ./... -- -race -timeout 10m -cover -covermode atomic

test-ci:
	gotestsum --format pkgname --packages ./... --junitfile tests.xml -- -race -timeout 10m -coverprofile=coverage.txt -covermode atomic
	gocover-cobertura < coverage.txt > coverage.xml
	go tool cover -html=coverage.txt -o coverage.html

lint:
	golangci-lint run --timeout 4m --color always --fix

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
	rm -rf coverage.* codeclimate.json tests.xml *.nix coverage dist search wikipedia mapping moma

release:
	npx --yes --package 'release-it@15.4.2' --package '@release-it/keep-a-changelog@3.1.0' -- release-it

lint-docs:
	npx --yes --package 'markdownlint-cli@~0.30.0' -- markdownlint --ignore-path .gitignore --ignore testdata/ '**/*.md'

audit:
	go list -json -deps | nancy sleuth --skip-update-check

watch:
	CompileDaemon -build="make --silent search" -command="./search -d -k localhost+2.pem -K localhost+2-key.pem" -include="*.tmpl" -include="*.json" -include="go.mod" -include="go.sum" -exclude-dir=.git -exclude-dir=.cache -exclude-dir=output -graceful-kill=true -log-prefix=false -color=true
