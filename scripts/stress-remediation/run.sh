#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
OUTPUT_DIR="${STRESS_OUTPUT_DIR:-$SCRIPT_DIR/results}"
PROFILE="${STRESS_PROFILE:-smoke}"
FIXTURE="${STRESS_FIXTURE_FILE:-}"
IMAGE="grafana/k6@sha256:e66db15b860113878fa74670e31f5e274830b7b6e42c8bff28b2f2d86a257603"

if [[ -z "$FIXTURE" ]]; then
  echo "Define STRESS_FIXTURE_FILE con una fixture local externa." >&2
  exit 2
fi
if [[ ! -r "$FIXTURE" ]]; then
  echo "No se puede leer STRESS_FIXTURE_FILE." >&2
  exit 2
fi

node "$SCRIPT_DIR/target-check.js"
if [[ "$PROFILE" == "smoke" ]]; then
  node "$SCRIPT_DIR/fixture-check.js" "$FIXTURE" "$PROFILE"
else
  if [[ "${STRESS_SMOKE_ACCEPTED:-}" != "true" ]]; then
    echo "Los perfiles posteriores requieren STRESS_SMOKE_ACCEPTED=true." >&2
    exit 2
  fi
  if [[ "$PROFILE" == "soak" && "${STRESS_ENABLE_SOAK:-}" != "true" ]]; then
    echo "Soak requiere STRESS_ENABLE_SOAK=true." >&2
    exit 2
  fi
  node "$SCRIPT_DIR/summary-check.js" "$OUTPUT_DIR/smoke.json"
  STRESS_SMOKE_SUMMARY=/out/smoke.json STRESS_SMOKE_ACCEPTED=true STRESS_ENABLE_SOAK="${STRESS_ENABLE_SOAK:-}" \
    node "$SCRIPT_DIR/fixture-check.js" "$FIXTURE" "$PROFILE"
fi

mkdir -p "$OUTPUT_DIR"
FIXTURE_ABS="$(cd "$(dirname -- "$FIXTURE")" && pwd)/$(basename -- "$FIXTURE")"
docker run --rm \
  --network bridge \
  --add-host=host.docker.internal:host-gateway \
  --volume "$SCRIPT_DIR:/suite:ro" \
  --volume "$FIXTURE_ABS:/run/fixtures.json:ro" \
  --volume "$OUTPUT_DIR:/out" \
  --workdir /suite \
  --env BASE_URL \
  --env STRESS_CONFIRM_TARGET \
  --env STRESS_ALLOW_REMOTE \
  --env STRESS_EXPECT_SCHEMA \
  --env STRESS_EXPECT_429_ROUTES \
  --env "STRESS_PROFILE=$PROFILE" \
  --env "STRESS_FIXTURE_FILE=/run/fixtures.json" \
  --env "STRESS_SMOKE_ACCEPTED=${STRESS_SMOKE_ACCEPTED:-false}" \
  --env "STRESS_ENABLE_SOAK=${STRESS_ENABLE_SOAK:-false}" \
  --env "STRESS_SMOKE_SUMMARY=/out/smoke.json" \
  "$IMAGE" run --summary-export="/out/$PROFILE.json" /suite/suite.js
