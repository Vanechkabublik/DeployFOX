"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, post } from "@/lib/api";
import { TOTPSettings } from "./totp-settings";
import { SessionsView } from "./sessions-view";
export function SettingsView({
  email,
  notify,
}: {
  email: string;
  notify: (message: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const audit = useQuery({
    queryKey: ["audit"],
    queryFn: () =>
      api<{ action: string; target: string; created_at: string }[]>("/audit"),
  });
  return (
    <>
      <TOTPSettings notify={notify} />
      <SessionsView notify={notify} />
      <section className="panel">
        <h2>Аккаунт администратора</h2>
        <p className="muted">{email} · Единственный владелец сервера</p>
        <form
          className="settings-form"
          onSubmit={async (e) => {
            e.preventDefault();
            const data = new FormData(e.currentTarget);
            setBusy(true);
            try {
              await post("/auth/password", {
                current: data.get("current"),
                password: data.get("password"),
              });
              window.location.reload();
            } catch (err) {
              notify((err as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            Текущий пароль
            <input
              name="current"
              type="password"
              required
              autoComplete="current-password"
            />
          </label>
          <label>
            Новый пароль
            <input
              name="password"
              type="password"
              minLength={12}
              maxLength={256}
              required
              autoComplete="new-password"
            />
          </label>
          <p className="small muted">
            Смена пароля завершит все активные сессии.
          </p>
          <button className="primary" disabled={busy}>
            Сменить пароль
          </button>
        </form>
      </section>
      <section className="panel">
        <h2>Журнал аудита</h2>
        <p className="muted">
          Последние 100 критических действий. Секреты в журнал не записываются.
        </p>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>ДЕЙСТВИЕ</th>
                <th>ОБЪЕКТ</th>
                <th>ВРЕМЯ</th>
              </tr>
            </thead>
            <tbody>
              {audit.data?.map((event, i) => (
                <tr key={i}>
                  <td>
                    <code>{event.action}</code>
                  </td>
                  <td className="muted">{event.target.slice(0, 12) || "—"}</td>
                  <td className="muted">
                    {new Date(event.created_at).toLocaleString("ru-RU")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {audit.isError && <div className="error">{audit.error.message}</div>}
      </section>
    </>
  );
}
