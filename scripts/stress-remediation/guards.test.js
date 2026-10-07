import test from "node:test";
import assert from "node:assert/strict";
import {
  evaluateSmokeSummary,
  validateBaseUrl,
  validateFixtureDocument,
  validateProfile,
} from "./guards.js";

const fixture = (emails = ["load-1@example.test", "load-2@example.test", "load-3@example.test"]) => ({
  schema_version: 1,
  identities: emails.map((email, index) => ({ vu: index + 1, email, password: "synthetic-password-long", account_type: "CLIENTE_FINAL" })),
});

const passingSummary = () => ({ metrics: {
  api_error_rate: { values: { rate: 0.001 } },
  checks: { values: { rate: 0.999 } },
  api_read_duration: { values: { "p(95)": 410, "p(99)": 890 } },
  workflow_duration: { values: { "p(95)": 2200 } },
} });

test("allows loopback without remote acknowledgements", () => {
  assert.throws(() => validateBaseUrl("http://127.0.0.1:8080"), /STRESS_CONFIRM_TARGET=local/);
  assert.equal(validateBaseUrl("http://127.0.0.1:8080", { confirmTarget: "local" }), "http://127.0.0.1:8080");
});

test("allows only explicitly confirmed HTTPS testing target", () => {
  assert.throws(() => validateBaseUrl("https://testing.puntazo.pro"), /STRESS_ALLOW_REMOTE/);
  assert.equal(validateBaseUrl("https://testing.puntazo.pro", {
    allowRemoteTesting: true,
    confirmTarget: "testing.puntazo.pro",
  }), "https://testing.puntazo.pro");
});

test("blocks production, unapproved hosts, paths, and embedded credentials", () => {
  assert.throws(() => validateBaseUrl("https://puntazo.pro", { allowRemoteTesting: true, confirmTarget: "puntazo.pro" }), /Destino bloqueado/);
  assert.throws(() => validateBaseUrl("https://api.example.com"), /Destino bloqueado/);
  assert.throws(() => validateBaseUrl("https://testing.puntazo.pro/v1"), /path/);
  assert.throws(() => validateBaseUrl("https://user:secret@localhost"), /credenciales/);
});

test("requires a distinct fixture identity for every VU", () => {
  assert.equal(validateFixtureDocument(fixture(), 3), true);
  assert.throws(() => validateFixtureDocument(fixture(["same@example.test", "same@example.test"]), 2), /independiente/);
  const incompleteFixture = fixture();
  incompleteFixture.identities = incompleteFixture.identities.slice(0, 2);
  assert.throws(() => validateFixtureDocument(incompleteFixture, 3), /identidad exclusiva/);
  assert.throws(() => validateFixtureDocument({ schema_version: 1, identities: [{ vu: 1, email: "x@example.invalid", password: "REPLACE_ME", account_type: "CLIENTE_FINAL" }] }, 1), /fixture de ejemplo/);
});

test("gates later profiles on explicit opt-in and a passing smoke summary", () => {
  assert.equal(validateProfile("smoke"), "smoke");
  assert.throws(() => validateProfile("sustained", { STRESS_SMOKE_SUMMARY: "/tmp/smoke.json" }), /SMOKE_ACCEPTED/);
  assert.throws(() => validateProfile("soak", { STRESS_SMOKE_ACCEPTED: "true", STRESS_SMOKE_SUMMARY: "/tmp/smoke.json" }), /STRESS_ENABLE_SOAK/);
  assert.deepEqual(evaluateSmokeSummary(passingSummary()), {
    apiErrorRate: 0.001,
    checksRate: 0.999,
    apiP95: 410,
    apiP99: 890,
    workflowP95: 2200,
  });
  const failing = passingSummary();
  failing.metrics.api_read_duration.values["p(99)"] = 1000;
  assert.throws(() => evaluateSmokeSummary(failing), /api_read_duration p\(99\)/);
});
