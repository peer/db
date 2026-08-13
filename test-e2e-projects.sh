#!/bin/bash

# Runs the Playwright projects one phase at a time.
#
# The phases are ordered because they share one populated database: the projects which only read run first and
# are screenshotted, and the projects which change documents run after them. playwright.config.ts states that
# ordering as project dependencies, which is what a run started by hand uses, but a dependency does more than
# order: a project whose dependency has a failing test is not run at all (hasFailedDeps in the Playwright
# runner). One screenshot which no longer matches would then take the editing and creating projects with it, so
# the phases are ordered here instead, by running them one after another with --no-deps, and every phase runs
# whatever the phases before it did.
#
# Each phase writes a blob report, which are merged into the HTML and JUnit reports the pipeline publishes, and
# a JSON report, which is read to tell a phase which failed on nothing but screenshot comparisons from one which
# failed on something else. The errors are told apart by what they say, and only the screenshot comparison in
# checkpoint (tests/utils.ts) counts as a screenshot: a run which is red on those alone is a set of baselines to
# look through, while anything else is something to fix.

set -e
set -o pipefail

UPDATE_SNAPSHOTS="${UPDATE_SCREENSHOTS:-missing}"

# What every screenshot comparison fails with.
SNAPSHOT_ERROR="expect(Buffer).toMatchSnapshot"

# How many errors a phase had and how many of them were not screenshot comparisons, followed by the first line
# of each of those. Errors sit on the results of the tests, and the errors which belong to no test at all (a
# worker which died, a file which did not load) sit on the report itself, so both objects carrying an "errors"
# array are collected. The colouring is taken out of a message before it is matched and before it is printed,
# because a message written for a terminal carries escape sequences in the middle of the words.
ERRORS_QUERY='
  [.. | objects | select(has("errors")) | .errors[]? | (.message // "") | gsub("\u001b\\[[0-9;]*m"; "")] as $messages
  | ($messages | map(select(contains($snapshot) | not))) as $other
  | "\($messages | length) \($other | length)",
    ($other[] | "  " + (split("\n")[0]))
'

PHASE_REPORTS="blob-report"
MERGED_REPORTS="blob-report-merged"

# Whether any phase came out with failing tests. This is what the sentence at the end of the run talks about, and
# together with the flag below it is what the run exits with.
tests_failed=0
# Whether a test failed for a reason other than a screenshot which no longer matches, which is what tells a run
# made red by an interface change apart from one made red by something being broken. A phase whose report cannot
# be read counts here too, because what its tests failed on is then not known.
other_failures=0
# Whether the reports of the phases could not be gathered or merged. That is the run failing at itself rather
# than at what it tested, so it is kept apart: it makes the run red without saying anything about the tests.
reports_failed=0

mkdir -p test-results "$MERGED_REPORTS"

run_phase() {
  local phase="$1"
  shift

  echo "Running E2E phase $phase..."

  local report="test-results/$phase.json"
  # The blob reporter empties its output directory when it starts, so every phase writes into one of its own and
  # the reports are gathered afterwards.
  if PLAYWRIGHT_BLOB_OUTPUT_DIR="$PHASE_REPORTS/$phase" \
    PLAYWRIGHT_JSON_OUTPUT_FILE="$report" \
    npx playwright test --config=playwright.config.ts --update-snapshots="$UPDATE_SNAPSHOTS" --no-deps --reporter=blob,json "$@"; then
    echo "Phase $phase: nothing failed"
    return
  fi

  # Only a phase which failed is classified. What it failed on is read out of its report, so a report which is
  # not there or cannot be read leaves the question open, which counts as a failure of its own kind.
  tests_failed=1

  if [ ! -f "$report" ]; then
    echo "Phase $phase: no report was written, so what failed cannot be told"
    other_failures=1
    return
  fi

  local summary
  if ! summary=$(jq -r --arg snapshot "$SNAPSHOT_ERROR" "$ERRORS_QUERY" "$report"); then
    echo "Phase $phase: its report could not be read, so what failed cannot be told"
    other_failures=1
    return
  fi

  # The counts are the first line of the query's output, which is what read takes, and the lines after it are the
  # errors which were not screenshot comparisons.
  local errors other
  read -r errors other <<< "$summary" || true
  if [ -z "$errors" ] || [ -z "$other" ]; then
    echo "Phase $phase: its report carried no counts, so what failed cannot be told"
    other_failures=1
    return
  fi

  echo "Phase $phase: $errors errors, $((errors - other)) of them screenshot comparisons"
  if [ "$other" -gt 0 ]; then
    other_failures=1
    tail --lines=+2 <<< "$summary"
  fi
}

run_phase readonly --project=chrome --project=pages --project=search --project=document --project=permissions
run_phase edit --project=edit
run_phase create --project=create

echo "Merging the reports of the phases..."

cp "$PHASE_REPORTS"/*/*.zip "$MERGED_REPORTS"/ || reports_failed=1
# Without --reporter the reporters of the configuration are used, which is what puts the merged reports where the
# pipeline collects them from. Naming the reporters on the command line would drop the paths configured for them.
npx playwright merge-reports --config=playwright.config.ts "$MERGED_REPORTS" || reports_failed=1

# The coverage the browsers collected is turned into a report of its own, whatever the tests did: a run which
# failed is exactly the one whose coverage is worth looking at.
npx nyc report || reports_failed=1

if [ "$tests_failed" -ne 0 ] && [ "$other_failures" -eq 0 ]; then
  echo "Every test which failed in this run failed on a screenshot which no longer matches."
fi

if [ "$tests_failed" -ne 0 ] || [ "$reports_failed" -ne 0 ]; then
  exit 1
fi
