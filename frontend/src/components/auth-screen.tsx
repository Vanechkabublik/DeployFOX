"use client";
import { useState } from "react";
import {
  ArrowRight,
  Check,
  KeyRound,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { post } from "@/lib/api";
export function AuthScreen({
  initialized,
  onLogin,
}: {
  initialized: boolean;
  onLogin: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <main className="auth-shell">
      <div className="auth-story">
        <div className="brand">
          <span className="fox">◈</span> deployfox
          <span className="version">SELF-HOSTED</span>
        </div>
        <div>
          <span className="eyebrow">YOUR INFRASTRUCTURE. YOUR RULES.</span>
          <h1>
            From a commit
            <br />
            to <em>live.</em>
          </h1>
          <p>
            Ваш код, ваш сервер, одна панель.
            <br />
            Разворачивайте проекты и контролируйте каждый релиз.
          </p>
          <div className="terminal-preview">
            <div>
              <i />
              <i />
              <i />
              <span>deployfox / getting started</span>
            </div>
            <code>
              <b>$</b> docker compose up -d
              <br />
              <span>✓ Control plane is ready</span>
              <br />
              <span>✓ Deployment worker connected</span>
              <br />
              <b>→</b> Waiting for your first project
              <span className="cursor"> ▋</span>
            </code>
          </div>
        </div>
        <small>
          <ShieldCheck size={15} /> Работает на вашем VPS · Только для владельца
        </small>
      </div>
      <section className="auth-form">
        <div className="auth-card">
          <span className="icon-tile">
            <KeyRound size={24} />
          </span>
          <h2>
            {initialized ? "С возвращением" : "Начнём с безопасного входа"}
          </h2>
          <p>
            {initialized
              ? "Войдите в панель вашего сервера."
              : "Создайте единственный аккаунт администратора."}
          </p>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              if (!initialized) return;
              setBusy(true);
              setError("");
              const data = new FormData(event.currentTarget);
              try {
                await post("/auth/login", {
                  email: data.get("email"),
                  password: data.get("password"),
                  code: data.get("code") || "",
                });
                onLogin();
              } catch (err) {
                setError((err as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            {!initialized && <SetupForm onLogin={onLogin} />}
            {initialized && (
              <>
                <label>
                  Email
                  <input
                    name="email"
                    type="email"
                    autoComplete="username"
                    required
                    placeholder="you@example.com"
                  />
                </label>
                <label>
                  Пароль
                  <input
                    name="password"
                    type="password"
                    autoComplete="current-password"
                    required
                  />
                </label>
                <label>
                  Код 2FA или резервный код · если включено
                  <input
                    name="code"
                    type="text"
                    autoComplete="one-time-code"
                    placeholder="Необязательно"
                    maxLength={20}
                  />
                </label>
                {error && (
                  <div className="error" role="alert">
                    {error}
                  </div>
                )}
                <button className="primary full" disabled={busy}>
                  {busy ? "Входим…" : "Открыть панель"}
                  <ArrowRight size={17} />
                </button>
              </>
            )}
          </form>
          <div className="secure-note">
            <Check size={14} /> Зашифрованные секреты · Защищённые сессии
          </div>
        </div>
        <small>
          <Terminal size={14} /> DeployFox / Single server. Endless
          possibilities.
        </small>
      </section>
    </main>
  );
}

function SetupForm({ onLogin }: { onLogin: () => void }) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <>
      <label>
        Одноразовый токен установки
        <input
          name="setup_token"
          type="password"
          required
          autoComplete="off"
          placeholder="Из .env на вашем сервере"
        />
      </label>
      <label>
        Email администратора
        <input name="email" type="email" required autoComplete="username" />
      </label>
      <label>
        Пароль · минимум 12 символов
        <input
          name="password"
          type="password"
          required
          minLength={12}
          maxLength={256}
          autoComplete="new-password"
        />
      </label>
      <p className="muted small">
        Токен SETUP_TOKEN находится в файле .env. После создания аккаунта
        повторная инициализация блокируется базой данных.
      </p>
      {error && (
        <div className="error" role="alert">
          {error}
        </div>
      )}
      <button
        className="primary full"
        disabled={busy}
        onClick={async (event) => {
          event.preventDefault();
          const form = event.currentTarget.form;
          if (!form?.reportValidity()) return;
          const data = new FormData(form);
          setBusy(true);
          setError("");
          try {
            const response = await fetch("/api/v1/setup", {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                "X-Setup-Token": String(data.get("setup_token")),
              },
              body: JSON.stringify({
                email: data.get("email"),
                password: data.get("password"),
              }),
            });
            const result = await response.json();
            if (!response.ok) throw new Error(result.error);
            onLogin();
          } catch (err) {
            setError((err as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "Создаём…" : "Создать администратора"}
        <ArrowRight size={17} />
      </button>
    </>
  );
}
