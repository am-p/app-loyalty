import { readFileSync } from "node:fs";
import { evaluateSmokeSummary } from "./guards.js";

const summaryPath = process.argv[2];
if (!summaryPath) {
  console.error("Uso: node summary-check.js /ruta/results/smoke.json");
  process.exit(2);
}

try {
  const values = evaluateSmokeSummary(JSON.parse(readFileSync(summaryPath, "utf8")));
  console.log(`Smoke gate aprobado: api_error_rate=${values.apiErrorRate}; checks=${values.checksRate}; API read p95=${values.apiP95} ms; p99=${values.apiP99} ms; workflow p95=${values.workflowP95} ms.`);
} catch (error) {
  console.error(`Smoke gate rechazado: ${error.message}`);
  process.exit(1);
}
