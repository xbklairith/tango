import { readFileSync, rmSync, existsSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

const STATE_FILE = join(tmpdir(), "ari-e2e-state.json");

async function globalTeardown() {
  // Skip teardown if KEEP_SERVER is set
  if (process.env.KEEP_SERVER) {
    console.log("[e2e] KEEP_SERVER set — skipping teardown, server stays running");
    return;
  }

  if (!existsSync(STATE_FILE)) {
    console.log("[e2e] No state file found, skipping teardown");
    return;
  }

  const state = JSON.parse(readFileSync(STATE_FILE, "utf-8"));

  // Kill server
  try {
    process.kill(state.pid, "SIGTERM");
    console.log(`[e2e] Killed server PID ${state.pid}`);
  } catch {
    // already dead
  }

  // Clean up realm DB dir (postgres data) — keeps secrets/config intact for faster re-runs
  if (state.ariHome && state.realmID) {
    const realmDBDir = join(state.ariHome, "realms", state.realmID, "db");
    try {
      rmSync(realmDBDir, { recursive: true, force: true });
      console.log(`[e2e] Cleaned up ${realmDBDir}`);
    } catch {
      // ignore
    }
  }

  // Clean up binary
  const binPath = join(tmpdir(), "ari-e2e-bin");
  try {
    rmSync(binPath, { force: true });
  } catch {
    // ignore
  }

  // Clean up state file
  rmSync(STATE_FILE, { force: true });

  console.log("[e2e] Global teardown complete");
}

export default globalTeardown;
