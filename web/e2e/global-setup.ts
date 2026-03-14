import { execSync, spawn, type ChildProcess } from "child_process";
import { mkdtempSync, writeFileSync, existsSync } from "fs";
import { tmpdir } from "os";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const STATE_FILE = join(tmpdir(), "ari-e2e-state.json");

interface E2EState {
  pid: number;
  baseURL: string;
  dataDir: string;
  adminEmail: string;
  adminPassword: string;
  adminName: string;
}

async function globalSetup() {
  const projectRoot = join(__dirname, "..", "..");
  const webDir = join(projectRoot, "web");

  // 1. Build frontend
  console.log("[e2e] Building frontend...");
  execSync("npm run build", { cwd: webDir, stdio: "inherit" });

  // 2. Build Go binary
  console.log("[e2e] Building Go binary...");
  const binPath = join(tmpdir(), "ari-e2e-bin");
  execSync(`go build -o ${binPath} ./cmd/ari`, {
    cwd: projectRoot,
    stdio: "inherit",
  });

  // 3. Start server
  const dataDir = mkdtempSync(join(tmpdir(), "ari-e2e-data-"));
  // Use E2E_PORT env var or default 3199 — must match playwright.config.ts
  const port = parseInt(process.env.E2E_PORT || "3199", 10);
  const pgPort = 15432 + Math.floor(Math.random() * 10000);
  const baseURL = `http://127.0.0.1:${port}`;

  console.log(`[e2e] Starting server on ${baseURL}...`);
  console.log(`[e2e] Data dir: ${dataDir}`);
  const server: ChildProcess = spawn(binPath, ["run"], {
    env: {
      ...process.env,
      ARI_PORT: String(port),
      ARI_HOST: "127.0.0.1",
      ARI_DATA_DIR: dataDir,
      ARI_DATABASE_URL: "",
      ARI_DEPLOYMENT_MODE: "authenticated",
      ARI_EMBEDDED_PG_PORT: String(pgPort),
      ARI_ENV: "development",
    },
    stdio: "pipe",
    detached: true,
  });

  server.unref();

  // Always log server output during setup for debugging startup issues
  server.stdout?.on("data", (data: Buffer) => {
    console.log(`[server:stdout] ${data.toString().trim()}`);
  });
  server.stderr?.on("data", (data: Buffer) => {
    console.log(`[server:stderr] ${data.toString().trim()}`);
  });

  server.on("error", (err) => {
    console.error(`[server:error] ${err.message}`);
  });

  server.on("exit", (code, signal) => {
    console.log(`[server:exit] code=${code} signal=${signal}`);
  });

  // 4. Wait for health check
  console.log("[e2e] Waiting for server to become healthy...");
  const deadline = Date.now() + 120_000;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(`${baseURL}/api/health`);
      if (resp.ok) {
        console.log("[e2e] Server is healthy!");
        break;
      }
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }

  if (Date.now() >= deadline) {
    server.kill();
    throw new Error("Server did not become healthy within 120s");
  }

  // 5. Seed admin user
  const adminEmail = "admin@e2e.test";
  const adminPassword = "TestP@ss1234!";
  const adminName = "E2E Admin";

  const registerResp = await fetch(`${baseURL}/api/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: adminEmail,
      displayName: adminName,
      password: adminPassword,
    }),
  });

  if (!registerResp.ok) {
    const body = await registerResp.text();
    server.kill();
    throw new Error(`Failed to seed admin user: ${registerResp.status} ${body}`);
  }

  console.log("[e2e] Admin user seeded");

  // 6. Save state
  const state: E2EState = {
    pid: server.pid!,
    baseURL,
    dataDir,
    adminEmail,
    adminPassword,
    adminName,
  };

  writeFileSync(STATE_FILE, JSON.stringify(state));
  process.env.E2E_BASE_URL = baseURL;
  process.env.E2E_ADMIN_EMAIL = adminEmail;
  process.env.E2E_ADMIN_PASSWORD = adminPassword;
  process.env.E2E_ADMIN_NAME = adminName;

  console.log("[e2e] Global setup complete");
}

export default globalSetup;
