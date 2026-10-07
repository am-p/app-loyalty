import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";
import {
  evaluateSmokeSummary,
  originHost,
  validateBaseUrl,
  validateFixtureDocument,
  validateProfile,
} from "./guards.js";

const profileName = __ENV.STRESS_PROFILE || "smoke";
const profiles = {
  smoke: { duration: "60s", durationMs: 60_000, vus: 3, maxVus: 3, type: "constant" },
  sustained: { duration: "5m", durationMs: 300_000, vus: 3, maxVus: 3, type: "constant" },
  spike: { duration: "60s", durationMs: 60_000, vus: 5, maxVus: 5, type: "spike" },
  soak: { duration: "2h", durationMs: 7_200_000, vus: 3, maxVus: 3, type: "constant" },
};

validateProfile(profileName, __ENV);
const profile = profiles[profileName];
const fixtureDocument = __ENV.STRESS_FIXTURE_FILE ? JSON.parse(open(__ENV.STRESS_FIXTURE_FILE)) : { identities: [] };
const baseUrl = __ENV.BASE_URL ? validateBaseUrl(__ENV.BASE_URL, {
  allowRemoteTesting: __ENV.STRESS_ALLOW_REMOTE === "true",
  confirmTarget: __ENV.STRESS_CONFIRM_TARGET || "",
}) : "";
const expected429Routes = new Set((__ENV.STRESS_EXPECT_429_ROUTES || "").split(",").map((value) => value.trim()).filter(Boolean));
const apiErrorRate = new Rate("api_error_rate");
const expected429 = new Counter("expected_429");
const unexpected429 = new Counter("unexpected_429");
const readDuration = new Trend("api_read_duration", true);
const workflowDuration = new Trend("workflow_duration", true);
const sessionLogouts = new Counter("session_logout");

function threshold(threshold) {
  return { threshold, abortOnFail: true, delayAbortEval: "10s" };
}

const scenario = profile.type === "spike"
  ? {
      executor: "ramping-vus",
      startVUs: 1,
      stages: [
        { duration: "15s", target: 3 },
        { duration: "15s", target: 5 },
        { duration: "15s", target: 3 },
        { duration: "15s", target: 0 },
      ],
      gracefulRampDown: "20s",
      gracefulStop: "45s",
    }
  : {
      executor: "constant-vus",
      vus: profile.vus,
      duration: profile.duration,
      gracefulStop: "45s",
    };

export const options = {
  scenarios: { authenticated_reads: { ...scenario, exec: "authenticatedReadJourney" } },
  thresholds: {
    api_error_rate: [threshold("rate<0.01")],
    checks: [threshold("rate>0.99")],
    api_read_duration: [threshold("p(95)<500"), threshold("p(99)<1000")],
    workflow_duration: [threshold("p(95)<3000")],
  },
  noConnectionReuse: false,
  discardResponseBodies: true,
  userAgent: "puntazo-stress-remediation/1.0",
};

const credentialsByVu = new Map(fixtureDocument.identities.map((identity) => [identity.vu, identity]));
const vuState = new Map();
const expectedApiStatus = (response, name, successStatus = 200) => {
  const routeExpects429 = expected429Routes.has(name);
  const isExpected429 = response.status === 429 && routeExpects429;
  const isSuccess = response.status === successStatus || isExpected429;
  const is429 = response.status === 429;
  expected429.add(isExpected429 ? 1 : 0, { name });
  unexpected429.add(is429 && !isExpected429 ? 1 : 0, { name });
  apiErrorRate.add(!isSuccess && !isExpected429, { name });
  const checked = check(response, { [`${name}: status esperado`]: () => isSuccess });
  return isSuccess && checked;
};

