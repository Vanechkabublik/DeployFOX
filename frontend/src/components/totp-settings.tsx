"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, post } from "@/lib/api";
import type { Session } from "@/types";

export function TOTPSettings({
  notify,
}: {
  notify: (message: string) => void;
}) {
  const cache = useQueryClient();
  const session = useQuery({
    queryKey: ["session"],
    queryFn: () => api<Session>("/auth/me"),
  });
  const [secret, setSecret] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  async function run(action: string) {
    setBusy(true);
    try {
      if (action === "start") {
        const result = await post<{ secret: string }>("/auth/totp/start", {
          password,
        });
        setSecret(result.secret);
        setPassword("");
      }
      if (action === "enable") {
        const result = await post<{ recovery_codes: string[] }>(
          "/auth/totp/enable",
          { code },
        );
        setRecovery(result.recovery_codes);
        setSecret("");
        setCode("");
        await cache.invalidateQueries({ queryKey: ["session"] });
        notify("2FA включена. Сохраните резервные коды вне VPS.");
      }
      if (action === "disable") {
        await post("/auth/totp/disable", { password, code });
        setPassword("");
        setCode("");
        setRecovery([]);
        await cache.invalidateQueries({ queryKey: ["session"] });
        notify("2FA отключена");
      }
    } catch (err) {
      notify((err as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel">
      <h2>Двухфакторная аутентификация</h2>
      <p className="muted">
        {session.data?.totp_enabled
          ? "TOTP включена. При входе нужен пароль и код из приложения-аутентификатора."
          : "Добавьте второй фактор в приложении-аутентификаторе с поддержкой TOTP."}
      </p>
      <div className="settings-form">
        {!secret && (
          <label>
            Текущий пароль
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
        )}
        {secret && (
          <>
            <p>
              Добавьте ключ вручную: аккаунт DeployFox, тип «по времени», 6
              цифр, период 30 секунд.
            </p>
            <pre className="command">{secret}</pre>
            <p className="small muted">
              Ключ показывается только при настройке. Никому его не передавайте.
            </p>
          </>
        )}
        {(secret || session.data?.totp_enabled) && (
          <label>
            Код TOTP или резервный код
            <input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              autoComplete="one-time-code"
              maxLength={20}
            />
          </label>
        )}
        <button
          className="primary"
          disabled={busy}
          onClick={() =>
            run(
              session.data?.totp_enabled
                ? "disable"
                : secret
                  ? "enable"
                  : "start",
            )
          }
        >
          {session.data?.totp_enabled
            ? "Отключить 2FA"
            : secret
              ? "Подтвердить и включить"
              : "Начать настройку 2FA"}
        </button>
      </div>
      {recovery.length > 0 && (
        <div className="info-box">
          <div>
            <b>
              Резервные одноразовые коды. Сохраните сейчас — повторно они не
              показываются.
            </b>
            <pre>{recovery.join("\n")}</pre>
            <button className="secondary" onClick={() => setRecovery([])}>
              Я сохранил коды, скрыть
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
