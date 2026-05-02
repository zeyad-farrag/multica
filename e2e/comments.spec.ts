import { test, expect } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Comments", () => {
  let api: TestApiClient;
  let issueId: string;
  let issueTitle: string;
  let workspaceSlug: string;

  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    issueTitle = "E2E Comment Test " + Date.now();
    const issue = await api.createIssue(issueTitle);
    issueId = issue.id;
    workspaceSlug = await loginAsDefault(page);
  });

  test.afterEach(async () => {
    await api.cleanup();
  });

  test("can add a comment on an issue", async ({ page }) => {
    await page.getByRole("link", { name: new RegExp(issueTitle) }).click();
    await page.waitForURL(new RegExp(`/${workspaceSlug}/issues/${issueId}$`));

    // Wait for issue detail to load
    await expect(page.locator("text=Properties")).toBeVisible();

    // Type a comment
    const commentText = "E2E comment " + Date.now();
    const commentInput = page.locator('[contenteditable="true"]').last();
    const commentBox = commentInput.locator(
      'xpath=ancestor::div[contains(@class, "rounded-lg")][1]',
    );
    await commentInput.fill(commentText);

    // Submit the comment
    await commentBox.getByRole("button").last().click();

    // Comment should appear in the activity section
    await expect(page.locator(`text=${commentText}`)).toBeVisible({
      timeout: 5000,
    });
  });

  test("comment submit button is disabled when empty", async ({ page }) => {
    await page.getByRole("link", { name: new RegExp(issueTitle) }).click();
    await page.waitForURL(new RegExp(`/${workspaceSlug}/issues/${issueId}$`));

    await expect(page.locator("text=Properties")).toBeVisible();

    // Submit button should be disabled when input is empty
    const commentInput = page.locator('[contenteditable="true"]').last();
    const commentBox = commentInput.locator(
      'xpath=ancestor::div[contains(@class, "rounded-lg")][1]',
    );
    const submitBtn = commentBox.getByRole("button").last();
    await expect(submitBtn).toBeDisabled();
  });
});
