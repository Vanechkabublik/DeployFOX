import { test, expect } from "@playwright/test";

test("mobile navigation and layout", async ({ page }) => {
  test.skip(
    !process.env.E2E_EMAIL || !process.env.E2E_PASSWORD,
    "Initialized QA stack required",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.getByLabel("Email", { exact: true }).fill(process.env.E2E_EMAIL!);
  await page
    .getByLabel("Пароль", { exact: true })
    .fill(process.env.E2E_PASSWORD!);
  await page.getByRole("button", { name: "Открыть панель" }).click();
  await expect(page.getByText("Всё под контролем.")).toBeVisible();
  const sizes = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth,
    viewport: innerWidth,
  }));
  expect(sizes.page).toBeLessThanOrEqual(sizes.viewport);
  await page.screenshot({
    path: "test-results/deployfox-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Открыть меню" }).click();
  await page.getByRole("button", { name: "Настройки", exact: true }).click();
  await expect(
    page.getByText("Аккаунт администратора", { exact: true }),
  ).toBeVisible();
});
