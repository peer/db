# PeerDB Search

[![Go Report Card](https://goreportcard.com/badge/gitlab.com/peerdb/search)](https://goreportcard.com/report/gitlab.com/peerdb/search)
[![pipeline status](https://gitlab.com/peerdb/search/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/peerdb/search/-/pipelines)
[![coverage report](https://gitlab.com/peerdb/search/badges/main/coverage.svg)](https://gitlab.com/peerdb/search/-/graphs/main/charts)

PeerDB Search is opinionated open source search system incorporating best practices in search and user
interfaces/experience to provide an intuitive, fast, and easy to use search over semantic, structured, and full-text data.
Its user interface automatically adapts to data and search results and provides relevant filters. The goal of the user
interface is to allow users without technical knowledge to easily find results they want, without having to write queries.
The system also allows multiple data sources to be used and merged together.

As a demonstration we provide a search service for English Wikipedia articles, Wikimedia Commons files,
and Wikidata data at [https://wikipedia.peerdb.org/](https://wikipedia.peerdb.org/).

## Installation

You can run PeerDB Search behind a reverse proxy (which should support HTTP2), or simply run it directly
(it is safe to do so). PeerDB Search is compiled into one backend binary which has frontend files embedded
and they are served by the backend as well.

[Releases page](https://gitlab.com/peerdb/search/-/releases)
contains a list of stable versions. Each includes:

* A statically compiled binary.
* Docker images.
* A nix package.

### Static binary

The latest stable statically compiled binary for Linux (amd64) is available at:

[`https://gitlab.com/peerdb/search/-/releases/permalink/latest/downloads/linux-amd64/peerdb-search`](https://gitlab.com/peerdb/search/-/releases/permalink/latest/downloads/linux-amd64/peerdb-search)

You can also download older versions on [releases page](https://gitlab.com/peerdb/search/-/releases).

The latest successfully built development (`main` branch) binary is available at:

[`https://gitlab.com/peerdb/search/-/jobs/artifacts/main/raw/peerdb-search-linux-amd64?job=docker`](https://gitlab.com/peerdb/search/-/jobs/artifacts/main/raw/peerdb-search-linux-amd64?job=docker)

### Docker

Docker images for stable versions are available as:

`registry.gitlab.com/peerdb/search/tag/<version>:latest`

`<version>` is a version string with `.` replaced with `-`. E.g., `v0.1.0` becomes `v0-1-0`.

The docker image contains only PeerDB Search binary, which is image's entrypoint.
If you need a shell as well, then use the debug version of the image:

`registry.gitlab.com/peerdb/search/tag/<version>:latest-debug`

In that case you have to override the entrypoint (i.e., `--entrypoint sh` argument to `docker run`).

The latest successfully built development (`main` branch) image is available as:

`registry.gitlab.com/peerdb/search/branch/main:latest`

generated in the current directory as described above:

### Nix

You can build a binary yourself using [Nix](https://nixos.org/). For the latest stable version, run:

```sh
nix-build -E "with import <nixpkgs> { }; callPackage (import (fetchTarball https://gitlab.com/peerdb/search/-/releases/permalink/latest/downloads/nix/nix.tgz)) { }"
```

The built binary is available at `./result/bin/search`.

If you download a `nix.tgz` file for an [older version](https://gitlab.com/peerdb/search/-/releases),
you can build the binary with:

```sh
nix-build -E "with import <nixpkgs> { }; callPackage (import (fetchTarball file://$(pwd)/nix.tgz)) { }"
```

To build the latest development (`main` branch) binary, run:

```sh
nix-build -E "with import <nixpkgs> { }; callPackage (import (fetchTarball https://gitlab.com/peerdb/search/-/jobs/artifacts/main/raw/nix.tgz?job=nix)) { }"
```

## Usage

PeerDB Search requires an ElasticSearch instance. To run one locally you can use Docker:

```sh
docker network create peerdb
docker run -d --network peerdb --name elasticsearch -p 127.0.0.1:9200:9200 \
 -e network.bind_host=0.0.0.0 -e network.publish_host=elasticsearch -e ES_JAVA_OPTS="-Xmx1000m" \
 -e "discovery.type=single-node" -e "xpack.security.enabled=false" -e "ingest.geoip.downloader.enabled=false" \
 elasticsearch:7.16.3
```

Feel free to change any of the above parameters (e.g., remove `ES_JAVA_OPTS` if you have enough memory).
The parameters above are primarily meant for development on a local machine.

Next, to run PeerDB Search you need a HTTPS TLS certificate (as required by HTTP2). When running locally
you can use [mkcert](https://github.com/FiloSottile/mkcert), a tool to create a local CA
keypair which is then used to create a TLS certificate.

```sh
go install filippo.io/mkcert@latest
mkcert -install
mkcert localhost 127.0.0.1 ::1
```

This creates two files, `localhost+2.pem` and `localhost+2-key.pem`, which you can provide to PeerDB Search as:

```sh
./search -c localhost+2.pem -k localhost+2-key.pem
```

When running using Docker, you have to provide them to the container through a volume, e.g.:

```sh
docker run -d --network peerdb --name peerdb-search -p 8080:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/search/branch/main:latest -e http://elasticsearch:9200 \
 -c /data/localhost+2.pem -k /data/localhost+2-key.pem
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser to access the web interface.

When running it directly publicly on the Internet (it is safe to do so), PeerDB Search is able to
obtain a HTTPS TLS certificate from [Let's Encrypt](https://letsencrypt.org) automatically:

```sh
docker run -d --network peerdb --name peerdb-search -p 443:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/search/branch/main:latest -e http://elasticsearch:9200 \
 -D public.domain.example.com -E name@example.com -C /data/letsencrypt
```

PeerDB Search would then be available at `https://public.domain.example.com`.

When using Let's Encrypt you accept its Terms of Service.

## Populating with data

### Wikipedia search

To populate search with English Wikipedia articles, Wikimedia Commons files, and Wikidata data,
clone the repository and run (you need Go 1.19 or newer):

```sh
make
./wikipedia
```

This will do multiple passes:

* `wikidata` downloads Wikidata dump and imports data into search (70 GB download, runtime 2 days).
* `commons-files` populates search with Wikimedia Commons files from images table SQL dump (10 GB download, runtime 1 day).
* `wikipedia-files` populates search with Wikipedia files from table SQL dump (100 MB download, runtime 10 minutes).
* `commons` (20 GB download, runtime 3 days)
* `wikipedia-articles` downloads Wikipedia articles HTML dump and imports articles (100 GB download, runtime 0.5 days)
* `wikipedia-file-descriptions` downloads Wikipedia files HTML dump and imports file descriptions
  (2 GB download, runtime 1 hour)
* `wikipedia-categories` downloads Wikipedia categories HTML dump and imports their articles as descriptions
  (2 GB download, runtime 1 hour)
* `wikipedia-templates` uses API to fetch data about templates Wikipedia (runtime 0.5 days)
* `commons-file-descriptions` uses API to fetch descriptions of Wikimedia Commons files (runtime 35 days)
* `commons-categories` uses API to fetch data about categories Wikimedia Commons (runtime 4 days)
* `commons-templates` uses API to fetch data about templates Wikimedia Commons (runtime 2.5 hours)
* `prepare` goes over imported documents and process them for PeerDB Search (runtime 6 days).
* `optimize` forces merging of ElasticSearch segments (runtime few hours).

The whole process requires substantial amount of disk space (at least 1.5 TB), bandwidth, and time (weeks).
Because of this you might want to run only a subset of passes.

To populate only with Wikidata (all references to Wikimedia Commons files will not be available):

```sh
./wikipedia wikidata
./wikipedia prepare
```

To populate with Wikidata and with basic metadata of Wikimedia Commons files:

```sh
./wikipedia wikidata
./wikipedia commons-files
./wikipedia prepare
```

Or maybe you also want English Wikipedia articles:

```sh
./wikipedia wikidata
./wikipedia commons-files
./wikipedia wikipedia-articles
./wikipedia prepare
```

## Development

During PeerDB Search development run backend and frontend as separate processes. During development the backend
proxies frontend requests to Vite, which in turn compiles frontend files and serves them, hot-reloading
the frontend as necessary.

### Backend

The backend is implemented in [Go](https://golang.org/) (requires 1.19 or newer)
and provides a HTTP2 API.

Then clone the repository and run:

```sh
make
./search -d -c localhost+2.pem -k localhost+2-key.pem
```

`localhost+2.pem` and `localhost+2-key.pem` are files of a TLS certificate
generated as described in the [Usage section](#usage).
Backend listens at [https://localhost:8080/](https://localhost:8080/).

`-d` CLI argument makes the backend proxy unknown requests (non-API requests)
to the frontend.

You can also run `make watch` to reload the backend on file changes. You have to install
[CompileDaemon](https://github.com/githubnemo/CompileDaemon) first:

```sh
go install github.com/githubnemo/CompileDaemon@latest
```

### Frontend

The frontend is implemented in [TypeScript](https://www.typescriptlang.org/) and
[Vue](https://vuejs.org/) and during development we use [Vite](https://vitejs.dev/).
Vite compiles frontend files and serves them. It also watches for changes in frontend files,
recompiles them, and hot-reloads the frontend as necessary. Node 16 or newer is required.

To install all dependencies and run frontend for development:

```sh
npm install
npm run serve
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser, which will connect
you to the backend which then proxies unknown requests (non-API requests) to the frontend.

## GitHub mirror

There is also a [read-only GitHub mirror available](https://github.com/peer/db-search),
if you need to fork the project there.

## Funding

This project was funded through the [NGI0 Discovery Fund](https://nlnet.nl/discovery/), a
fund established by NLnet with financial support from the European Commission's
[Next Generation Internet](https://ngi.eu/) programme, under the aegis of DG Communications
Networks, Content and Technology under grant agreement No 825322.
