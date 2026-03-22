# PeerDB

[![pkg.go.dev](https://pkg.go.dev/badge/gitlab.com/peerdb/peerdb)](https://pkg.go.dev/gitlab.com/peerdb/peerdb)
[![Go Report Card](https://goreportcard.com/badge/gitlab.com/peerdb/peerdb)](https://goreportcard.com/report/gitlab.com/peerdb/peerdb)
[![pipeline status](https://gitlab.com/peerdb/peerdb/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/peerdb/peerdb/-/pipelines)
[![coverage report](https://gitlab.com/peerdb/peerdb/badges/main/coverage.svg)](https://gitlab.com/peerdb/peerdb/-/graphs/main/charts)

PeerDB a database software layer which supports different types of collaboration out of the box.
Build collaborative applications like you would build traditional applications and leave it to
PeerDB to take care of collaboration.
Common user interface components are included.

Demos:

- [wikipedia.peerdb.org](https://wikipedia.peerdb.org/): a search service for English Wikipedia articles,
  Wikimedia Commons files, and Wikidata data (its [repository](https://gitlab.com/peerdb/wikipedia)).
- [moma.peerdb.org](https://moma.peerdb.org/): a search service for The
  Museum of Modern Art (MoMA) artists and artworks (its [repository](https://gitlab.com/peerdb/moma)).

## Components

- PeerDB Store is a key-value store with features enabling collaboration. It supports versioning, forking and merging,
  transactions at human-scale (i.e., to support merge requests), concurrent different views over the data.
- PeerDB Coordinator provides an append-only log of operations to support synchronizing real-time collaboration sessions.
- PeerDB Storage provides file storage with versioning and interactive uploads.
- PeerDB Search is a search for semantic, structured, and full-text data. It is opinionated open source search system incorporating
  best practices in search and user interfaces/experience to provide an intuitive, fast, and easy to use search over semantic,
  structured, and full-text data. Its user interface automatically adapts to data and search results and provides relevant
  filters. The goal of the user interface is to allow users without technical knowledge to easily find results they want,
  without having to write queries.

## Installation

You can run PeerDB behind a reverse proxy (which should support HTTP2), or simply run it directly
(it is safe to do so). PeerDB is compiled into one backend binary which has frontend files embedded
and they are served by the backend as well.

The [releases page](https://gitlab.com/peerdb/peerdb/-/releases)
contains a list of stable versions. Each includes:

- A statically compiled binary.
- Docker images.

### Static binary

The latest stable statically compiled binary for Linux (amd64) is available at:

[`https://gitlab.com/peerdb/peerdb/-/releases/permalink/latest/downloads/linux-amd64/peerdb`](https://gitlab.com/peerdb/peerdb/-/releases/permalink/latest/downloads/linux-amd64/peerdb)

You can also download older versions on the [releases page](https://gitlab.com/peerdb/peerdb/-/releases).

The latest successfully built development (`main` branch) binary is available at:

[`https://gitlab.com/peerdb/peerdb/-/jobs/artifacts/main/raw/peerdb-linux-amd64?job=docker`](https://gitlab.com/peerdb/peerdb/-/jobs/artifacts/main/raw/peerdb-linux-amd64?job=docker)

### Docker

Docker images for stable versions are available as:

`registry.gitlab.com/peerdb/peerdb/tag/<version>:latest`

`<version>` is a version string with `.` replaced with `-`. E.g., `v0.1.0` becomes `v0-1-0`.

The docker image contains only PeerDB binary, which is image's entrypoint.
If you need a shell as well, then use the debug version of the image:

`registry.gitlab.com/peerdb/peerdb/tag/<version>:latest-debug`

In that case you have to override the entrypoint (i.e., `--entrypoint sh` argument to `docker run`).

The latest successfully built development (`main` branch) image is available as:

`registry.gitlab.com/peerdb/peerdb/branch/main:latest`

generated in the current directory as described above:

## Usage

PeerDB requires a [PostgreSQL](https://www.postgresql.org/) database. Using Docker, you can run:

```sh
docker network create peerdb
docker run -d --network peerdb --name pgsql -p 127.0.0.1:5432:5432 \
 -e LOG_TO_STDOUT=1 -e PGSQL_ROLE_1_USERNAME=test -e PGSQL_ROLE_1_PASSWORD=test -e PGSQL_DB_1_NAME=test -e PGSQL_DB_1_OWNER=test \
 registry.gitlab.com/tozd/docker/postgresql:18
```

Create also a file with PostgreSQL secret:

```sh
echo "postgres://test:test@127.0.0.1:5432/test" > .postgresql.secret
export POSTGRES_URL_PATH=.postgresql.secret
```

PeerDB requires an ElasticSearch instance. To run one locally you can use Docker:

```sh
docker run -d --network peerdb --name elasticsearch -p 127.0.0.1:9200:9200 \
 -e network.bind_host=0.0.0.0 -e network.publish_host=elasticsearch -e ES_JAVA_OPTS="-Xmx1000m" \
 -e "discovery.type=single-node" -e "xpack.security.enabled=false" -e "ingest.geoip.downloader.enabled=false" \
 -e "cluster.routing.allocation.disk.watermark.flood_stage=100%" \
 registry.gitlab.com/peerdb/peerdb/elastic/7.17.9:latest
```

Feel free to change any of the above parameters (e.g., remove `ES_JAVA_OPTS` if you have enough memory).
The parameters above are primarily meant for development on a local machine. ElasticSearch Docker image
used above is standard Docker image [with additional plugins installed](./elastic.dockerfile).

Next, to run PeerDB you need a HTTPS TLS certificate (as required by HTTP2). When running locally
you can use [mkcert](https://github.com/FiloSottile/mkcert), a tool to create a local CA
keypair which is then used to create a TLS certificate.

```sh
go install filippo.io/mkcert@latest
mkcert -install
mkcert localhost 127.0.0.1 ::1
```

This creates two files, `localhost+2.pem` and `localhost+2-key.pem`, which you can provide to PeerDB as:

```sh
./peerdb -k localhost+2.pem -K localhost+2-key.pem
```

When running using Docker, you have to provide them (and `.postgresql.secret` file)
to the container through a volume, e.g.:

```sh
docker run -d --network peerdb --name peerdb -p 8080:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/peerdb/branch/main:latest -e http://elasticsearch:9200 \
 -k /data/localhost+2.pem -K /data/localhost+2-key.pem \
 -d /data/.postgresql.secret
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser to access the web interface.

Temporary accepted self-signed certificates are not recommended because
[not all browser features work](https://stackoverflow.com/questions/74161355/are-any-web-features-not-available-in-browsers-when-using-self-signed-certificat).
If you want to use a self-signed certificate instead of `mkcert`, add the certificate to
your browser's certificate store.

When running it directly publicly on the Internet (it is safe to do so), PeerDB is able to
obtain a HTTPS TLS certificate from [Let's Encrypt](https://letsencrypt.org) automatically:

```sh
docker run -d --network peerdb --name peerdb -p 443:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/peerdb/branch/main:latest -e http://elasticsearch:9200 \
 -d /data/.postgresql.secret \
 --domain public.domain.example.com -E name@example.com -C /data/letsencrypt
```

PeerDB would then be available at `https://public.domain.example.com`.

When using Let's Encrypt you accept its Terms of Service.

## Populating with data

Power of PeerDB comes from having data organized into documents in PeerDB schema.
The schema is designed to allow describing almost any data. Moreover, data and properties to describe data can be changed
at runtime without having to reconfigure PeerDB, e.g., it adapts search filters automatically.
The schema also allows multiple data sources to be used and merged together.

At a high-level PeerDB documents look like:

```json
{
  "id": "22 character ID",
  "base": ["example.com", "base for computing ID"],
  "claims": {
    "id": [
      {
        "id": "22 character ID",
        "confidence": 1.0,
        "prop": {
          "id": "22 character property ID"
        },
        "value": "external ID value"
      }
    ],
    "string": [...],
    "html": [...],
    "amount": [...],
    "amountInterval": [...],
    "time": [...],
    "timeInterval": [...],
    "ref": [...],
    "rel": [...],
    "has": [...],
    "none": [...],
    "unknown": [...]
  }
}
```

Besides core metadata (`id` and `base`) all other data is organized
in claims (seen under `claims` above) which are then organized based on claim
(data) type. For example, there are `id` claims which are used to store external
ID values. `prop` is a reference to a property document which describes the ID value.

Which properties you use and how you use them to map your data to PeerDB documents
is left to you. We do suggest that you first populate the index using core PeerDB
properties. You can do that by running:

```sh
./peerdb populate
```

This also creates an ElasticSearch index if it does not yet exist and configures it
with PeerDB ElasticSearch mapping. Otherwise you have to create such index
yourself.

Then you populate the index with documents for properties for you data. For example, if you
have a blog post like:

```json
{
  "id": 123,
  "title": "Some title",
  "body": "Some <b>blog post</b> body HTML",
  "author": {
    "username": "foobar",
    "displayName": "Foo Bar"
  }
}
```

To convert the blog post:

- You could create two documents, one for `title` property and another for `body` property.
  But you could also decide to map `title` to existing `TITLE` core property, and `body` to
  existing `DESCRIPTION` core property.
- Maybe you also want to record the original blog post ID and author `username`,
  so create property documents for them as well.
- Author's `displayName` can be mapped to `NAME` core property.
- Another property document is needed for the `author` property.
- Documents should also have additional claims to describe relations between them.
  Properties should be marked as instances of the `PROPERTY` class.
  It is useful to create classes for user documents, in this case blog posts,
  which can in turn be subclasses of the `DOCUMENT` core class.

Assuming that the author does not yet have its document, you could convert the above blog
post into the following two PeerDB documents:

```json
{
  "id": "XCunBDJGYmhiC7roqLu5dU",
  "base": ["example.com", "1"],
  "claims": {
    "string": [
      {
        "id": "YWYAgufBtjegbQc512GFci",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "Foo Bar"
      }
    ],
    "id": [
      {
        "id": "9TfczNe5aa4LrKqWQWfnFF",
        "confidence": 1.0,
        "prop": {
          "id": "KLE8hEjuUpxS1ZMtM44cUo" // author username
        },
        "value": "foobar"
      }
    ],
    "rel": [
      {
        "id": "sgpzxwxPyn51j5VfH992ZQ",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "Xx51U4FnVQ7E63fJKnRgrG" // user
        }
      }
    ]
  }
}
```

```json
{
  "id": "HBqKdb2iTgqj6ji7NLy8Y3",
  "base": ["example.com", "2"],
  "claims": {
    "string": [
      {
        "id": "KjwGqHqihAQEgNabdNQSNc",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "Some title"
      }
    ],
    "html": [
      {
        "id": "VdX1HZm1ETw8K77nLTV6yt",
        "confidence": 1.0,
        "prop": {
          "id": "QduJFqUu12297s4F62fyoc" // DESCRIPTION
        },
        "html": "Some <b>blog post</b> body HTML"
      }
    ],
    "id": [
      {
        "id": "Ci3A1tLF6MHZ4y5zBibvGg",
        "confidence": 1.0,
        "prop": {
          "id": "1DP4FkpdgJ9jLvTfPhDJqr" // blog post ID
        },
        "value": "123"
      }
    ],
    "rel": [
      {
        "id": "xbufQEChDXvtg3hh4i1PvT",
        "confidence": 1.0,
        "prop": {
          "id": "72J8Hd39A18GNs4aTqS3mw" // author
        },
        "to": {
          "id": "XCunBDJGYmhiC7roqLu5dU" // Foo Bar
        }
      },
      {
        "id": "LJNg7QaiMxE1crjMiijpaN",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "XQHttenpK8ywz3ZyPU5akH" // blog post
        }
      }
    ]
  }
}
```

<details>
<summary>Some additional documents need to exist in the index as well. Click to expand to see them.</summary>

```json
{
  "id": "KLE8hEjuUpxS1ZMtM44cUo",
  "base": ["example.com", "AUTHOR_USERNAME"],
  "claims": {
    "string": [
      {
        "id": "c4xGoqaYKaFNqy6hD7RgV8",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "author username"
      }
    ],
    "rel": [
      {
        "id": "5zZZ6nJFKuA5oNBu9QbsYY",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "V9ZHft2KVKA6qGAv6g8enE" // PROPERTY
        }
      }
    ]
  }
}
```

```json
{
  "id": "1DP4FkpdgJ9jLvTfPhDJqr",
  "base": ["example.com", "BLOG_POST_ID"],
  "claims": {
    "string": [
      {
        "id": "Bj3wKeu49j8ncXYpZ7tuZ5",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "blog post ID"
      }
    ],
    "rel": [
      {
        "id": "DWYDFZ2DbasS4Tyehnko2U",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "V9ZHft2KVKA6qGAv6g8enE" // PROPERTY
        }
      }
    ]
  }
}
```

```json
{
  "id": "72J8Hd39A18GNs4aTqS3mw",
  "base": ["example.com", "AUTHOR"],
  "claims": {
    "string": [
      {
        "id": "1NH97QxtHqJS6JAz1hzCxo",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "author"
      }
    ],
    "rel": [
      {
        "id": "gK8nXxJ3AXErTmGPoAVF78",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "V9ZHft2KVKA6qGAv6g8enE" // PROPERTY
        }
      }
    ]
  }
}
```

```json
{
  "id": "Xx51U4FnVQ7E63fJKnRgrG",
  "base": ["example.com", "USER"],
  "claims": {
    "string": [
      {
        "id": "cVvsHG2ru2ojRJhV27Zj3E",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "user"
      }
    ],
    "rel": [
      {
        "id": "79m7fNMHy7SRinSmB3WARM",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "SdQgTgtcqTm7xHV4PV1oAd" // CLASS
        }
      },
      {
        "id": "EuGC7ZRK7weuHNoZLDC8ah",
        "confidence": 1.0,
        "prop": {
          "id": "UrBttJgHvm7kbFe7X9WcS1" // SUBCLASS_OF
        },
        "to": {
          "id": "MneSnmJvyYd9u7RKzhyS5p" // DOCUMENT
        }
      }
    ]
  }
}
```

```json
{
  "id": "XQHttenpK8ywz3ZyPU5akH",
  "base": ["example.com", "BLOG_POST"],
  "claims": {
    "string": [
      {
        "id": "961QCaiVomNHjjwRKtNejK",
        "confidence": 1.0,
        "prop": {
          "id": "K746ZT8FqJeMvm1WfLBNUH" // NAME
        },
        "string": "blog post"
      }
    ],
    "rel": [
      {
        "id": "c24VwrPEMwZUhRgECzSn1b",
        "confidence": 1.0,
        "prop": {
          "id": "NkeB6fpAmk6awysbH77n8H" // INSTANCE_OF
        },
        "to": {
          "id": "SdQgTgtcqTm7xHV4PV1oAd" // CLASS
        }
      },
      {
        "id": "faByPL18Y1ZH2NAhea4FBy",
        "confidence": 1.0,
        "prop": {
          "id": "UrBttJgHvm7kbFe7X9WcS1" // SUBCLASS_OF
        },
        "to": {
          "id": "MneSnmJvyYd9u7RKzhyS5p" // DOCUMENT
        }
      }
    ]
  }
}
```

</details>

## Configuration

PeerDB can be configured through CLI arguments and a config file. CLI arguments have precedence
over the config file. Config file is a YAML file with the structure corresponding to the structure of
CLI flags and commands and is defined by the [`Config`](https://pkg.go.dev/gitlab.com/peerdb/peerdb#Config)
struct. Run `./peerdb --help` for list of available flags and commands. If no command is
specified, `serve` command is the default. Each PeerDB instance can serve multiple sites and Let's
Encrypt can be used to obtain HTTPS TLS certificates for them automatically.

To use the config file with Docker, you could do:

```sh
docker run -d --network peerdb --name peerdb -p 443:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/peerdb/branch/main:latest -e http://elasticsearch:9200 \
 -E name@example.com -C /data/letsencrypt -c /data/config.yml
```

## Development

During PeerDB development run backend and frontend as separate processes. During development the backend
proxies frontend requests to Vite, which in turn compiles frontend files and serves them, hot-reloading
the frontend as necessary.

### Backend

The backend is implemented in [Go](https://golang.org/) (requires 1.25 or newer)
and provides a HTTP2 API. Node 24 or newer is required as well.

Automatic media type detection uses file extensions and a file extension database has
to be available on the system.
On Alpine this can be `mailcap` package.
On Debian/Ubuntu `media-types` package.

Then clone the repository and run:

```sh
make peerdb
./peerdb -D -k localhost+2.pem -K localhost+2-key.pem
```

`localhost+2.pem` and `localhost+2-key.pem` are files of a TLS certificate
generated as described in the [Usage section](#usage).
Backend listens at [https://localhost:8080/](https://localhost:8080/).

`-D` CLI flag makes the backend proxy unknown requests (non-API requests)
to the frontend. In this mode any placeholders in HTML files are not rendered.

You can also run `make watch` to reload the backend on file changes. You have to install
[CompileDaemon](https://github.com/githubnemo/CompileDaemon) first:

```sh
go install github.com/githubnemo/CompileDaemon@latest
```

### Frontend

The frontend is implemented in [TypeScript](https://www.typescriptlang.org/) and
[Vue](https://vuejs.org/) and during development we use [Vite](https://vitejs.dev/).
Vite compiles frontend files and serves them. It also watches for changes in frontend files,
recompiles them, and hot-reloads the frontend as necessary.

To install all dependencies and run frontend for development:

```sh
npm install
npm run serve
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser, which will connect
you to the backend which then proxies unknown requests (non-API requests) to the frontend.

## GitHub mirror

There is also a [read-only GitHub mirror available](https://github.com/peer/db),
if you need to fork the project there.

## Acknowledgements

This project was funded through the [NGI0 Discovery Fund](https://nlnet.nl/discovery/), a
fund established by [NLnet](https://nlnet.nl) with financial support from the European Commission's
[Next Generation Internet](https://ngi.eu/) programme, under the aegis of DG Communications
Networks, Content and Technology under grant agreement No 825322.

The project gratefully acknowledge the [HPC RIVR consortium](https://www.hpc-rivr.si) and
[EuroHPC JU](https://eurohpc-ju.europa.eu) for funding this project by providing computing
resources of the HPC system Vega at the
[Institute of Information Science](https://www.izum.si).

Funded by the European Union. Views and opinions expressed are however those of the author(s) only
and do not necessarily reflect those of the European Union or European Commission.
Neither the European Union nor the granting authority can be held responsible for them.
Funded within the framework of the [NGI Search](https://www.ngisearch.eu/)
project under grant agreement No 101069364.

<!-- markdownlint-disable MD033 -->

<img src="EN_FundedbytheEU_RGB_POS.png" alt="Funded by the European Union emblem" height="60" />
<img src="NGISearch_logo.svg" alt="NGI Search logo" height="60" />

<!-- markdownlint-enable MD033 -->
