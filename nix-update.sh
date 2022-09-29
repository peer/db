#!/usr/bin/env bash

sed -e "s/__VERSION__/$(git describe --tags --always --dirty=+)/g" -e "s/__REVISION__/$(git rev-parse HEAD)/g" default.nix.tmpl > default.nix

# We first try to build and it fails with hash mismatch, and we use it to populate sha256.
SRC_SHA256="$(nix-build -E 'with import <nixpkgs> { }; callPackage ./default.nix { }' 2>&1 | grep -oP 'got:\s+\Ksha256-\S+')"
sed -i -e "s|sha256 = lib.fakeSha256;|sha256 = \"$SRC_SHA256\";|g" default.nix

# We try again to build and it fails with hash mismatch, and we use it to populate vendorSha256.
VENDOR_SHA256="$(nix-build -E 'with import <nixpkgs> { }; callPackage ./default.nix { }' 2>&1 | grep -oP 'got:\s+\Ksha256-\S+')"
sed -i -e "s|vendorSha256 = lib.fakeSha256;|vendorSha256 = \"$VENDOR_SHA256\";|g" default.nix
