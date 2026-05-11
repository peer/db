variable "PEERDB_IMAGE" {
  default = "peerdb-image"
}

variable "PLAYWRIGHT_IMAGE" {
  default = "peerdb-playwright-image"
}

group "default" {
  targets = ["peerdb", "playwright"]
}

target "peerdb" {
  context    = "."
  dockerfile = "Dockerfile"
  target     = "production"
  args = {
    PEERDB_BUILD_FLAGS = "-cover -race -covermode atomic"
    VITE_COVERAGE      = "true"
    VITE_E2E_TESTS     = "true"
  }
  tags = ["${PEERDB_IMAGE}"]
}

target "playwright" {
  context    = "."
  dockerfile = "playwright.dockerfile"
  tags       = ["${PLAYWRIGHT_IMAGE}"]
}
