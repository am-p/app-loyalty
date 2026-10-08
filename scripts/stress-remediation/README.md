# Puntazo stress remediation suite

Reproducible, low-resource k6 scenarios for authenticated reads against a local API or the explicitly selected testing API. This harness does not create users, run migrations, seed databases, or send load while validating fixtures. The selected source checkout is the API evidence plane; it does not make the candidate equal to the deployed runtime.

The router currently registers the flow as four authenticated reads: `GET /v1/me`, `GET /v1/clientes/me`, `GET /v1/clientes/me/tarjetas?page=1&page_size=20`, and `GET /v1/clientes/me/movimientos?page=1&page_size=20`. Login, refresh, and logout use the native transport header `X-Client-Platform: native`; native auth returns tokens in JSON. Setup logs in once per distinct synthetic customer and each VU uses only its assigned session, retains its refresh token in that VU's JavaScript state, and refreshes before the 900-second access token expires. There is no registration/password-hashing workload in the iteration loop.

## Profiles

| Profile | Load | Fixture identities | Gate |
|---|---:|---:|---|
| `smoke` | 3 VUs for 60 s | 3 | Preflight and explicit destination confirmation. |
| `sustained` | 3 VUs for 5 min | 3 | `STRESS_SMOKE_ACCEPTED=true` and passing prior smoke summary. |
| `spike` | 1→3→5→3→0 VUs across 60 s | 5 | Same smoke gate; maximum 5 VUs. |
| `soak` | 3 VUs for 2 h | 3 | Same smoke gate and `STRESS_ENABLE_SOAK=true`; hard cap 3 VUs. |

Each completed journey sleeps one second before its next iteration. No rate or capacity claim follows from these fixed virtual users; the RPS actually achieved depends on response times and is part of the output. This first suite measures authenticated GETs and their four-call workflow. It does not generate browser Core Web Vitals; LCP <2.5 s, INP <200 ms, and CLS <0.1 require the separate representative browser run. API acceptance thresholds are error rate <1%, checks >99%, GET p95 <500 ms, GET p99 <1,000 ms, and workflow p95 <3,000 ms.

The `api_error_rate` excludes only HTTP 429 routes explicitly listed in `STRESS_EXPECT_429_ROUTES`, and every 429 is separately counted in `expected_429` or `unexpected_429`. This read suite has no expected 429 route by default. Do not mark a route's 429 expected to hide a regression; only do so for a separately approved quota test, and report both counters with the result.

Thresholds use `abortOnFail`. The final iteration attempts logout; teardown also sends idempotent logout using each original family token, covering spike VUs removed during ramp-down and rotated descendants. These tokens stay only in k6 memory and never enter summaries or artifacts. A forced process stop that bypasses teardown may still leave fixture sessions active; inspect/revoke only the new dedicated fixture identities, never historical test accounts.

## Fixture contract

The load runner requires one distinct synthetic customer account per VU and rejects repeated emails, missing VU numbers, `.invalid` addresses, and placeholder passwords. The repository includes a local-only Go fixture command that prepares the representative dataset and writes three or five synthetic credentials to an external JSON file with mode 0600. It uses one bcrypt hash generated for all fixture customers, avoiding a bcrypt workload per user. It performs SQL inserts in a single transaction and verifies foreign keys, row counts, movement chains, and card balances before commit. It never calls the HTTP API.

The fixture command is intentionally restricted to an empty database named `puntazo_load` on loopback port 55437, schema migrations 0001–0034, and requires `STRESS_LOCAL_FIXTURES=true`. The database connection variable is `STRESS_DATABASE_URL`; do not point it at a shared, testing, or production database. The output credential JSON must be an absolute path outside the repository and does not overwrite an existing file. It seeds 100 brands, 100 branch/program/staff memberships, 10,000 customers/cards, and 100,000 credit movements (10 per card) ending at 100 points per card. No bulk HTTP account registration is part of load preparation. The harness logs in only once per VU; 5 VUs stay below the current login limiter of 10 attempts per IP per 10 minutes, provided the same source IP is not running another test or retrying after failures. This suite does not support 10+ VUs or mint pre-issued sessions; add and review a separate local-only session-fixture workflow before expanding the VU cap. Do not raise the login quota or share refresh tokens.

## Safety and run procedure

The only permitted targets are loopback/local hosts and `https://api-testing.puntazo.pro` (current independent API) or the legacy testing host. Production and every other remote host are hard blocked in `guards.js`. Every run requires `STRESS_CONFIRM_TARGET=local` for a local target. Testing also requires `STRESS_ALLOW_REMOTE=true` and `STRESS_CONFIRM_TARGET=api-testing.puntazo.pro`; testing load is authorized by the remediation plan; confirm the exact target before running. k6 preflights `/v1/health/ready` and `/v1/version`, records version/commit/schema, and refuses a schema mismatch when `STRESS_EXPECT_SCHEMA` is set.

1. After migrating the isolated local `puntazo_load` database through schema 0034, create the representative dataset and a private credential fixture outside the repository. The command refuses a non-empty database or a non-loopback target:

   ```sh
   STRESS_LOCAL_FIXTURES=true \
   STRESS_DATABASE_URL="$TEST_DATABASE_URL" \
   go run ./scripts/stress-remediation/fixtures -out /tmp/puntazo-stress-fixtures.json -vus 3
   ```

   Use `-vus 5` for the spike profile. Keep the generated file private and remove it after the campaign.

