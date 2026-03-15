"""Capture Ari UI screenshots for advertising."""
import time
from playwright.sync_api import sync_playwright

SQUAD_ID = "e10d19ad-9922-4007-93f7-47c17075d2df"
AGENT_ID = "03bd033f-5cdb-43a0-a3d6-bef2ea44bd2c"
ISSUE_ID = "ce81444b-c84e-43e1-bce8-8cd90929fef8"
BASE = "http://localhost:5173"
OUT = "/Users/xb/builder/ari/screenshots"

def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(
            viewport={"width": 1440, "height": 900},
            device_scale_factor=2,
        )
        page = ctx.new_page()

        # 1. Squad detail (shows sidebar with nav)
        page.goto(f"{BASE}/squads/{SQUAD_ID}")
        page.wait_for_load_state("networkidle")
        time.sleep(1)
        page.screenshot(path=f"{OUT}/01-squad-detail.png")
        print("1/5 Squad detail captured")

        # 2. Agents list page with status indicators
        page.goto(f"{BASE}/squads/{SQUAD_ID}/agents")
        page.wait_for_load_state("networkidle")
        time.sleep(1)
        page.screenshot(path=f"{OUT}/02-agents-list.png")
        print("2/5 Agents list captured")

        # 3. Agent detail page (direct URL)
        page.goto(f"{BASE}/agents/{AGENT_ID}")
        page.wait_for_load_state("networkidle")
        time.sleep(1)
        page.screenshot(path=f"{OUT}/03-agent-detail.png")
        print("3/5 Agent detail captured")

        # 4. Issues list page with filters
        page.goto(f"{BASE}/squads/{SQUAD_ID}/issues")
        page.wait_for_load_state("networkidle")
        time.sleep(1)
        page.screenshot(path=f"{OUT}/04-issues-list.png")
        print("4/5 Issues list captured")

        # 5. Issue detail page (direct URL)
        page.goto(f"{BASE}/issues/{ISSUE_ID}")
        page.wait_for_load_state("networkidle")
        time.sleep(1)
        page.screenshot(path=f"{OUT}/05-issue-detail.png")
        print("5/5 Issue detail captured")

        browser.close()
        print(f"\nAll screenshots saved to {OUT}/")

if __name__ == "__main__":
    main()
