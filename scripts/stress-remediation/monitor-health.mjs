#!/usr/bin/env node

import { validateBaseUrl } from "./guards.js";

const DEFAULT_INTERVAL_MS = 30_000;
const REQUEST_TIMEOUT_MS = 3_000;
const MAX_RESPONSE_BYTES = 8 * 1024;
const MAX_ALERT_BYTES = 2 * 1024;
const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "::1", "[::1]"]);

export function validateWebhookUrl(raw) {
  if (!raw) return null;
  let url;
  try {
    url = new URL(raw);
  } catch {
    throw new Error("WEBHOOK_URL debe ser una URL absoluta loopback.");
  }
  if (url.protocol !== "http:" || !LOCAL_HOSTS.has(url.hostname.toLowerCase()) || url.username || url.password || url.search || url.hash) {
    throw new Error("WEBHOOK_URL solo permite HTTP loopback sin credenciales, query ni fragmento.");
  }
  return url.toString();
}

function boundedText(value, max = 96) {
  return typeof value === "string" ? value.slice(0, max) : "";
}

async function readBoundedJson(response, maxBytes = MAX_RESPONSE_BYTES) {
  if (!response.body) return { ok: false, reason: "body_missing" };
  const reader = response.body.getReader();
  const chunks = [];
  let length = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > maxBytes) {
        await reader.cancel();
        return { ok: false, reason: "body_too_large" };
      }
      chunks.push(value);
    }
  } catch {
    return { ok: false, reason: "body_unreadable" };
  }
  try {
    const bytes = new Uint8Array(length);
    let offset = 0;
    for (const chunk of chunks) {
      bytes.set(chunk, offset);
      offset += chunk.byteLength;
    }
    return { ok: true, value: JSON.parse(new TextDecoder().decode(bytes)) };
  } catch {
    return { ok: false, reason: "json_invalid" };
  }
}

async function timedFetch(fetchImpl, url, options, timeoutMs) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetchImpl(url, { redirect: "error", ...options, signal: controller.signal });
  } catch (error) {
    return { status: null, errorKind: error?.name === "AbortError" ? "timeout" : "network_error" };
  } finally {
    clearTimeout(timer);
  }
}

async function checkEndpoint(fetchImpl, url, timeoutMs, versionEndpoint = false) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  let response;
  try {
    response = await fetchImpl(url, { method: "GET", headers: { Accept: "application/json" }, redirect: "error", signal: controller.signal });
    if (response.status !== 200) {
      await response.body?.cancel().catch(() => {});
      return { status: response.status, ok: false, reason: "http_status" };
    }
    const body = await readBoundedJson(response);
    if (!body.ok) return { status: response.status, ok: false, reason: controller.signal.aborted ? "timeout" : body.reason };
    if (versionEndpoint) {
      const value = body.value;
      if (!value || typeof value !== "object" || typeof value.version !== "string" || typeof value.commit !== "string" || !Number.isInteger(value.schema_version)) {
        return { status: response.status, ok: false, reason: "version_invalid" };
      }
      return {
        status: response.status,
        ok: true,
        version: boundedText(value.version),
        commit: boundedText(value.commit),
        schema_version: value.schema_version,
      };
    }
    if (!body.value || typeof body.value !== "object" || body.value.status !== "ok") {
      return { status: response.status, ok: false, reason: "readiness_invalid" };
    }
    return { status: response.status, ok: true };
  } catch (error) {
    return { status: response?.status ?? null, ok: false, reason: controller.signal.aborted || error?.name === "AbortError" ? "timeout" : "network_error" };
  } finally {
    clearTimeout(timer);
  }
}

function alertPayload(event, origin, probe, now) {
  const payload = {
    schema_version: 1,
    service: "puntazo-api-readiness",
    event,
    status: event === "recovery" ? "healthy" : "unhealthy",
    occurred_at: now().toISOString(),
    target: origin,
    checks: {
      readiness: { status: probe.readiness.status, reason: probe.readiness.ok ? null : probe.readiness.reason },
      version: {
        status: probe.version.status,
        reason: probe.version.ok ? null : probe.version.reason,
        version: probe.version.version || null,
        commit: probe.version.commit || null,
        schema_version: probe.version.schema_version ?? null,
      },
    },
  };
  let json = JSON.stringify(payload);
  if (Buffer.byteLength(json) > MAX_ALERT_BYTES) {
    payload.checks.version.version = null;
    payload.checks.version.commit = null;
    json = JSON.stringify(payload);
  }
  if (Buffer.byteLength(json) > MAX_ALERT_BYTES) throw new Error("alert payload exceeded its hard size bound");
  return { payload, json };
}

