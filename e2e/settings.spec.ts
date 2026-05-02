import { test, expect } from "@playwright/test";
import { loginAsDefault } from "./helpers";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

test.describe("Settings", () => {
  test("updating workspace name reflects in sidebar immediately", async ({
    page,
  }) => {
    await loginAsDefault(page);

    // Read the current workspace name from the sidebar
    const sidebarName = page.locator('button[data-sidebar="menu-button"]').first();
    const originalName = (await sidebarName.innerText())
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .at(-1);
    if (!originalName) {
      throw new Error("Could not resolve workspace name from sidebar menu button");
    }

    // Navigate to settings
    await page.getByRole("link", { name: "Settings" }).click();
    await page.waitForURL("**/settings");
    await page.getByRole("tab", { name: "General" }).click();

    // Change workspace name
    const nameInput = page
      .getByRole("tabpanel", { name: "General" })
      .getByRole("textbox")
      .first();
    await nameInput.clear();
    const newName = "Renamed WS " + Date.now();
    await nameInput.fill(newName);

    // Save
    await page.locator("button", { hasText: "Save" }).click();

    // Sidebar should reflect the new name WITHOUT page refresh
    await expect(page.getByRole("button", { name: new RegExp(escapeRegExp(newName)) })).toBeVisible();

    // Restore original name so other tests aren't affected
    await nameInput.clear();
    await nameInput.fill(originalName);
    await page.locator("button", { hasText: "Save" }).click();
    await expect(page.getByRole("button", { name: new RegExp(escapeRegExp(originalName)) })).toBeVisible();
  });
});
