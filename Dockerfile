# This Dockerfile requires DOCKER_BUILDKIT=1 to be build.
# We do not use syntax header so that we do not have to wait
# for the Dockerfile frontend image to be pulled.
FROM node:20.24-alpine3.22 as node-build

RUN apk --update add make bash
COPY . /src/peerdb
WORKDIR /src/peerdb
RUN \
  npm ci --audit=false && \
  npm audit signatures && \
  make dist

FROM golang:1.25-alpine3.22 AS go-build

RUN apk --update add make bash git gcc musl-dev ca-certificates tzdata mailcap && \
  adduser -D -H -g "" -s /sbin/nologin -u 1000 user
COPY . /src/peerdb
COPY --from=node-build /src/peerdb/dist /src/peerdb/dist
WORKDIR /src/peerdb
# We want Docker image for build timestamp label to match the one in
# the binary so we take a timestamp once outside and pass it in.
ARG BUILD_TIMESTAMP
ARG PEERDB_BUILD_FLAGS
# We run make with "-o dist" which prevents dist from being build here as it was done
# in the node-build stage and we cannot (missing node, etc.) and do not want to build
# it again, but it might have file timestamps which would otherwise trigger a build.
RUN \
  BUILD_TIMESTAMP=$BUILD_TIMESTAMP PEERDB_BUILD_FLAGS="$PEERDB_BUILD_FLAGS" make -o dist build-static && \
  mv peerdb /go/bin/peerdb

FROM alpine:3.22 AS debug
COPY --from=go-build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-build /etc/mime.types /etc/mime.types
COPY --from=go-build /etc/passwd /etc/passwd
COPY --from=go-build /etc/group /etc/group
COPY --from=go-build /go/bin/peerdb /
USER user:user
EXPOSE 8080
ENTRYPOINT ["/peerdb"]

FROM scratch AS production
RUN --mount=from=busybox:1.36-musl,src=/bin/,dst=/bin/ ["/bin/mkdir", "-m", "1755", "/tmp"]
COPY --from=go-build /etc/services /etc/services
COPY --from=go-build /etc/protocols /etc/protocols
# The rest is the same as for the debug image.
COPY --from=go-build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-build /etc/mime.types /etc/mime.types
COPY --from=go-build /etc/passwd /etc/passwd
COPY --from=go-build /etc/group /etc/group
COPY --from=go-build /go/bin/peerdb /
USER user:user
EXPOSE 8080
ENTRYPOINT ["/peerdb"]
