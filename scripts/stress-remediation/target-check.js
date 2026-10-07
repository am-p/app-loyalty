import { validateBaseUrl } from "./guards.js";

try {
  const target = validateBaseUrl(process.env.BASE_URL, {
    allowRemoteTesting: process.env.STRESS_ALLOW_REMOTE === "true",
    confirmTarget: process.env.STRESS_CONFIRM_TARGET || "",
  });
  console.log(`Destino permitido: ${new URL(target).hostname}.`);
} catch (error) {
  console.error(`Destino bloqueado: ${error.message}`);
  process.exit(1);
}