export function createHealthMonitor({
  baseUrl,
  confirmTarget = "",
  allowRemoteTesting = false,
  webhookUrl,
  fetchImpl = fetch,
  emit = (entry) => process.stdout.write(`${JSON.stringify(entry)}\n`),
  now = () => new Date(),
  timeoutMs = REQUEST_TIMEOUT_MS,
} = {}) {
  const origin = validateBaseUrl(baseUrl, { confirmTarget, allowRemoteTesting });
  const webhook = validateWebhookUrl(webhookUrl);
  const target = new URL(origin);
  const safeOrigin = { scheme: target.protocol.slice(0, -1), host: target.hostname, port: target.port || null };
  let state = "starting";

  async function poll() {
    const [readiness, version] = await Promise.all([
      checkEndpoint(fetchImpl, `${origin}/v1/health/ready`, timeoutMs),
      checkEndpoint(fetchImpl, `${origin}/v1/version`, timeoutMs, true),
    ]);
    const healthy = readiness.ok && version.ok;
    const nextState = healthy ? "healthy" : "unhealthy";
    let event = null;
    if (state === "starting" && !healthy) event = "startup_failure";
    else if (state === "healthy" && !healthy) event = "unhealthy";
    else if (state === "unhealthy" && healthy) event = "recovery";
    state = nextState;

    const result = {
      kind: "health_probe",
      status: nextState,
      checked_at: now().toISOString(),
      target: safeOrigin,
      checks: { readiness, version },
    };
    emit(result);
    if (event) {
      const { payload, json } = alertPayload(event, safeOrigin, { readiness, version }, now);
      emit(payload);
      if (webhook) {
        const response = await timedFetch(fetchImpl, webhook, {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: json,
        }, timeoutMs);
        await response.body?.cancel().catch(() => {});
        if (response.status === null || response.status < 200 || response.status >= 300) {
          emit({ kind: "webhook_delivery", status: "failed", http_status: response.status, reason: response.errorKind || "http_status" });
        } else {
          emit({ kind: "webhook_delivery", status: "delivered", http_status: response.status });
        }
      }
    }
    return { status: nextState, event, readiness, version };
  }

  return { poll, get state() { return state; } };
}

function wait(ms, signal) {
  return new Promise((resolve) => {
    if (signal?.aborted) return resolve();
    const timer = setTimeout(done, ms);
    function done() {
      signal?.removeEventListener("abort", done);
      resolve();
    }
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      done();
    }, { once: true });
  });
}

export async function runHealthMonitor({ intervalMs = DEFAULT_INTERVAL_MS, signal, ...options } = {}) {
  if (!Number.isInteger(intervalMs) || intervalMs < 1_000 || intervalMs > 3_600_000) {
    throw new Error("MONITOR_INTERVAL_SECONDS debe quedar entre 1 y 3600 segundos.");
  }
  const monitor = createHealthMonitor(options);
  while (!signal?.aborted) {
    await monitor.poll();
    if (signal?.aborted) break;
    await wait(intervalMs, signal);
  }
  return monitor.state;
}

function isMain() {
  return process.argv[1] && new URL(import.meta.url).href === new URL(`file://${process.argv[1]}`).href;
}

if (isMain()) {
  const controller = new AbortController();
  process.once("SIGINT", () => controller.abort());
  process.once("SIGTERM", () => controller.abort());
  const intervalSeconds = Number(process.env.MONITOR_INTERVAL_SECONDS || DEFAULT_INTERVAL_MS / 1000);
  runHealthMonitor({
    baseUrl: process.env.BASE_URL,
    confirmTarget: process.env.STRESS_CONFIRM_TARGET || "",
    allowRemoteTesting: process.env.STRESS_ALLOW_REMOTE === "true",
    webhookUrl: process.env.WEBHOOK_URL,
    intervalMs: intervalSeconds * 1000,
    signal: controller.signal,
  }).catch((error) => {
    process.stderr.write(`${JSON.stringify({ kind: "monitor_startup_error", error: boundedText(error.message) })}\n`);
    process.exitCode = 2;
  });
}
