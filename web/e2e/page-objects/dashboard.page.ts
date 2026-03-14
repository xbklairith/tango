import type { Page, Locator } from "@playwright/test";

export class DashboardPage {
  readonly page: Page;
  readonly welcomeMessage: Locator;
  readonly squadName: Locator;

  constructor(page: Page) {
    this.page = page;
    this.welcomeMessage = page.getByText("Welcome to Ari");
    this.squadName = page.locator("h2.text-xl.font-semibold");
  }

  async getStatValue(name: string): Promise<string | null> {
    // Find the card by its title text, then get the value
    const card = this.page
      .locator(".rounded-xl")
      .filter({ hasText: name });
    const value = card.locator("p.text-2xl");
    if (await value.isVisible()) {
      return value.textContent();
    }
    return null;
  }

  async getSquadName(): Promise<string | null> {
    return this.squadName.textContent();
  }

  async isWelcomeVisible(): Promise<boolean> {
    return this.welcomeMessage.isVisible();
  }
}
