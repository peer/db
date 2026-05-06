#!/bin/bash

set -e
set -o pipefail

# Run locally with:
#docker run --privileged --rm \
#  -v "$(pwd):/workspace" \
#  --name dind \
#  -w /workspace \
#  docker:28-dind \
#  sh -c " \
#    dockerd-entrypoint.sh > /tmp/dockerd.log 2>&1 & \
#    sleep 2 && \
#    DOCKER_HOST=unix:///var/run/docker.sock ./test-e2e.sh \
#  "

echo "=== E2E Test Script ==="

cleanup_peerdb_container=0
cleanup_elasticsearch_container=0
cleanup_postgres_container=0
cleanup_peerdb_image=0
cleanup_playwright_image=0
cleanup_network=0
cleanup_certs=0

cleanup() {
  set +e

  if [ "$cleanup_peerdb_container" -ne 0 ]; then
    echo "Logs PeerDB"
    docker logs peerdb-container

    echo "Stopping PeerDB Docker container (if still running)"
    docker stop peerdb-container
    docker rm -f peerdb-container
  fi

  if [ "$cleanup_elasticsearch_container" -ne 0 ]; then
    echo "Logs elasticsearch"
    docker logs peerdb-elastic

    echo "Stopping elasticsearch Docker container (if still running)"
    docker stop peerdb-elastic
    docker rm -f peerdb-elastic
  fi

  if [ "$cleanup_postgres_container" -ne 0 ]; then
    echo "Logs postgres"
    docker logs peerdb-postgres

    echo "Stopping postgres Docker container (if still running)"
    docker stop peerdb-postgres
    docker rm -f peerdb-postgres
  fi

  if [ "$cleanup_peerdb_image" -ne 0 ]; then
    echo "Removing PeerDB Docker image"
    docker image rm -f peerdb-image
  fi

  if [ "$cleanup_playwright_image" -ne 0 ]; then
    echo "Removing playwright Docker image"
    docker image rm -f peerdb-playwright-image
  fi

  if [ "$cleanup_network" -ne 0 ]; then
    echo "Removing Docker network"
    docker network rm peerdb-e2e-network
  fi

  if [ "$cleanup_certs" -ne 0 ]; then
    echo "Cleaning up temporary files"
    rm test-e2e-rootCA.pem
  fi
}

trap cleanup EXIT

# Create Docker network for E2E tests.
echo "Creating Docker network..."
docker network create peerdb-e2e-network
cleanup_network=1

echo "1. Installing dependencies and generating certificates..."

# Install required tools for certificate generation.
apk --update add openssl curl

# Install Go tools for certificate generation.
curl -L https://github.com/FiloSottile/mkcert/releases/download/v1.4.4/mkcert-v1.4.4-linux-amd64 -o /usr/local/bin/mkcert && chmod +x /usr/local/bin/mkcert
mkcert -install

# Generate certificates
mkcert peerdb-container 127.0.0.1 ::1
chmod 644 peerdb-container+2.pem peerdb-container+2-key.pem

# Copy mkcert CA certificate for Docker build.
cp "$(mkcert -CAROOT)/rootCA.pem" test-e2e-rootCA.pem
cleanup_certs=1

echo "2. Building Docker images..."

# Build the PeerDB Docker image from Dockerfile.
docker build --target production --build-arg PEERDB_BUILD_FLAGS="-cover -race -covermode atomic" --build-arg VITE_COVERAGE=true --build-arg VITE_E2E_TESTS=true -t peerdb-image .
cleanup_peerdb_image=1

# Build the Playwright test image.
docker build -f playwright.dockerfile -t peerdb-playwright-image .
cleanup_playwright_image=1

echo "3. Starting PostgreSQL container..."
  docker run -d \
  --name peerdb-postgres \
  --network peerdb-e2e-network  \
  -e PGSQL_ROLE_1_USERNAME=test \
  -e PGSQL_ROLE_1_PASSWORD=test \
  -e PGSQL_DB_1_NAME=test \
  -e PGSQL_DB_1_OWNER=test \
  registry.gitlab.com/tozd/docker/postgresql:18
