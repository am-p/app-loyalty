import test from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { getEventListeners } from "node:events";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { createHealthMonitor, validateWebhookUrl, wait } from "./monitor-health.mjs";

test("completed poll waits do not retain abort listeners across repeated cycles", async () => {
  const controller = new AbortController();
  const existingListener = () => {};
  controller.signal.addEventListener("abort", existingListener);
  for (let cycle = 0; cycle < 25; cycle++) await wait(0, controller.signal);
  assert.deepEqual(getEventListeners(controller.signal, "abort"), [existingListener]);
  controller.signal.removeEventListener("abort", existingListener);
});

test("an already aborted wait does not allocate a timer or listener", async (t) => {
  const controller = new AbortController();
  controller.abort();
  const timers = t.mock.method(globalThis, "setTimeout");
  await wait(60_000, controller.signal);
  assert.equal(timers.mock.calls.length, 0);
  assert.equal(getEventListeners(controller.signal, "abort").length, 0);
});

test("aborting concurrent waits cancels their timers and removes only their listeners", async (t) => {
  const controller = new AbortController();
  const existingListener = () => {};
  controller.signal.addEventListener("abort", existingListener);
  const timers = t.mock.method(globalThis, "setTimeout");
  const cleared = t.mock.method(globalThis, "clearTimeout");
  const completed = wait(0, controller.signal);
  const pending = [wait(60_000, controller.signal), wait(60_000, controller.signal)];
  const handles = timers.mock.calls.map((call) => call.result);
  await completed;
  assert.equal(getEventListeners(controller.signal, "abort").length, 3);
  controller.abort();
  await Promise.all(pending);
  assert.deepEqual(getEventListeners(controller.signal, "abort"), [existingListener]);
  for (const handle of handles) {
    assert.ok(cleared.mock.calls.some((call) => call.arguments[0] === handle), "each wait must cancel its own timer");
  }
  controller.signal.removeEventListener("abort", existingListener);
});

test("SIGTERM stops the real monitor process during its poll wait cleanly", { timeout: 5_000 }, async (t) => {
  const api = await listen((request, response) => {
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify(request.url === "/v1/version"
      ? { version: "local", commit: "test", schema_version: "0035" }
      : { status: "ok" }));
  });
  t.after(() => close(api.server));
  const child = spawn(process.execPath, [fileURLToPath(new URL("./monitor-health.mjs", import.meta.url))], {
    env: { BASE_URL: api.url, STRESS_CONFIRM_TARGET: "local", MONITOR_INTERVAL_SECONDS: "30" },
    stdio: ["ignore", "pipe", "pipe"],
  });
  t.after(() => { if (child.exitCode === null) child.kill("SIGKILL"); });
  let stderr = "";
  child.stderr.setEncoding("utf8").on("data", (chunk) => { stderr += chunk; });
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => resolve({ code, signal }));
  });
  await new Promise((resolve, reject) => {
    child.stdout.setEncoding("utf8");
    let stdout = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      if (stdout.includes('"kind":"health_probe"') && stdout.includes('"status":"healthy"')) resolve();
    });
    child.once("error", reject);
    child.once("close", () => reject(new Error("monitor exited before its first healthy poll")));
  });
  const stoppedAt = performance.now();
  assert.equal(child.kill("SIGTERM"), true);
  assert.deepEqual(await exited, { code: 0, signal: null });
  assert.ok(performance.now() - stoppedAt < 1_500, "shutdown must cancel the thirty-second wait");
  assert.equal(stderr, "");
});

async function listen(handler) {
  const server = createServer(handler);
  server.listen(0, "127.0.0.1");
  await new Promise((resolve, reject) => {
    server.once("listening", resolve);
    server.once("error", reject);
  });
  const address = server.address();
  return { server, url: `http://127.0.0.1:${address.port}` };
}

async function close(server) {
  const closed = new Promise((resolve, reject) => {
    if (!server.listening) return resolve();
    server.once("close", resolve);
    server.once("error", reject);
  });
  server.closeAllConnections();
  server.close();
  await closed;
}

