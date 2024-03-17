# This Dockerfile requires DOCKER_BUILDKIT=1 to be build.
# We do not use syntax header so that we do not have to wait
# for the Dockerfile frontend image to be pulled.
FROM node:20.11-alpine3.18 as node-build

RUN apk --update add make
COPY . /src/peerdb-search
WORKDIR /src/peerdb-search
RUN \
  npm ci --audit=false && \
  npm audit signatures && \
  make dist

FROM golang:1.21-alpine3.18 AS go-build

RUN apk --update add make bash git gcc musl-dev ca-certificates tzdata mailcap && \
  adduser -D -H -g "" -s /sbin/nologin -u 1000 user
COPY . /src/peerdb-search
# We make an empty node_modules so that Makefile does not try to run npm install (dist files are
# copied next). It has to be made before COPY so that it looks older than dist to make.
RUN mkdir /src/peerdb-search/node_modules
COPY --from=node-build /src/peerdb-search/dist /src/peerdb-search/dist
WORKDIR /src/peerdb-search
# We want Docker image for build timestamp label to match the one in
# the binary so we take a timestamp once outside and pass it in.
ARG BUILD_TIMESTAMP
RUN \
  BUILD_TIMESTAMP=$BUILD_TIMESTAMP make build-static && \
  mv search /go/bin/search

FROM alpine:3.18 AS debug
COPY --from=go-build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-build /etc/mime.types /etc/mime.types
COPY --from=go-build /etc/passwd /etc/passwd
COPY --from=go-build /etc/group /etc/group
COPY --from=go-build /go/bin/search /
USER user:user
EXPOSE 8080
ENTRYPOINT ["/search"]

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
COPY --from=go-build /go/bin/search /
USER user:user
EXPOSE 8080
ENTRYPOINT ["/search"]