cleanup_postgres_container=1

echo "4. Starting Elastic container..."
  docker run -d \
   --name peerdb-elastic \
    --network peerdb-e2e-network \
    -e network.bind_host=0.0.0.0 \
    -e network.publish_host=localhost \
    -e discovery.type=single-node \
    -e "xpack.security.enabled=false" \
    -e "ingest.geoip.downloader.enabled=false" \
    -e "cluster.routing.allocation.disk.watermark.flood_stage=100%" \
    "${CI_REGISTRY_IMAGE:-registry.gitlab.com/peerdb/peerdb}/elastic/${ELASTIC_VERSION:-7.17.9}:latest"
cleanup_elasticsearch_container=1

echo "5. Waiting for Elasticsearch service to be ready..."
for i in $(seq 1 120); do docker exec peerdb-elastic curl -sf "http://localhost:9200/_cluster/health?wait_for_status=yellow&timeout=10s" && break || { [ "$i" -eq 120 ] && exit 1; sleep 1; }; done

echo "6. Populating PeerDB with documents..."

echo "postgres://test:test@peerdb-postgres:5432/test" > .postgresql.secret

mkdir -p coverage
# We chown to the container user so the process running inside Docker container can write to coverage.
chown 1000:1000 coverage

docker run --rm \
  --network peerdb-e2e-network \
  -v "$(pwd):/data" \
  -e GOCOVERDIR=/data/coverage \
  -e SSL_CERT_FILE=/data/test-e2e-rootCA.pem \
  -e SSL_CERT_DIR=/etc/ssl/certs \
  peerdb-image \
  -d /data/.postgresql.secret \
  --elastic.url=http://peerdb-elastic:9200 \
  populate

echo "7. Starting PeerDB container..."

# Start PeerDB container with certificates.
docker run -d \
  --name peerdb-container \
  --network peerdb-e2e-network \
  -v "$(pwd):/data" \
  -e GOCOVERDIR=/data/coverage \
  -e SSL_CERT_FILE=/data/test-e2e-rootCA.pem \
  -e SSL_CERT_DIR=/etc/ssl/certs \
  peerdb-image \
  -k /data/peerdb-container+2.pem \
  -K /data/peerdb-container+2-key.pem \
  -d /data/.postgresql.secret \
  --elastic.url=http://peerdb-elastic:9200
cleanup_peerdb_container=1

echo "8. Waiting for PeerDB service to be ready..."

sleep 5

echo "9. Running Playwright tests..."

# Set environment variables for Playwright.
export LINK_PUBLISH_JOB_ID="${CI_JOB_ID}"

# Run Playwright tests in separate container.
docker run --rm \
  --name peerdb-playwright \
  --network peerdb-e2e-network \
  -v "$(pwd)/playwright-report:/src/peerdb/playwright-report" \
  -v "$(pwd)/test-results:/src/peerdb/test-results" \
  -v "$(pwd)/playwright-screenshots:/src/peerdb/playwright-screenshots" \
  -v "$(pwd)/coverage-frontend:/src/peerdb/coverage-frontend" \
  -v "$(pwd)/a11y-report:/src/peerdb/a11y-report" \
  -v "$(pwd)/.nyc_output:/src/peerdb/.nyc_output" \
  -e PEERDB_URL="https://peerdb-container:8080" \
  -e LINK_PUBLISH_JOB_ID \
  -e UPDATE_SCREENSHOTS \
  peerdb-playwright-image

# Stop the PeerDB container and check its exit code.
echo "10. Stopping PeerDB container..."
docker stop peerdb-container
PEERDB_EXIT_CODE=$(docker wait peerdb-container)

if [ "$PEERDB_EXIT_CODE" -ne 0 ]; then
  echo "ERROR: PeerDB container exited with code $PEERDB_EXIT_CODE"
  exit 1
fi

echo "=== E2E Tests Completed Successfully ==="
