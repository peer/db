# PeerDB Search

[![Go Report Card](https://goreportcard.com/badge/gitlab.com/peerdb/search)](https://goreportcard.com/report/gitlab.com/peerdb/search)
[![pipeline status](https://gitlab.com/peerdb/search/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/peerdb/search/-/pipelines)
[![coverage report](https://gitlab.com/peerdb/search/badges/main/coverage.svg)](https://gitlab.com/peerdb/search/-/graphs/main/charts)

PeerDB Search is opinionated open source search system incorporating best practices in search and user
interfaces/experience to provide an intuitive, fast, and easy to use search over semantic, structured, and full-text data.
Its user interface automatically adapts to data and search results and provides relevant filters. The goal of the user
interface is to allow users without technical knowledge to easily find results they want, without having to write queries.

Demos:

- [https://wikipedia.peerdb.org/](https://wikipedia.peerdb.org/): a search service for English Wikipedia articles,
  Wikimedia Commons files, and Wikidata data.
- [https://moma.peerdb.org/](https://moma.peerdb.org/): a search service for The
  Museum of Modern Art (MoMA) artists and artworks.
- [https://omni.peerdb.org/](https://omni.peerdb.org/): using an ElasticSearch alias to combine other demos into one
  search service

## Installation

You can run PeerDB Search behind a reverse proxy (which should support HTTP2), or simply run it directly
(it is safe to do so). PeerDB Search is compiled into one backend binary which has frontend files embedded
and they are served by the backend as well.

The [releases page](https://gitlab.com/peerdb/search/-/releases)
contains a list of stable versions. Each includes:

- A statically compiled binary.
- Docker images.
- A nix package.

### Static binary

The latest stable statically compiled binary for Linux (amd64) is available at:

[`https://gitlab.com/peerdb/search/-/releases/permalink/latest/downloads/linux-amd64/peerdb-search`](https://gitlab.com/peerdb/search/-/releases/permalink/latest/downloads/linux-amd64/peerdb-search)

You can also download older versions on the [releases page](https://gitlab.com/peerdb/search/-/releases).

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

ElasticSearch instance needs to have an index with documents in PeerDB Search schema
and configured with PeerDB Search mapping.
If you already have such an index, proceed to run PeerDB Search, otherwise first
[populate ElasticSearch with data](#populating-with-data).

Next, to run PeerDB Search you need a HTTPS TLS certificate (as required by HTTP2). When running locally
you can use [mkcert](https://github.com/FiloSottile/mkcert), a tool to create a local CA
keypair which is then used to create a TLS certificate. Use Go 1.19 or newer.

```sh
go install filippo.io/mkcert@latest
mkcert -install
mkcert localhost 127.0.0.1 ::1
```

This creates two files, `localhost+2.pem` and `localhost+2-key.pem`, which you can provide to PeerDB Search as:

```sh
./search -k localhost+2.pem -K localhost+2-key.pem
```

When running using Docker, you have to provide them to the container through a volume, e.g.:

```sh
docker run -d --network peerdb --name peerdb-search -p 8080:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/search/branch/main:latest -e http://elasticsearch:9200 \
 -k /data/localhost+2.pem -K /data/localhost+2-key.pem
```

Open [https://localhost:8080/](https://localhost:8080/) in your browser to access the web interface.

Temporary accepted self-signed certificates are not recommended because
[not all browser features work](https://stackoverflow.com/questions/74161355/are-any-web-features-not-available-in-browsers-when-using-self-signed-certificat).
If you want to use a self-signed certificate instead of `mkcert`, add the certificate to
your browser's certificate store.

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

Power of PeerDB Search comes from having data in ElasticSearch index organized into documents in PeerDB Search schema.
The schema is designed to allow describing almost any data. Moreover, data and properties to describe data can be changed
at runtime without having to reconfigure PeerDB Search, e.g., it adapts filters automatically.
The schema also allows multiple data sources to be used and merged together.

PeerDB Search schema of documents is fully described in
[JSON Schema](https://json-schema.org/) and is available [here](./schema/doc.json).
But at a high-level look like:

```json
{
  "_id": "22 character ID",
  "name": {
    "en": "name in English"
  },
  "score": 1.0,
  "active": {
    "id": [
      {
        "_id": "22 character ID",
        "confidence": 1.0,
        "prop": {
          "_id": "22 character property ID",
          "name": {
            "en": "property name in English"
          },
          "score": 1.0
        },
        "id": "external ID value"
      }
    ],
    "ref": [...],
    "text": [...],
    "amount": [...],
    "rel": [...],
    "file": [...],
    "time": [...]
  }
}
```

Besides core metadata (`_id`, `name`, and `score`) all other data is organized
in claims (seen under `active` claims above) which are then organized based on claim
(data) type. For example, there are `id` claims which are used to store external
ID values. `prop` is a reference to a property document which describes the ID value.

Which properties you use and how you use them to map your data to PeerDB Search documents
is left to you. We do suggest that you first populate the index using core PeerDB Search
properties. You can do that by running:

```sh
./search populate
```

This also creates an ElasticSearch index if it does not yet exist and configures it
with [PeerDB Search mapping](./index.json). Otherwise you have to create such index
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
  But you could also decide to map `title` to core `name` metadata, and `body` to
  existing `DESCRIPTION` (for shorter HTML contents shown in search results as well)
  or `ARTICLE` (for longer HTML contents) core property (and its label `HAS_ARTICLE`).
- Maybe you also want to record the original blog post ID and author `username`,
  so create property documents for them as well.
- Author `displayName` name can be mapped to `name` core metadata.
- Another property document is needed for the `author` property.
- Documents should also have additional claims to describe relations between them.
  Properties should be marked as properties and which claim type they are meant to be
  used for. It is useful to create properties for user documents and blog post documents,
  which can in turn be more general (core) items.

Assuming that the author does not yet have its document, you could convert the above blog
post into the following two PeerDB Search documents:

```json
{
  "_id": "LcrxeiU9XjxosmX8kiPCx6",
  "name": {
    "en": "Foo Bar"
  },
  "score": 1.0,
  "active": {
    "id": [
      {
        "_id": "9TfczNe5aa4LrKqWQWfnFF",
        "confidence": 1.0,
        "prop": {
          "_id": "Hx5zknvxsmPRiLFbGMPeiZ",
          "name": {
            "en": "author username"
          },
          "score": 1.0
        },
        "id": "foobar"
      }
    ],
    "rel": [
      {
        "_id": "sgpzxwxPyn51j5VfH992ZQ",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "6asppjBfRGSTt5Df7Zvomb",
          "name": {
            "en": "user"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

```json
{
  "_id": "MpGZyd7grTBPYhMhETAuHV",
  "name": {
    "en": "Some title"
  },
  "score": 1.0,
  "active": {
    "id": [
      {
        "_id": "Ci3A1tLF6MHZ4y5zBibvGg",
        "confidence": 1.0,
        "prop": {
          "_id": "8mu7vrUK7zJ4Me2JwYUG6t",
          "name": {
            "en": "blog post ID"
          },
          "score": 1.0
        },
        "id": "123"
      }
    ],
    "text": [
      {
        "_id": "VdX1HZm1ETw8K77nLTV6yt",
        "confidence": 1.0,
        "prop": {
          "_id": "FJJLydayUgDuqFsRK2ZtbR",
          "name": {
            "en": "article"
          },
          "score": 1.0
        },
        "html": {
          "en": "Some <b>blog post</b> body HTML"
        }
      }
    ],
    "rel": [
      {
        "_id": "xbufQEChDXvtg3hh4i1PvT",
        "confidence": 1.0,
        "prop": {
          "_id": "fmUeT7JN8qPuFw28Vdredm",
          "name": {
            "en": "author"
          },
          "score": 1.0
        },
        "to": {
          "_id": "LcrxeiU9XjxosmX8kiPCx6",
          "name": {
            "en": "Foo Bar"
          },
          "score": 1.0
        }
      },
      {
        "_id": "LJNg7QaiMxE1crjMiijpaN",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "3APZXnK3uofpdEJV55Po18",
          "name": {
            "en": "blog post"
          },
          "score": 1.0
        }
      },
      {
        "_id": "UxhYEJY6mpA147eujZ489B",
        "confidence": 1.0,
        "prop": {
          "_id": "5SoFeEFk5aWXUYFC1EZFec",
          "name": {
            "en": "label"
          },
          "score": 1.0
        },
        "to": {
          "_id": "MQYs7JmAR3tge25eTPS8XT",
          "name": {
            "en": "has article"
          },
          "score": 1.0
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
  "_id": "Hx5zknvxsmPRiLFbGMPeiZ",
  "name": {
    "en": "author username"
  },
  "score": 1.0,
  "active": {
    "rel": [
      {
        "_id": "5zZZ6nJFKuA5oNBu9QbsYY",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HohteEmv2o7gPRnJ5wukVe",
          "name": {
            "en": "property"
          },
          "score": 1.0
        }
      },
      {
        "_id": "jjcWxq9VoVLhKqV2tnqz1A",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "UJEVrqCGa9f3vAWi2mNWc7",
          "name": {
            "en": "\"identifier\" claim type"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

```json
{
  "_id": "8mu7vrUK7zJ4Me2JwYUG6t",
  "name": {
    "en": "blog post ID"
  },
  "score": 1.0,
  "active": {
    "rel": [
      {
        "_id": "DWYDFZ2DbasS4Tyehnko2U",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HohteEmv2o7gPRnJ5wukVe",
          "name": {
            "en": "property"
          },
          "score": 1.0
        }
      },
      {
        "_id": "ivhonZLA2ktDwyMawBLuKV",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "UJEVrqCGa9f3vAWi2mNWc7",
          "name": {
            "en": "\"identifier\" claim type"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

```json
{
  "_id": "fmUeT7JN8qPuFw28Vdredm",
  "name": {
    "en": "author"
  },
  "score": 1.0,
  "active": {
    "rel": [
      {
        "_id": "gK8nXxJ3AXErTmGPoAVF78",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HohteEmv2o7gPRnJ5wukVe",
          "name": {
            "en": "property"
          },
          "score": 1.0
        }
      },
      {
        "_id": "pDBnb32Vd2VPd2UjPnd1eS",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HZ41G8fECg4CjfZN4dmYwf",
          "name": {
            "en": "\"relation\" claim type"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

```json
{
  "_id": "6asppjBfRGSTt5Df7Zvomb",
  "name": {
    "en": "user"
  },
  "score": 1.0,
  "active": {
    "rel": [
      {
        "_id": "79m7fNMHy7SRinSmB3WARM",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HohteEmv2o7gPRnJ5wukVe",
          "name": {
            "en": "property"
          },
          "score": 1.0
        }
      },
      {
        "_id": "EuGC7ZRK7weuHNoZLDC8ah",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "6HpkLLj1iSK3XBhgHpc6n3",
          "name": {
            "en": "item"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

```json
{
  "_id": "3APZXnK3uofpdEJV55Po18",
  "name": {
    "en": "blog post"
  },
  "score": 1.0,
  "active": {
    "rel": [
      {
        "_id": "c24VwrPEMwZUhRgECzSn1b",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "HohteEmv2o7gPRnJ5wukVe",
          "name": {
            "en": "property"
          },
          "score": 1.0
        }
      },
      {
        "_id": "faByPL18Y1ZH2NAhea4FBy",
        "confidence": 1.0,
        "prop": {
          "_id": "2fjzZyP7rv8E4aHnBc6KAa",
          "name": {
            "en": "is"
          },
          "score": 1.0
        },
        "to": {
          "_id": "6HpkLLj1iSK3XBhgHpc6n3",
          "name": {
            "en": "item"
          },
          "score": 1.0
        }
      }
    ]
  }
}
```

</details>

### MoMA search

To populate search with [The Museum of Modern Art](https://www.moma.org/) (MoMA)
artists and artworks (from [this dataset](https://github.com/MuseumofModernArt/collection)),
clone the repository and run (you need Go 1.19 or newer):

```sh
make moma
./moma
```

Runtime is few minutes. If you also want to add articles (to have more full-text data)
and (more) images from MoMA's website, run instead:

```sh
./moma --website-data
```

Fetching data from the website takes time, so runtime is around 12 hours.

### Wikipedia search

To populate search with [English Wikipedia](https://en.wikipedia.org/wiki/Main_Page)
articles, [Wikimedia Commons](https://commons.wikimedia.org/wiki/Main_Page) files,
and [Wikidata](https://www.wikidata.org/wiki/Wikidata:Main_Page) data,
clone the repository and run (you need Go 1.19 or newer):

```sh
make wikipedia
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
- `optimize` forces merging of ElasticSearch segments (runtime few hours).

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

## Configuration

PeerDB Search can be configured through CLI arguments and a config file. CLI arguments have precedence
over the config file. Config file is a YAML file with the structure corresponding to the structure of
CLI flags and commands. Run `./search --help` for list of available flags and commands. If no command is
specified, `serve` command is the default.

Each PeerDB Search instance can serve multiple sites and Let's Encrypt can be used to obtain
HTTPS TLS certificates for them automatically. Example config file for all demos is available
in [`demos.yml`](./demos.yml). It configures sites, their titles, and ElasticSearch indices
for each site. To use the config file with Docker, you could do:

```sh
docker run -d --network peerdb --name peerdb-search -p 443:8080 -v "$(pwd):/data" \
 registry.gitlab.com/peerdb/search/branch/main:latest -e http://elasticsearch:9200 \
 -E name@example.com -C /data/letsencrypt -c /data/demos.yml
```

### Size of documents filter

PeerDB Search can filter on size of documents, but it requires
[installed mapper-size ElasticSearch plugin](https://www.elastic.co/guide/en/elasticsearch/plugins/current/mapper-size.html)
and enabled size field in the [index](https://www.elastic.co/guide/en/elasticsearch/plugins/current/mapper-size-usage.html).
If you use populate command to create the index, you can enable the size field with the `--size-field` argument:

```sh
./search --size-field populate
```

Alternatively, you can set `sizeField` in site configuration.

If you use Docker to run ElasticSearch, you can
[use create a custom Docker image with the plugin installed](https://www.elastic.co/guide/en/elasticsearch/reference/7.17/docker.html#_c_customized_image),
or run on your running container:

```sh
docker exec -t -i elasticsearch bin/elasticsearch-plugin install mapper-size
docker restart elasticsearch
```

### Use with ElasticSearch alias

If you use an
[ElasticSearch alias](https://www.elastic.co/guide/en/elasticsearch/reference/7.17/aliases.html)
instead of an index, PeerDB Search will provide a filter to filter documents based
on which index they come from.

## Development

During PeerDB Search development run backend and frontend as separate processes. During development the backend
proxies frontend requests to Vite, which in turn compiles frontend files and serves them, hot-reloading
the frontend as necessary.

### Backend

The backend is implemented in [Go](https://golang.org/) (requires 1.19 or newer)
and provides a HTTP2 API. Node 16 or newer is required as well.

Then clone the repository and run:

```sh
make search
./search -d -k localhost+2.pem -K localhost+2-key.pem
```

`localhost+2.pem` and `localhost+2-key.pem` are files of a TLS certificate
generated as described in the [Usage section](#usage).
Backend listens at [https://localhost:8080/](https://localhost:8080/).

`-d` CLI argument makes the backend proxy unknown requests (non-API requests)
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
