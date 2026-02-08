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

.PHONY: build peerdb wikipedia mapping moma products build-static lib test test-ci lint lint-ci fmt fmt-ci upgrade clean release lint-docs lint-docs-ci audit encrypt decrypt sops watch

build: peerdb wikipedia mapping moma products

# dist is built only if it is missing. Use "make clean" to remove it to build it again.
peerdb: dist
	go build -trimpath -ldflags "-s -w -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o $@ gitlab.com/peerdb/peerdb/cmd/$@

wikipedia mapping moma products:
	go build -trimpath -ldflags "-s -w -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o $@ gitlab.com/peerdb/peerdb/cmd/$@

# dist is built only if it is missing. Use "make clean" to remove it to build it again.
build-static: dist
	go build $(PEERDB_BUILD_FLAGS) -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o peerdb gitlab.com/peerdb/peerdb/cmd/peerdb
	go build $(PEERDB_BUILD_FLAGS) -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o wikipedia gitlab.com/peerdb/peerdb/cmd/wikipedia
	go build $(PEERDB_BUILD_FLAGS) -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o mapping gitlab.com/peerdb/peerdb/cmd/mapping
	go build $(PEERDB_BUILD_FLAGS) -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o moma gitlab.com/peerdb/peerdb/cmd/moma
	go build $(PEERDB_BUILD_FLAGS) -trimpath -ldflags "-s -w -linkmode external -extldflags '-static' -X gitlab.com/tozd/go/cli.Version=${VERSION} -X gitlab.com/tozd/go/cli.BuildTimestamp=${BUILD_TIMESTAMP} -X gitlab.com/tozd/go/cli.Revision=${REVISION}" -o products gitlab.com/peerdb/peerdb/cmd/products

dist: node_modules src vite.config.ts tsconfig.json tsconfig.node.json LICENSE
	npm run build

lib: node_modules src vite.config.lib.ts tsconfig.json tsconfig.node.json LICENSE
	npm run build-lib

node_modules: package-lock.json

package-lock.json: package.json
	npm install

test:
	gotestsum --format pkgname --packages ./... -- -race -timeout 10m -cover -covermode atomic

test-ci:
	gotestsum --format pkgname --packages ./... --junitfile tests.xml -- -race -timeout 10m -coverprofile=coverage.txt -covermode atomic
	gocover-cobertura < coverage.txt > coverage.xml
	go tool cover -html=coverage.txt -o coverage.html

lint:
	golangci-lint run --output.text.colors --allow-parallel-runners --fix

lint-ci:
	golangci-lint run --output.text.path=stdout --output.code-climate.path=codeclimate.json

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
	rm -rf coverage.* codeclimate.json tests.xml coverage dist lib peerdb wikipedia mapping moma products

release:
	npx --yes --package 'release-it@19.0.5' --package '@release-it/keep-a-changelog@7.0.0' -- release-it

lint-docs:
	npx --yes --package 'markdownlint-cli@~0.45.0' -- markdownlint --ignore-path .gitignore --ignore testdata/ --fix '**/*.md'

lint-docs-ci: lint-docs
	git diff --exit-code --color=always

audit:
	go list -json -deps ./... | nancy sleuth --skip-update-check

encrypt:
	gitlab-config sops --encrypt --mac-only-encrypted --in-place --encrypted-comment-regex sops:enc .gitlab-conf.yml

decrypt:
	SOPS_AGE_KEY_FILE=keys.txt gitlab-config sops --decrypt --in-place .gitlab-conf.yml

sops:
	SOPS_AGE_KEY_FILE=keys.txt gitlab-config sops .gitlab-conf.yml

watch:
	CompileDaemon -build="make --silent peerdb" -command="./peerdb -D -k localhost+2.pem -K localhost+2-key.pem" -include="*.tmpl" -include="*.json" -include="go.mod" -include="go.sum" -exclude-dir=.git -exclude-dir=.cache -exclude-dir=output -graceful-kill=true -log-prefix=false -color=true
