"""Capture agent detail with mock console entries for advertising."""
import time
from playwright.sync_api import sync_playwright

AGENT_ID = "03bd033f-5cdb-43a0-a3d6-bef2ea44bd2c"
BASE = "http://localhost:5173"
OUT = "/Users/xb/builder/ari/screenshots"

MOCK_HTML = """
<div style="display:flex;flex-direction:column;gap:2px">

<div style="display:flex;align-items:flex-start;gap:8px;padding:4px 0;border-bottom:1px solid rgba(255,255,255,0.05)">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:01</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&bull;</span>
  <span style="font-size:12px;color:#60a5fa;font-weight:500">Status: active &rarr; running</span>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:4px 0;border-bottom:1px solid rgba(255,255,255,0.05)">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:01</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&bull;</span>
  <span style="font-size:12px;color:#60a5fa;font-weight:500">Run started (source: assignment)</span>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:02</span>
  <span style="color:#f472b6;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#129504;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">THINK</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">Analyzing task ALP-1: Implement user authentication</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:03</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#128196;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">READ</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">internal/auth/middleware.go</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:03</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#128196;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">READ</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">internal/auth/jwt.go</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:04</span>
  <span style="color:#facc15;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#128269;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">SEARCH</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">grep -r "AuthHandler" internal/server/handlers/</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:05</span>
  <span style="color:#f472b6;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#129504;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">THINK</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">Need to add JWT validation and session management</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:06</span>
  <span style="color:#4ade80;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#9999;&#65039;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">WRITE</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">internal/auth/middleware.go <span style="color:#4ade80">+42 lines</span></span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:07</span>
  <span style="color:#fb923c;font-size:14px;width:20px;text-align:center;flex-shrink:0">&plusmn;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">EDIT</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">internal/server/server.go <span style="color:#4ade80">+3</span> <span style="color:#f87171">-1</span></span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:08</span>
  <span style="color:#22d3ee;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#9654;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">RUN</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">go test ./internal/auth/ -v</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:10</span>
  <span style="color:#9ca3af;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#10095;</span>
  <span style="font-size:12px;font-family:monospace;color:#4ade80">ok &emsp;github.com/xb/ari/internal/auth&emsp;42/42 tests passed</span>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:2px 0">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:11</span>
  <span style="color:#a78bfa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#127760;</span>
  <div><span style="font-size:11px;font-weight:500;color:#6b7280;text-transform:uppercase;letter-spacing:0.05em;margin-right:8px">API</span><span style="font-size:12px;font-family:monospace;color:#d1d5db">PATCH /api/agent/me/task {"status": "done"}</span></div>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:4px 0;border-top:1px solid rgba(255,255,255,0.05)">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:11</span>
  <span style="color:#4ade80;font-size:14px;width:20px;text-align:center;flex-shrink:0">&#10004;</span>
  <span style="font-size:12px;color:#4ade80;font-weight:500">Task ALP-1 marked as done</span>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:4px 0;border-top:1px solid rgba(255,255,255,0.05)">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:11</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&bull;</span>
  <span style="font-size:12px;color:#60a5fa;font-weight:500">Run succeeded (exit code: 0)</span>
</div>

<div style="display:flex;align-items:flex-start;gap:8px;padding:4px 0;border-top:1px solid rgba(255,255,255,0.05)">
  <span style="flex-shrink:0;width:62px;font-size:11px;color:#4b5563;font-family:monospace">13:52:12</span>
  <span style="color:#60a5fa;font-size:14px;width:20px;text-align:center;flex-shrink:0">&bull;</span>
  <span style="font-size:12px;color:#60a5fa;font-weight:500">Status: running &rarr; idle</span>
</div>

</div>
"""

def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(
            viewport={"width": 1440, "height": 900},
            device_scale_factor=2,
        )
        page = ctx.new_page()
        page.goto(f"{BASE}/agents/{AGENT_ID}")
        page.wait_for_load_state("networkidle")
        time.sleep(2)

        # Inject mock console entries
        page.evaluate(f"""() => {{
            const container = document.querySelector('[class*="font-mono"][class*="min-h-"]');
            if (container) {{
                container.innerHTML = `{MOCK_HTML}`;
            }}
        }}""")

        time.sleep(0.5)
        page.screenshot(path=f"{OUT}/03-agent-detail.png", full_page=True)
        print("Agent detail with mock console captured")
        browser.close()

if __name__ == "__main__":
    main()
