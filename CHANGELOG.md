# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Search using a natural language prompt which is parsed by a LLM into a search
  query and filters.

### Changed

- Upgrade to Go 1.23.

## [0.3.0] - 2024-03-22

### Changed

- Upgrade to Go 1.21 and Node 20.
- Remove active/inactive claims split and have only one set of claims.
- Rename repository from `gitlab.com/peerdb/search` to `gitlab.com/peerdb/peerdb`,
  including Go package namespace. Main binary is now `peerdb`.

## [0.2.0] - 2022-11-08

### Added

- Support serving multiple sites/indices.
- Support config file.
- MoMA demo site.
- Filter on size of documents.
- Filter on source index when using ElasticSearch alias.
- Omni demo site.

## [0.1.0] - 2022-09-30

### Added

- First release.

[unreleased]: https://gitlab.com/peerdb/peerdb/-/compare/v0.3.0...main
[0.3.0]: https://gitlab.com/peerdb/peerdb/-/compare/v0.2.0...v0.3.0
[0.2.0]: https://gitlab.com/peerdb/peerdb/-/compare/v0.1.0...v0.2.0
[0.1.0]: https://gitlab.com/peerdb/peerdb/-/tags/v0.1.0

<!-- markdownlint-disable-file MD024 -->