2. Point to a local API reachable from Docker. On Docker Desktop, `host.docker.internal` resolves to the host; on Linux, `run.sh` adds the Docker host gateway. Select the confirmation explicitly:

   ```sh
   export BASE_URL=http://host.docker.internal:8080
   export STRESS_CONFIRM_TARGET=local
   export STRESS_FIXTURE_FILE=/tmp/puntazo-stress-fixtures.json
   scripts/stress-remediation/run.sh
   ```

3. Review `scripts/stress-remediation/results/smoke.json` and the printed run metadata. Check the `checks`, `api_error_rate`, `api_read_duration`, `workflow_duration`, `expected_429`, `unexpected_429`, and `session_logout` metrics. A human sets the explicit acceptance flag only after reviewing those values:

   ```sh
   node scripts/stress-remediation/summary-check.js scripts/stress-remediation/results/smoke.json
   STRESS_PROFILE=sustained STRESS_SMOKE_ACCEPTED=true scripts/stress-remediation/run.sh
   ```

4. Run `spike` only after the smoke gate passes. Run the two-hour soak only when the sustained and spike evidence has been reviewed and explicitly opt in:

   ```sh
   STRESS_PROFILE=spike STRESS_SMOKE_ACCEPTED=true scripts/stress-remediation/run.sh
   STRESS_PROFILE=soak STRESS_SMOKE_ACCEPTED=true STRESS_ENABLE_SOAK=true scripts/stress-remediation/run.sh
   ```

The image is pinned to the official `grafana/k6` v2.3.0 digest `sha256:e66db15b860113878fa74670e31f5e274830b7b6e42c8bff28b2f2d86a257603`; no npm k6 dependency or image build is needed. The runner writes summaries only under the ignored `results/` directory. Do not run the remote-testing target without a separate campaign authorization, fixture review, and maintenance window.

For the larger scenario, use this isolated seed or a reviewed local restore that documents schema, account/card/movement counts, and balance-ledger checks. An optional PostgreSQL CPU increase is a separate measurement and should be temporary; routine testing remains on the VPS's existing resource limits. Production sizing is a separate campaign.

## Readiness monitor

`monitor-health.mjs` uses Node.js 20 or newer to check `/v1/health/ready` and `/v1/version` every 30 seconds. It accepts the API's four-digit schema string, such as `"0035"`, preserving its leading zeros. Requests time out after three seconds, response bodies are limited to 8 KiB, and transition alerts are limited to 2 KiB. Polls do not overlap.

```sh
BASE_URL=https://api-testing.puntazo.pro \
STRESS_ALLOW_REMOTE=true \
STRESS_CONFIRM_TARGET=api-testing.puntazo.pro \
MONITOR_INTERVAL_SECONDS=30 \
node --max-old-space-size=32 scripts/stress-remediation/monitor-health.mjs
```

For local verification, use a loopback `BASE_URL` and `STRESS_CONFIRM_TARGET=local`. Without `WEBHOOK_URL`, the monitor only writes JSON lines to standard output, suitable for a service journal. A failing first probe emits `startup_failure`; later failure and recovery emit one `unhealthy` or `recovery` event per transition. Repeated probes in the same state do not repeat the alert. Healthy startup produces a probe without a transition alert. No session, credentials, private endpoint, or external webhook is required. An optional webhook is restricted to HTTP loopback.

The Node heap limit does not limit total process memory. The isolated local dependency test observed about 80 MiB RSS with a 32 MiB heap limit. The selected worker budget is therefore 96 MiB total memory and 0.1 CPU, with no published ports. Installing or creating that worker is a separate infrastructure action; this script is not included in the API runtime image.

The separate `Dockerfile.monitor` copies only the Node binary from the pinned official Node 22 image into pinned Alpine 3.24, updates Alpine packages, and installs CA certificates and the C++ runtime. npm, Yarn and their dependency trees are absent. After fetching the selected current source branch and passing the clean-source gate, build and scan the monitor separately from the API:

```sh
docker build --platform linux/amd64 \
  --build-arg SOURCE_COMMIT="$(git rev-parse HEAD)" \
  -f scripts/stress-remediation/Dockerfile.monitor \
  -t puntazo-readiness-monitor:20261007 .
docker scout cves --only-severity critical,high puntazo-readiness-monitor:20261007
docker run --rm --name puntazo-readiness-monitor-testing \
  --network host --user 65534:65534 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --memory 96m --cpus 0.1 \
  --log-driver json-file --log-opt max-size=10m --log-opt max-file=3 \
  --env BASE_URL=https://api-testing.puntazo.pro \
  --env STRESS_ALLOW_REMOTE=true \
  --env STRESS_CONFIRM_TARGET=api-testing.puntazo.pro \
  --env MONITOR_INTERVAL_SECONDS=30 \
  puntazo-readiness-monitor:20261007
```

Deployment must use the scanned image ID recorded after that build and verify three healthy probes with the resource limits applied. Image tags and SHAs in previous reports are evidence; future builds start from the fetched current selected branch.

## Validation

Run the non-network guard tests with `npm test` from this directory. Validate a private fixture and confirm the destination with `node fixture-check.js <fixture> smoke` and `node target-check.js`. Those commands do not make network requests. `run.sh` is the only entry point that starts k6 requests.
