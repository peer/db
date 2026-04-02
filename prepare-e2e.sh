#!/bin/bash

set -e
set -o pipefail

if [ -n "$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" ]; then
  # Check if target branch has an open MR.
  echo "Checking if target branch $CI_MERGE_REQUEST_TARGET_BRANCH_NAME has an open merge request..."
  TARGET_MR_IID=$(wget -q --header="PRIVATE-TOKEN: ${CI_JOB_TOKEN}" \
    -O - "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/merge_requests?source_branch=${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}&state=opened" \
    | jq -r '.[0].iid // empty')

  if [ -n "$TARGET_MR_IID" ]; then
    echo "Target branch has open MR !${TARGET_MR_IID}, downloading screenshots from merged results pipeline."
    wget -O artifacts.zip "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/jobs/artifacts/refs/merge-requests/${TARGET_MR_IID}/merge/download?job=test_e2e"
  else
    echo "Target branch has no open MR, downloading screenshots from branch artifacts."
    wget -O artifacts.zip "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/jobs/artifacts/${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}/download?job=test_e2e"
  fi
elif [ -n "$CI_COMMIT_TAG" ] || [ "$CI_COMMIT_BRANCH" = "$CI_DEFAULT_BRANCH" ]; then
  echo "Tag or main branch pipeline, update screenshots."
  export UPDATE_SCREENSHOTS=changed
else
  echo "Branch pipeline for '$CI_COMMIT_BRANCH', downloading screenshots from default branch '($CI_DEFAULT_BRANCH)'."
  wget -O artifacts.zip "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/jobs/artifacts/${CI_DEFAULT_BRANCH}/download?job=test_e2e"
fi

# Unzip artifacts if downloaded.
if [ -f artifacts.zip ]; then
  unzip -o artifacts.zip -x playwright-report/\* -x test-results/\* -x coverage.html -x coverage.xml -x coverage-frontend/\* -x a11y-report/\*
fi
