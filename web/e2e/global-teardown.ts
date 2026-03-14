import { readFileSync, rmSync, existsSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

const STATE_FILE = join(tmpdir(), "ari-e2e-state.json");

async function globalTeardown() {
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

  // Clean up temp dir
  try {
    rmSync(state.dataDir, { recursive: true, force: true });
    console.log(`[e2e] Cleaned up ${state.dataDir}`);
  } catch {
    // ignore
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
