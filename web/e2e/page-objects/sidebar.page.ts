import type { Page, Locator } from "@playwright/test";

export class SidebarPage {
  readonly page: Page;
  readonly dashboardLink: Locator;
  readonly squadsLink: Locator;
  readonly agentsLink: Locator;
  readonly issuesLink: Locator;
  readonly projectsLink: Locator;
  readonly goalsLink: Locator;
  readonly logoutButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.dashboardLink = page.getByRole("link", { name: "Dashboard" });
    this.squadsLink = page.getByRole("link", { name: "Squads" });
    this.agentsLink = page.getByRole("link", { name: "Agents" });
    this.issuesLink = page.getByRole("link", { name: "Issues" });
    this.projectsLink = page.getByRole("link", { name: "Projects" });
    this.goalsLink = page.getByRole("link", { name: "Goals" });
    this.logoutButton = page.getByTitle("Logout");
  }

  async navigateTo(
    section: "Dashboard" | "Squads" | "Agents" | "Issues" | "Projects" | "Goals",
  ) {
    const link = this.page.getByRole("link", { name: section });
    await link.click();
  }

  async getUserName(): Promise<string | null> {
    // User name is displayed in the sidebar footer
    const userText = this.page.locator(
      ".border-t .truncate.text-sm",
    );
    return userText.textContent();
  }

  async logout() {
    await this.logoutButton.click();
  }
}
