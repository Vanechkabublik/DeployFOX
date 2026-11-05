"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  Server,
  ShieldCheck,
} from "lucide-react";
import { api, post } from "@/lib/api";
import { NewProject } from "./new-project";
import { GitHubView } from "./github-view";

interface Checks {
  completed: boolean;
  projects: number;
  github_connected: boolean;
  public_url: string;
  provider: string;
}
interface Host {
  available: boolean;
  containers: number;
  memory_total: number;
  disk_available: boolean;
  disk_total: number;
  disk_used: number;
}
const steps = [
  "Аккаунт администратора",
  "Домен панели",
  "Docker Engine",
  "Доступное место",
  "Caddy и HTTPS",
  "Подключение GitHub",
  "Первый проект",
];

export function SetupWizard({
  email,
  done,
}: {
  email: string;
  done: () => void;
}) {
  const [step, setStep] = useState(() =>
    typeof window !== "undefined" &&
    new URLSearchParams(window.location.search).get("github") === "pending"
      ? 5
      : 0,
  );
  const [create, setCreate] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const checks = useQuery({
    queryKey: ["setup-checks"],
    queryFn: () => api<Checks>("/setup/checks"),
    refetchInterval: 5000,
  });
  const host = useQuery({
    queryKey: ["setup-host"],
    queryFn: () => api<Host>("/monitoring/host"),
    enabled: step >= 2 && checks.data?.provider === "docker",
  });
  const mock = checks.data?.provider === "mock";
  const caddy = useQuery({
    queryKey: ["setup-caddy"],
    queryFn: () =>
      api<{ available: boolean; valid: boolean }>("/monitoring/caddy"),
    enabled: step >= 4 && checks.data?.provider === "docker",
  });
  const free = host.data?.disk_available
    ? (host.data.disk_total - host.data.disk_used) / 1024 ** 3
    : null;
  const ready =
    step === 2
      ? mock || !!host.data?.available
      : step === 3
        ? mock || (free !== null && free >= 15)
        : step === 4
          ? mock || !!caddy.data?.valid
          : step === 6
            ? (checks.data?.projects || 0) > 0
            : true;
  return (
    <main className="setup-shell">
      <div className="brand">
        <span className="fox">◈</span>deployfox
      </div>
      <div className="setup-heading">
        <span className="eyebrow">LET’S GET YOUR SERVER READY</span>
        <h1>Первоначальная настройка</h1>
        <p className="muted">
          Один владелец, один сервер. Пройдём путь до первого проекта.
        </p>
      </div>
      <div className="setup-layout">
        <nav aria-label="Этапы установки">
          {steps.map((label, index) => (
            <button
              key={label}
              className={index === step ? "setup-step current" : "setup-step"}
              onClick={() => {
                if (index < step) setStep(index);
              }}
            >
              <span>
                {index < step ? (
                  <Check size={14} />
                ) : (
                  String(index + 1).padStart(2, "0")
                )}
              </span>
              {label}
            </button>
          ))}
        </nav>
        <section className="panel setup-content">
          <span className="eyebrow">ШАГ {step + 1} ИЗ 7</span>
          <h2>{steps[step]}</h2>
          {step === 0 && (
            <>
              <ShieldCheck size={40} className="active-text" />
              <h3>Аккаунт создан: {email}</h3>
              <p className="muted">
                Повторная первоначальная регистрация закрыта. После установки
                включите TOTP 2FA и сохраните резервные коды в менеджере
                паролей.
              </p>
            </>
          )}
          {step === 1 && (
            <>
              <h3>{checks.data?.public_url || "Загружаем домен…"}</h3>
              <p className="muted">
                Адрес задаётся установщиком в PUBLIC_URL / PANEL_DOMAIN. В
                production направьте DNS A-запись на IP VPS и используйте HTTPS.
                Если адрес неверный, исправьте .env на сервере и перезапустите
                панель по инструкции docs/06-connect-domain.md.
              </p>
              {mock && (
                <div className="info-box">
                  Локальный адрес localhost работает по HTTP. Это не проверка
                  production HTTPS.
                </div>
              )}
            </>
          )}
          {step === 2 && (
            <>
              <Server size={38} className="active-text" />
              <p>
                {mock
                  ? "Mock provider: доступ к Docker Engine намеренно не предоставлен панели."
                  : host.data?.available
                    ? `Docker agent подключён. Контейнеров на Engine: ${host.data.containers}.`
                    : "Ожидаем ответ Docker-agent…"}
              </p>
              <p className="muted">
                Socket доступен только отдельному agent. Не передавайте его
                приложениям и не запускайте недоверенный код.
              </p>
              {host.isError && (
                <div className="error">{host.error.message}</div>
              )}
            </>
          )}
          {step === 3 && (
            <>
              <h3>
                {mock
                  ? "Проверка VPS пропущена в mock-режиме"
                  : free !== null
                    ? `${free.toFixed(1)} GB свободно`
                    : "Диск Docker недоступен для проверки"}
              </h3>
              <p className="muted">
                Для начала требуется минимум 15 GB свободного диска. Образы,
                build cache, базы и копии используют один VPS. Копии нужно
                выносить за его пределы.
              </p>
              {!mock && (
                <button className="secondary" onClick={() => host.refetch()}>
                  Повторить проверку
                </button>
              )}
            </>
          )}
          {step === 4 && (
            <>
              <CheckCircle2 size={38} className="active-text" />
              <p className="muted">
                {mock
                  ? "В безопасном local mock Caddy не запускается; этот шаг не подтверждает HTTPS."
                  : caddy.data?.valid
                    ? "Agent проверил Caddy validate: конфигурация корректна. Сертификат панели проверьте по замку в браузере; внешняя доступность не подтверждается этой проверкой."
                    : "Проверяем конфигурацию Caddy через agent…"}
              </p>
              {caddy.isError && (
                <div className="error">
                  {caddy.error.message}
                  <button
                    className="text-button"
                    onClick={() => caddy.refetch()}
                  >
                    Повторить проверку
                  </button>
                </div>
              )}
              <p className="muted">
                После первого деплоя кнопка «Проверить DNS / HTTPS» в Домены
                покажет DNS, TCP и сертификат приложения. Нужен SERVER_IP в
                .env.
              </p>
            </>
          )}
          {step === 5 && (
            <>
              <GitHubView />
              <p className="small muted">
                Если используете готовый Docker image или публичный Git URL,
                подключение GitHub App можно пропустить. Для private
                repositories подключите App.
              </p>
            </>
          )}
          {step === 6 && (
            <>
              <h3>
                {checks.data?.projects
                  ? `Проектов создано: ${checks.data.projects}`
                  : "Создайте первый проект"}
              </h3>
              <p className="muted">
                Для знакомства выберите Docker image nginx:alpine, порт 80 и
                healthcheck /. В mock-режиме Deploy проверяет только жизненный
                цикл. Настоящее приложение запускается через Docker provider.
              </p>
              <button className="primary" onClick={() => setCreate(true)}>
                Создать первый проект
              </button>
            </>
          )}
          {(error || checks.isError) && (
            <div className="error" role="alert">
              {error || checks.error?.message}
            </div>
          )}
          <div className="modal-footer">
            <button
              className="text-button"
              disabled={step === 0}
              onClick={() => setStep(Math.max(0, step - 1))}
            >
              <ArrowLeft size={15} />
              Назад
            </button>
            <button
              className="primary"
              disabled={busy || !ready || checks.isPending}
              onClick={async () => {
                if (step < 6) {
                  setStep(step + 1);
                  return;
                }
                setBusy(true);
                setError("");
                try {
                  await post("/setup/complete");
                  done();
                } catch (err) {
                  setError((err as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {step === 6
                ? "Открыть Dashboard"
                : step === 5 && !checks.data?.github_connected
                  ? "Пропустить GitHub"
                  : "Далее"}
              <ArrowRight size={15} />
            </button>
          </div>
        </section>
      </div>
      {create && (
        <NewProject
          close={() => setCreate(false)}
          saved={() => {
            setCreate(false);
            checks.refetch();
          }}
        />
      )}
    </main>
  );
}