function authRequest(name, path, payload, token = "") {
  const headers = {
    "Content-Type": "application/json",
    Accept: "application/json",
    "X-Client-Platform": "native",
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = http.post(`${baseUrl}${path}`, JSON.stringify(payload), {
    headers,
    timeout: "5s",
    responseType: "text",
    tags: { name },
  });
  const ok = expectedApiStatus(response, name);
  if (!ok) throw new Error(`${name} falló; consulta status y métricas del run, nunca se imprimen credenciales ni bodies.`);
  return response;
}

function parseSession(response, name) {
  let data;
  try {
    data = response.json("data");
  } catch {
    throw new Error(`${name} devolvió JSON inválido.`);
  }
  const session = data?.session;
  if (!session?.access_token || !session?.refresh_token || !Number.isFinite(session.expires_in)) {
    throw new Error(`${name} no devolvió una sesión native completa.`);
  }
  return session;
}

function login(identity) {
  const response = authRequest("auth_login", "/v1/auth/login", {
    email: identity.email,
    password: identity.password,
  });
  const data = response.json("data");
  if (identity.account_type && data?.user?.account_type !== identity.account_type) {
    throw new Error("La cuenta fixture no tiene el account_type de cliente esperado.");
  }
  return parseSession(response, "auth_login");
}

function refresh(state) {
  const response = authRequest("auth_refresh", "/v1/auth/refresh", {
    refresh_token: state.refreshToken,
  });
  const session = parseSession(response, "auth_refresh");
  state.accessToken = session.access_token;
  state.refreshToken = session.refresh_token;
  state.accessExpiresAt = Date.now() + session.expires_in * 1000;
  state.lastRefreshAt = Date.now();
}

function ensureSession(identity) {
  let state = vuState.get(__VU);
  if (!state) {
    const session = login(identity);
    state = {
      accessToken: session.access_token,
      refreshToken: session.refresh_token,
      accessExpiresAt: Date.now() + session.expires_in * 1000,
      lastRefreshAt: Date.now(),
      closed: false,
    };
    vuState.set(__VU, state);
  } else if (Date.now() >= Math.min(state.accessExpiresAt - 60_000, state.lastRefreshAt + 14 * 60_000)) {
    refresh(state);
  }
  return state;
}

function logout(state) {
  if (!state || state.closed) return;
  const headers = {
    "Content-Type": "application/json",
    Accept: "application/json",
    "X-Client-Platform": "native",
    Authorization: `Bearer ${state.accessToken}`,
  };
  const response = http.post(`${baseUrl}/v1/auth/logout`, JSON.stringify({ refresh_token: state.refreshToken }), {
    headers,
    timeout: "5s",
    tags: { name: "auth_logout" },
  });
  const ok = response.status === 204;
  apiErrorRate.add(!ok, { name: "auth_logout" });
  check(response, { "auth_logout: 204": () => ok });
  if (ok) {
    state.closed = true;
    sessionLogouts.add(1);
  }
}

function readEndpoint(path, name, token) {
  const response = http.get(`${baseUrl}${path}`, {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    timeout: "5s",
    tags: { name },
  });
  const ok = expectedApiStatus(response, name);
  readDuration.add(response.timings.duration, { name });
  return ok;
}

export function setup() {
  if (!__ENV.BASE_URL) throw new Error("Define BASE_URL explícitamente; no se inició carga.");
  if (!__ENV.STRESS_FIXTURE_FILE) throw new Error("Define STRESS_FIXTURE_FILE con una fixture local externa; no se inició carga.");
  validateFixtureDocument(fixtureDocument, profile.maxVus);
  validateBaseUrl(__ENV.BASE_URL, {
    allowRemoteTesting: __ENV.STRESS_ALLOW_REMOTE === "true",
    confirmTarget: __ENV.STRESS_CONFIRM_TARGET || "",
  });

  const live = http.get(`${baseUrl}/v1/health/ready`, { timeout: "5s", tags: { name: "preflight_ready" } });
  if (live.status !== 200) throw new Error(`Preflight readiness rechazó el destino (HTTP ${live.status}); no se inició carga.`);
  let version;
  try {
    const versionResponse = http.get(`${baseUrl}/v1/version`, {
      timeout: "5s",
      responseType: "text",
      tags: { name: "preflight_version" },
    });
    if (versionResponse.status !== 200) throw new Error(`Preflight /v1/version rechazó el destino (HTTP ${versionResponse.status}).`);
    version = versionResponse.json();
  } catch {
    throw new Error("Preflight /v1/version devolvió una respuesta inválida.");
  }
  if (!version || typeof version.schema_version !== "number") throw new Error("Preflight /v1/version no informó schema_version.");
  if (__ENV.STRESS_EXPECT_SCHEMA && version.schema_version !== Number(__ENV.STRESS_EXPECT_SCHEMA)) {
    throw new Error(`Schema observado ${version.schema_version} difiere del esperado; no se inició carga.`);
  }

  if (profileName !== "smoke") {
    let summary;
    try {
      summary = JSON.parse(open(__ENV.STRESS_SMOKE_SUMMARY));
      evaluateSmokeSummary(summary);
    } catch (error) {
      throw new Error(`El summary smoke previo no pasó el gate: ${error.message}`);
    }
  }

  const now = Date.now();
  const plannedEnd = now + profile.durationMs;
  const refreshLifetimeMs = 30 * 24 * 60 * 60 * 1000;
  const runId = `${new Date(now).toISOString().replace(/[:.]/g, "-")}-${profileName}`;
  console.log(`stress_run=${runId} profile=${profileName} target=${originHost(baseUrl).hostname} version=${version.version || "unknown"} commit=${version.commit || "unknown"} schema=${version.schema_version} vus_cap=${profile.maxVus} planned_end_utc=${new Date(plannedEnd).toISOString()} maximum_unrevoked_session_expiry_utc=${new Date(plannedEnd + refreshLifetimeMs).toISOString()}`);
  return { plannedEnd, maximumExpiry: new Date(plannedEnd + refreshLifetimeMs).toISOString(), runId };
}

export function authenticatedReadJourney(run) {
  const identity = credentialsByVu.get(__VU);
  if (!identity) throw new Error(`No hay fixture para VU ${__VU}; se aborta sin iniciar requests de carga.`);
  let state = ensureSession(identity);

  // Keep the final iteration available for revoking this VU's current rotating session.
  if (Date.now() >= run.plannedEnd - 12_000) {
    logout(state);
    return;
  }

  const workflowStart = Date.now();
  let flowOk = true;
  flowOk = readEndpoint("/v1/me", "profile_me", state.accessToken) && flowOk;
  flowOk = readEndpoint("/v1/clientes/me", "customer_me", state.accessToken) && flowOk;
  flowOk = readEndpoint("/v1/clientes/me/tarjetas?page=1&page_size=20", "cards", state.accessToken) && flowOk;
  flowOk = readEndpoint("/v1/clientes/me/movimientos?page=1&page_size=20", "movements", state.accessToken) && flowOk;
  workflowDuration.add(Date.now() - workflowStart, { name: "authenticated_read_workflow" });
  check({ flowOk }, { "read workflow: cuatro endpoints respondieron correctamente": (result) => result.flowOk });
  if (Date.now() >= run.plannedEnd - 12_000) logout(state);
  else sleep(1);
}

export function teardown(run) {
  console.log(`stress_run=${run.runId} finished; logout_count es métrica session_logout. Si hubo abort/interrupción antes del cierre por VU, sesiones no revocadas pueden seguir válidas hasta ${run.maximumExpiry}; revisar/vencer por identidad fixture mediante el procedimiento operativo, sin imprimir tokens.`);
}