test("emits bounded startup, unhealthy, and recovery alerts once per transition", async (t) => {
  let readinessStatus = 503;
  const api = await listen((request, response) => {
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/v1/health/ready") {
      response.writeHead(readinessStatus);
      response.end(JSON.stringify({ status: readinessStatus === 200 ? "ok" : "unavailable" }));
    } else if (request.url === "/v1/version") {
      response.writeHead(200);
      response.end(JSON.stringify({ version: "v".repeat(3_000), commit: "c".repeat(3_000), schema_version: "0035" }));
    } else {
      response.writeHead(404).end();
    }
  });
  t.after(() => close(api.server));

  const received = [];
  const webhook = await listen(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    received.push({ method: request.method, contentType: request.headers["content-type"], body: Buffer.concat(chunks).toString("utf8") });
    response.writeHead(204).end();
  });
  t.after(() => close(webhook.server));

  const emitted = [];
  const monitor = createHealthMonitor({
    baseUrl: api.url,
    confirmTarget: "local",
    webhookUrl: `${webhook.url}/capture`,
    emit: (entry) => emitted.push(entry),
  });

  assert.equal((await monitor.poll()).event, "startup_failure");
  assert.equal((await monitor.poll()).event, null, "repeat failure must not duplicate the alert");
  readinessStatus = 200;
  assert.equal((await monitor.poll()).event, "recovery");
  readinessStatus = 503;
  assert.equal((await monitor.poll()).event, "unhealthy");
  assert.equal((await monitor.poll()).event, null, "persistent unhealthy state must be deduplicated");
  readinessStatus = 200;
  assert.equal((await monitor.poll()).event, "recovery");

  assert.deepEqual(received.map((entry) => JSON.parse(entry.body).event), ["startup_failure", "recovery", "unhealthy", "recovery"]);
  assert.ok(received.every((entry) => entry.method === "POST" && entry.contentType === "application/json"));
  assert.ok(received.every((entry) => Buffer.byteLength(entry.body) <= 2048));
  assert.ok(received.every((entry) => !/password|token|secret|authorization/i.test(entry.body)));
  assert.ok(received.every((entry) => JSON.parse(entry.body).checks.version.version.length <= 96));
  assert.ok(received.every((entry) => JSON.parse(entry.body).checks.version.commit.length <= 96));
  assert.ok(received.every((entry) => JSON.parse(entry.body).checks.version.schema_version === "0035"));
  assert.equal(emitted.filter((entry) => entry.kind === "webhook_delivery" && entry.status === "delivered").length, 4);
});

test("rejects any webhook that is not plain HTTP loopback", () => {
  assert.equal(validateWebhookUrl("http://127.0.0.1:9999/alerts"), "http://127.0.0.1:9999/alerts");
  for (const url of ["https://127.0.0.1/hook", "http://example.com/hook", "http://user:pass@localhost/hook", "http://localhost/hook?token=abc", "http://192.168.1.10/hook"]) {
    assert.throws(() => validateWebhookUrl(url), /solo permite HTTP loopback/);
  }
});

test("accepts the real API version schema string and rejects malformed schema values", async () => {
  let schema = "0035";
  const monitor = createHealthMonitor({
    baseUrl: "http://127.0.0.1:55441",
    confirmTarget: "local",
    emit: () => {},
    fetchImpl: async (url) => Response.json(url.endsWith("/v1/version")
      ? { version: "stress-integrated-20261007", commit: "04cd4b2", schema_version: schema }
      : { status: "ok" }),
  });
  const healthy = await monitor.poll();
  assert.equal(healthy.status, "healthy");
  assert.equal(healthy.version.schema_version, "0035", "leading zeros must be preserved");
  for (schema of [35, 34, 3.5, null, undefined, false, {}, [], "35", "035", "00035", " 0035", "0035\n", "00a5"]) {
    const result = await monitor.poll();
    assert.equal(result.status, "unhealthy");
    assert.equal(result.version.reason, "version_invalid");
    assert.equal(result.version.schema_version, undefined);
  }
  schema = "0035";
  assert.equal((await monitor.poll()).event, "recovery");
});

test("journals dependency failure and recovery once without a webhook", async (t) => {
  let readinessStatus = 200;
  let postRequests = 0;
  const api = await listen((request, response) => {
    if (request.method === "POST") postRequests++;
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/v1/version") {
      response.end(JSON.stringify({ version: "stress-integrated-20261007", commit: "04cd4b2", schema_version: "0035" }));
    } else {
      response.writeHead(readinessStatus);
      response.end(JSON.stringify({ status: readinessStatus === 200 ? "ok" : "unavailable" }));
    }
  });
  t.after(() => close(api.server));
  const entries = [];
  const monitor = createHealthMonitor({ baseUrl: api.url, confirmTarget: "local", emit: (entry) => entries.push(entry) });
  assert.equal((await monitor.poll()).event, null);
  readinessStatus = 503;
  assert.equal((await monitor.poll()).event, "unhealthy");
  assert.equal((await monitor.poll()).event, null);
  readinessStatus = 200;
  assert.equal((await monitor.poll()).event, "recovery");
  assert.equal((await monitor.poll()).event, null);
  assert.deepEqual(entries.filter((entry) => entry.event).map((entry) => entry.event), ["unhealthy", "recovery"]);
  assert.equal(postRequests, 0);
  assert.equal(entries.filter((entry) => entry.kind === "webhook_delivery").length, 0);
});

test("reports an endpoint timeout without exposing response content", async (t) => {
  const api = await listen((request, response) => {
    if (request.url === "/v1/health/ready") {
      return;
    }
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ version: "local", commit: "test", schema_version: "0035" }));
  });
  t.after(() => close(api.server));

  const monitor = createHealthMonitor({ baseUrl: api.url, confirmTarget: "local", timeoutMs: 25, emit: () => {} });
  const result = await monitor.poll();
  assert.equal(result.status, "unhealthy");
  assert.equal(result.readiness.reason, "timeout");
});

test("allows no external API origin without the explicit testing gate", () => {
  assert.throws(() => createHealthMonitor({ baseUrl: "https://puntazo.pro", confirmTarget: "puntazo.pro" }), /Destino bloqueado/);
  assert.throws(() => createHealthMonitor({ baseUrl: "https://api-testing.puntazo.pro" }), /STRESS_ALLOW_REMOTE/);
});
