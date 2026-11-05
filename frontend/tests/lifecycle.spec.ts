import { test, expect } from "@playwright/test";
test("owner login, project creation, queue and live logs", async ({ page }) => {
  const password = process.env.E2E_PASSWORD;
  const email = process.env.E2E_EMAIL;
  test.skip(
    !password || !email,
    "Set E2E_EMAIL and E2E_PASSWORD for a disposable initialized local stack",
  );
  await page.goto("/");
  await page.getByLabel("Email", { exact: true }).fill(email!);
  await page.getByLabel("Пароль", { exact: true }).fill(password!);
  await page.getByRole("button", { name: "Открыть панель" }).click();
  await expect(
    page.getByRole("heading", {
      name: /Первоначальная настройка|Всё под контролем\./,
    }),
  ).toBeVisible();
  if (
    await page
      .getByRole("heading", { name: "Первоначальная настройка" })
      .isVisible()
  ) {
    for (let step = 0; step < 5; step++)
      await page.getByRole("button", { name: "Далее", exact: true }).click();
    await page
      .getByRole("button", { name: "Пропустить GitHub", exact: true })
      .click();
    const finish = page.getByRole("button", {
      name: "Открыть Dashboard",
      exact: true,
    });
    if (await finish.isDisabled()) {
      await page
        .getByRole("button", { name: "Создать первый проект", exact: true })
        .click();
      await page
        .getByLabel("Название", { exact: true })
        .fill(`wizard-${Date.now()}`);
      await page
        .getByLabel("Docker image", { exact: true })
        .fill("nginx:alpine");
      await page.getByLabel("Порт контейнера").fill("80");
      await page
        .getByRole("button", { name: "Создать проект", exact: true })
        .click();
    }
    await finish.click();
  }
  await expect(page.getByText("Всё под контролем.")).toBeVisible();
  await page.getByRole("button", { name: "Новый проект", exact: true }).click();
  const name = `e2e-${Date.now()}-long-project-name-without-extra-spaces`;
  await page.getByLabel("Название", { exact: true }).fill(name);
  await page.getByLabel("Docker image", { exact: true }).fill("nginx:alpine");
  await page.getByLabel("Порт контейнера").fill("80");
  await page
    .getByRole("button", { name: "Создать проект", exact: true })
    .click();
  const card = page
    .locator("article")
    .filter({ has: page.getByRole("button", { name, exact: true }) });
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "Deploy", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Deployment logs" }),
  ).toBeVisible();
  await expect(
    page.getByText("Deployment successful", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Закрыть", exact: true })
    .click();
  await card.getByRole("button", { name, exact: true }).click();
  await page
    .getByRole("button", { name: "Настройки проекта", exact: true })
    .click();
  await page.getByLabel("Память · MB").fill("256");
  await page
    .getByRole("button", { name: "Сохранить настройки", exact: true })
    .click();
  await expect(page.getByText("256 MB / 1 CPU", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Назад к проектам" }).click();
  await page.screenshot({
    path: "test-results/deployfox-desktop.png",
    fullPage: true,
  });
});
