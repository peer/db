# PeerDB Search

[![Go Report Card](https://goreportcard.com/badge/gitlab.com/peerdb/search)](https://goreportcard.com/report/gitlab.com/peerdb/search)
[![pipeline status](https://gitlab.com/peerdb/search/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/peerdb/search/-/pipelines)
[![coverage report](https://gitlab.com/peerdb/search/badges/main/coverage.svg)](https://gitlab.com/peerdb/search/-/graphs/main/charts)

PeerDB Search is an opinionated but flexible open source search system incorporating best practices in search and user
interfaces/experience to provide intuitive, fast, and easy to use search over both full-text data and semantic data
exposed as facets. The goal of the user interface is to allow users without technical knowledge to
easily find results they want, without having to write queries. The system also allows multiple data sources
to be used and merged together.

As a demonstration we provide a search service for Wikipedia articles and Wikidata data.

## Installation

### Backend

Backend is implemented in Go and provides a HTTP2 API. It requires an ElasticSearch instance.

To run backend locally first start an an ElasticSearch instance:

```sh
docker run -d --name elasticsearch -p 9200:9200 -p 9300:9300 -e "discovery.type=single-node" -e "xpack.security.enabled=false" elasticsearch:7.16.3
```

Then clone the repository and run:

```sh
make
go install filippo.io/mkcert@latest
mkcert -install
mkcert localhost 127.0.0.1 ::1
./search -c localhost+2.pem -k localhost+2-key.pem
```

This will expose [https://localhost:8080/d](https://localhost:8080/d) search API endpoint.

[mkcert](https://github.com/FiloSottile/mkcert) is a tool to create a local CA
keypair which is then used to create TLS certificates for development. PeerDB Search
requires a TLS certificate because it uses HTTP2.

### Wikipedia search

To populate search with Wikipedia articles and Wikidata data, run:

```sh
make
./wikipedia
```

This will do multiple passes:

* `commons-files` populates search with Wikimedia Commons files from images table SQL dump (10 GB download, runtime 0.5 day).
* `wikipedia-files` populates search with Wikipedia files from table SQL dump (100 MB download, runtime 30 minutes).
* `wikidata` downloads Wikidata dump (70GB) and imports data into search (runtime 3 days).
* `prepare` goes over imported documents and process them for PeerDB Search (runtime 1 day).
* `wikipedia-file-descriptions` downloads Wikipedia files HTML dump (2 GB) and imports file descriptions (runtime 30 minutes)
* `wikipedia-articles` downloads Wikipedia articles HTML dump (100GB) and imports articles (runtime 1 day)

The whole process requires substantial amount of disk space (at least 500 GB), bandwidth, and time.

## GitHub mirror

There is also a [read-only GitHub mirror available](https://github.com/peer/db-search),
if you need to fork the project there.

## Funding

This project was funded through the [NGI0 Discovery Fund](https://nlnet.nl/discovery/), a
fund established by NLnet with financial support from the European Commission's
[Next Generation Internet](https://ngi.eu/) programme, under the aegis of DG Communications
Networks, Content and Technology under grant agreement No 825322.
