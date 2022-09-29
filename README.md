# PeerDB Search

[![Go Report Card](https://goreportcard.com/badge/gitlab.com/peerdb/search)](https://goreportcard.com/report/gitlab.com/peerdb/search)
[![pipeline status](https://gitlab.com/peerdb/search/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/peerdb/search/-/pipelines)
[![coverage report](https://gitlab.com/peerdb/search/badges/main/coverage.svg)](https://gitlab.com/peerdb/search/-/graphs/main/charts)

PeerDB Search is an opinionated but flexible open source search system incorporating best practices in search and user
interfaces/experience to provide intuitive, fast, and easy to use search over both full-text data and semantic data
exposed as facets. The goal of the user interface is to allow users without technical knowledge to
easily find results they want, without having to write queries. The system also allows multiple data sources
to be used and merged together.

As a demonstration we provide a search service for Wikipedia articles and Wikidata data at
[https://wikipedia.peerdb.org/](https://wikipedia.peerdb.org/)
(**work in progress**).

## Installation

### Backend

Backend is implemented in Go (requires 1.19 or newer) and provides a HTTP2 API. It requires an ElasticSearch instance.

To run backend locally first start an an ElasticSearch instance:

```sh
docker run -d --name elasticsearch -p 9200:9200 -p 9300:9300 \
 -e network.bind_host=0.0.0.0 -e network.publish_host=127.0.0.1 -e ES_JAVA_OPTS="-Xmx1000m" \
 -e "discovery.type=single-node" -e "xpack.security.enabled=false" -e "ingest.geoip.downloader.enabled=false" \
 elasticsearch:7.16.3
```

Feel free to change any of the above parameters (e.g., remove `ES_JAVA_OPTS` if you have enough memory).
The parameters above are primarily meant for development on a local machine.

Then clone the repository and run:

```sh
make
go install filippo.io/mkcert@latest
mkcert -install
mkcert localhost 127.0.0.1 ::1
./search -d -c localhost+2.pem -k localhost+2-key.pem
```

Backend listens at [https://localhost:8080/](https://localhost:8080/).

[mkcert](https://github.com/FiloSottile/mkcert) is a tool to create a local CA
keypair which is then used to create TLS certificates for development. PeerDB Search
requires a TLS certificate because it uses HTTP2.

`-d` CLI argument makes the backend proxy unknown requests to the frontend.

### Frontend

Frontend is implemented in TypeScript and Vue. Node 16 or newer is required.
To install all dependencies and run frontend for development:

```sh
npm install
npm run serve
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser.

### Wikipedia search

To populate search with Wikipedia articles and Wikidata data, run:

```sh
make
./wikipedia
```

This will do multiple passes:

- `wikidata` downloads Wikidata dump and imports data into search (70 GB download, runtime 2 days).
- `commons-files` populates search with Wikimedia Commons files from images table SQL dump (10 GB download, runtime 1 day).
- `wikipedia-files` populates search with Wikipedia files from table SQL dump (100 MB download, runtime 10 minutes).
- `commons` (20 GB download, runtime 3 days)
- `wikipedia-articles` downloads Wikipedia articles HTML dump and imports articles (100 GB download, runtime 0.5 days)
- `wikipedia-file-descriptions` downloads Wikipedia files HTML dump and imports file descriptions
  (2 GB download, runtime 1 hour)
- `wikipedia-categories` downloads Wikipedia categories HTML dump and imports their articles as descriptions
  (2 GB download, runtime 1 hour)
- `wikipedia-templates` uses API to fetch data about templates Wikipedia (runtime 0.5 days)
- `commons-file-descriptions` uses API to fetch descriptions of Wikimedia Commons files (runtime 35 days)
- `commons-categories` uses API to fetch data about categories Wikimedia Commons (runtime 4 days)
- `commons-templates` uses API to fetch data about templates Wikimedia Commons (runtime 2.5 hours)
- `prepare` goes over imported documents and process them for PeerDB Search (runtime 6 days).
- `optimize` forces merging of ElasticSearch segments (few hours).

The whole process requires substantial amount of disk space (at least 1.5 TB), bandwidth, and time.

### Docker

Instead of compiling backend and frontend yourself, you can use a Docker image, e.g., one
build from the latest `main` branch. The following assumes a TLS certificate has been
generated in the current directory as described above:

```sh
docker run -d --name peerdb-search -p 8080:8080 -v "$(pwd):/code" \
 registry.gitlab.com/peerdb/search/branch/main:latest \
 --elastic=http://elasticsearch:9200 --logging.console.type=json --cert-file=/code/localhost+2.pem --key-file=/code/localhost+2-key.pem
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser.

## GitHub mirror

There is also a [read-only GitHub mirror available](https://github.com/peer/db-search),
if you need to fork the project there.

## Funding

This project was funded through the [NGI0 Discovery Fund](https://nlnet.nl/discovery/), a
fund established by NLnet with financial support from the European Commission's
[Next Generation Internet](https://ngi.eu/) programme, under the aegis of DG Communications
Networks, Content and Technology under grant agreement No 825322.
