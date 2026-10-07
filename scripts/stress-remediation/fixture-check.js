import { readFileSync } from "node:fs";
import { validateFixtureDocument, validateProfile } from "./guards.js";

const profileLimits = { smoke: 3, sustained: 3, spike: 5, soak: 3 };
const [fixturePath, profileName] = process.argv.slice(2);
if (!fixturePath || !profileLimits[profileName]) {
  console.error("Uso: node fixture-check.js /ruta/fixtures.local.json smoke|sustained|spike|soak");
  process.exit(2);
}

try {
  validateProfile(profileName, {
    STRESS_ENABLE_SOAK: process.env.STRESS_ENABLE_SOAK,
    STRESS_SMOKE_ACCEPTED: process.env.STRESS_SMOKE_ACCEPTED,
    STRESS_SMOKE_SUMMARY: process.env.STRESS_SMOKE_SUMMARY || "preflight-only",
  });
  const fixture = JSON.parse(readFileSync(fixturePath, "utf8"));
  validateFixtureDocument(fixture, profileLimits[profileName]);
  console.log(`Fixture válida: ${profileLimits[profileName]} identidades independientes para ${profileName}.`);
} catch (error) {
  console.error(`Fixture/preflight inválido: ${error.message}`);
  process.exit(1);
}
