SHELL = /usr/bin/env bash -o pipefail

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

.PHONY: build peerdb wikipedia mapping moma products build-static test test-ci lint lint-ci fmt fmt-ci upgrade clean release lint-docs lint-docs-ci audit watch

build: peerdb wikipedia mapping moma products

# dist is build only if it is missing. Use "make clean" to remove it to build it again.
peerdb: dist
	go build -trimpath -ldflags "-s -w -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o $@ gitlab.com/peerdb/peerdb/cmd/$@

wikipedia mapping moma products:
	go build -trimpath -ldflags "-s -w -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o $@ gitlab.com/peerdb/peerdb/cmd/$@

# dist is build only if it is missing. Use "make clean" to remove it to build it again.
build-static: dist
	go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o peerdb gitlab.com/peerdb/peerdb/cmd/peerdb
	go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o wikipedia gitlab.com/peerdb/peerdb/cmd/wikipedia
	go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o mapping gitlab.com/peerdb/peerdb/cmd/mapping
	go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o moma gitlab.com/peerdb/peerdb/cmd/moma
	go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o products gitlab.com/peerdb/peerdb/cmd/products

dist: node_modules src vite.config.ts tsconfig.json tsconfig.node.json tailwind.config.js LICENSE
	npm run build

node_modules:
	npm install

dist/index.html:
	mkdir -p dist
	if [ ! -e dist/index.html ]; then echo "<html><body>dummy content</body></html>" > dist/index.html; fi

test: dist/index.html
	gotestsum --format pkgname --packages ./... -- -race -timeout 10m -cover -covermode atomic -coverpkg ./...

# GitLab CI uses the last coverage which matches the regexp, so we use jq to obtain it and print it last.
# This might not be necessary if we could define the order of packages in the output and we could have the main package be the last.
# See: https://github.com/gotestyourself/gotestsum/issues/392
# This might not be necessary if "go tool covdata percent" could be used with --coverpkg and output the same value for the main package
# as it is reported during testing.
# See: https://github.com/golang/go/issues/66464
# TODO: Correctly compute total coverage across all packages and not use the main package's coverage.
#       See: https://github.com/golang/go/issues/66506
test-ci: dist/index.html
	gotestsum --format pkgname --jsonfile tests.json --packages ./... --junitfile tests.xml -- -race -timeout 10m -coverprofile=coverage.txt -covermode atomic -coverpkg ./...
	gocover-cobertura < coverage.txt > coverage.xml
	go tool cover -html=coverage.txt -o coverage.html
	jq -r '. | select(.Action == "output") | select(.Package == "gitlab.com/peerdb/peerdb") | select(.Output | startswith("coverage")) | .Output' tests.json

lint: dist/index.html
	golangci-lint run --timeout 4m --color always --allow-parallel-runners --fix --max-issues-per-linter 0 --max-same-issues 0

lint-ci: dist/index.html
	golangci-lint run --timeout 4m --max-issues-per-linter 0 --max-same-issues 0 --out-format colored-line-number,code-climate:codeclimate.json

fmt:
	go mod tidy
	git ls-files --cached --modified --other --exclude-standard -z | grep -z -Z '.go$$' | xargs -0 gofumpt -w
	git ls-files --cached --modified --other --exclude-standard -z | grep -z -Z '.go$$' | xargs -0 goimports -w -local gitlab.com/peerdb/peerdb

fmt-ci: fmt
	git diff --exit-code --color=always

upgrade:
	go run github.com/icholy/gomajor@v0.13.2 get all
	go mod tidy

clean:
	rm -rf coverage.* codeclimate.json tests.xml tests.json coverage dist peerdb wikipedia mapping moma products

release:
	npx --yes --package 'release-it@15.4.2' --package '@release-it/keep-a-changelog@3.1.0' -- release-it

lint-docs:
	npx --yes --package 'markdownlint-cli@~0.41.0' -- markdownlint --ignore-path .gitignore --ignore testdata/ --fix '**/*.md'

lint-docs-ci: lint-docs
	git diff --exit-code --color=always

audit: dist/index.html
	go list -json -deps ./... | nancy sleuth --skip-update-check

watch:
	CompileDaemon -build="make --silent peerdb" -command="./peerdb -D -k localhost+2.pem -K localhost+2-key.pem" -include="*.tmpl" -include="*.json" -include="go.mod" -include="go.sum" -exclude-dir=.git -exclude-dir=.cache -exclude-dir=output -graceful-kill=true -log-prefix=false -color=true
