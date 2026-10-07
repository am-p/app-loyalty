export const ACCEPTANCE = Object.freeze({
  apiErrorRateMaxExclusive: 0.01,
  checksRateMinExclusive: 0.99,
  apiReadP95MsMaxExclusive: 500,
  apiReadP99MsMaxExclusive: 1000,
  workflowP95MsMaxExclusive: 3000,
});

const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "::1", "[::1]", "host.docker.internal"]);
const TEST_HOSTS = new Set(["testing.puntazo.pro", "api-testing.puntazo.pro"]);

// k6 has no browser URL global. Restrict this parser to plain allowed origins.
export function originHost(raw) {
  const match = /^(https?):\/\/(\[::1\]|[a-zA-Z0-9.-]+)(?::([0-9]{1,5}))?\/?$/.exec(raw || "");
  if (!match || (match[3] && (+match[3] < 1 || +match[3] > 65535))) {
    throw new Error("BASE_URL debe ser un origen absoluto sin credenciales, path, query ni fragmento.");
  }
  return { hostname: match[2].toLowerCase(), protocol: match[1] + ':', origin: `${match[1]}://${match[2].toLowerCase()}${match[3] ? ':' + match[3] : ''}` };
}

export function validateBaseUrl(raw, { allowRemoteTesting = false, confirmTarget = "" } = {}) {
  const url = originHost(raw);
  const hostname = url.hostname;
  if (LOCAL_HOSTS.has(hostname)) {
    if (confirmTarget !== "local") throw new Error("Destino local requiere STRESS_CONFIRM_TARGET=local.");
    return url.origin;
  }
  if (TEST_HOSTS.has(hostname)) {
    if (url.protocol !== "https:") throw new Error("El testing remoto debe usar HTTPS.");
    if (!allowRemoteTesting || confirmTarget !== hostname) {
      throw new Error("Testing remoto requiere STRESS_ALLOW_REMOTE=true y confirmación del hostname exacto.");
    }
    return url.origin;
  }
  throw new Error("Destino bloqueado. La suite solo permite loopback/local o los hosts de testing explícitamente confirmados.");
}

export function validateProfile(profile, env = {}) {
  const allowed = new Set(["smoke", "sustained", "spike", "soak"]);
  if (!allowed.has(profile)) throw new Error("STRESS_PROFILE debe ser smoke, sustained, spike o soak.");
  if (profile === "soak" && env.STRESS_ENABLE_SOAK !== "true") {
    throw new Error("Soak de 2 h requiere STRESS_ENABLE_SOAK=true.");
  }
  if (profile !== "smoke") {
    if (env.STRESS_SMOKE_ACCEPTED !== "true") {
      throw new Error("Perfiles posteriores requieren STRESS_SMOKE_ACCEPTED=true.");
    }
    if (!env.STRESS_SMOKE_SUMMARY) throw new Error("Perfiles posteriores requieren STRESS_SMOKE_SUMMARY.");
  }
  return profile;
}

export function validateFixtureDocument(document, requiredVus, { allowPlaceholders = false } = {}) {
  if (!document || document.schema_version !== 1 || !Array.isArray(document.identities)) {
    throw new Error("Fixture inválido: se espera schema_version=1 e identities[].");
  }
  if (!Number.isInteger(requiredVus) || requiredVus < 1) throw new Error("La cantidad de VUs debe ser positiva.");

  const byVu = new Map();
  const emails = new Set();
  for (const identity of document.identities) {
    if (!identity || !Number.isInteger(identity.vu) || identity.vu < 1 || typeof identity.email !== "string" || typeof identity.password !== "string" || identity.account_type !== "CLIENTE_FINAL") {
      throw new Error("Cada identidad necesita vu, email, password y account_type=CLIENTE_FINAL.");
    }
    const email = identity.email.trim().toLowerCase();
    if (!email.includes("@")) throw new Error("Cada identidad necesita un email válido.");
    if (byVu.has(identity.vu)) throw new Error("Hay más de una identidad asignada al mismo VU.");
    if (emails.has(email)) throw new Error("Cada VU debe tener una identidad/email independiente.");
    if (!allowPlaceholders && (email.endsWith(".invalid") || /replace|changeme|placeholder/i.test(identity.password))) {
      throw new Error("La fixture de ejemplo no puede usarse para carga; prepara una fixture local real.");
    }
    if (!allowPlaceholders && identity.password.length < 8) throw new Error("La contraseña de la fixture es demasiado corta.");
    byVu.set(identity.vu, identity);
    emails.add(email);
  }

  for (let vu = 1; vu <= requiredVus; vu += 1) {
    if (!byVu.has(vu)) throw new Error(`Falta una identidad exclusiva para VU ${vu}.`);
  }
  return true;
}

export function evaluateSmokeSummary(summary) {
  const metrics = summary?.metrics;
  if (!metrics) throw new Error("El summary no contiene metrics; vuelve a exportarlo con --summary-export.");

  const values = (metric) => metric?.values || metric || {};
  const rate = (metric) => values(metric).rate ?? values(metric).value;
  const apiErrorRate = rate(metrics.api_error_rate);
  const checksRate = rate(metrics.checks);
  const apiP95 = values(metrics.api_read_duration)["p(95)"];
  const apiP99 = values(metrics.api_read_duration)["p(99)"];
  const workflowP95 = values(metrics.workflow_duration)["p(95)"];
  const observed = [apiErrorRate, checksRate, apiP95, apiP99, workflowP95];
  if (observed.some((value) => typeof value !== "number" || !Number.isFinite(value))) {
    throw new Error("El summary no trae las cinco métricas requeridas para abrir fases posteriores.");
  }

  const failed = [];
  if (apiErrorRate >= ACCEPTANCE.apiErrorRateMaxExclusive) failed.push("api_error_rate");
  if (checksRate <= ACCEPTANCE.checksRateMinExclusive) failed.push("checks");
  if (apiP95 >= ACCEPTANCE.apiReadP95MsMaxExclusive) failed.push("api_read_duration p(95)");
  if (apiP99 >= ACCEPTANCE.apiReadP99MsMaxExclusive) failed.push("api_read_duration p(99)");
  if (workflowP95 >= ACCEPTANCE.workflowP95MsMaxExclusive) failed.push("workflow_duration p(95)");
  if (failed.length) throw new Error(`Smoke previo no aceptado: ${failed.join(", ")}.`);
  return { apiErrorRate, checksRate, apiP95, apiP99, workflowP95 };
}
