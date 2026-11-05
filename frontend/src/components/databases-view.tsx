"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Database, Plus, Copy } from "lucide-react";
import { api, post } from "@/lib/api";
import { Badge } from "./badge";
interface DatabaseRecord {
  id: string;
  name: string;
  engine: string;
  version: string;
  status: string;
}
export function DatabasesView({
  notify,
}: {
  notify: (message: string) => void;
}) {
  const cache = useQueryClient();
  const [engine, setEngine] = useState("postgres");
  const [uri, setURI] = useState("");
  const [busy, setBusy] = useState(false);
  const databases = useQuery({
    queryKey: ["databases"],
    queryFn: () => api<DatabaseRecord[]>("/databases"),
    refetchInterval: 5000,
  });
  return (
    <>
      <section className="panel">
        <h2>Управляемые базы данных</h2>
        <p className="muted">
          Отдельный контейнер и persistent volume для каждой базы. Порты не
          публикуются. Приложения подключаются через внутреннюю сеть
          deployfox-apps.
        </p>
        <form
          className="database-form"
          onSubmit={async (event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            setBusy(true);
            try {
              await post("/databases", {
                name: data.get("name"),
                engine,
                version:
                  engine === "postgres"
                    ? "16"
                    : engine === "redis"
                      ? "7"
                      : "8.4",
              });
              await cache.invalidateQueries({ queryKey: ["databases"] });
              notify(
                "Команда создания базы выполнена. Проверьте подключение перед использованием.",
              );
            } catch (err) {
              notify((err as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <div className="form-grid">
            <label>
              Имя
              <input
                name="name"
                required
                minLength={2}
                maxLength={64}
                placeholder="production-db"
              />
            </label>
            <label>
              Тип
              <select
                value={engine}
                onChange={(e) => setEngine(e.target.value)}
              >
                <option value="postgres">PostgreSQL 16</option>
                <option value="redis">Redis 7</option>
                <option value="mysql">MySQL 8.4</option>
              </select>
            </label>
          </div>
          <button className="primary" disabled={busy}>
            <Plus size={15} />
            {busy ? "Создаём…" : "Создать базу"}
          </button>
        </form>
      </section>
      <div className="project-grid">
        {databases.data?.map((db) => (
          <section className="panel" key={db.id}>
            <div className="detail-heading">
              <Database size={21} />
              <h2>{db.name}</h2>
            </div>
            <p className="muted">
              {db.engine} {db.version}
            </p>
            <Badge status={db.status} />
            <div className="action-row">
              {[
                "start",
                "stop",
                "restart",
                "backup",
                "connection",
                ...(db.engine === "redis" ? [] : ["restore"]),
              ].map((action) => (
                <button
                  className="secondary"
                  key={action}
                  disabled={busy}
                  onClick={async () => {
                    let body:
                      { backup: string; confirmation: string } | undefined;
                    if (action === "restore") {
                      const backup = window.prompt(
                        "Имя backup-файла из backups/managed (UUID-дата.dump или .sql):",
                      );
                      if (!backup) return;
                      const confirmation = window.prompt(
                        `Восстановление заменит данные. Остановите использующие базу приложения. Введите ${db.name} для подтверждения:`,
                      );
                      if (confirmation !== db.name) return;
                      body = { backup, confirmation };
                    }
                    setBusy(true);
                    setURI("");
                    try {
                      const result = await post<{
                        uri?: string;
                        message?: string;
                      }>(`/databases/${db.id}/${action}`, body);
                      if (result.uri) setURI(result.uri);
                      else notify(result.message || "Команда выполнена");
                      cache.invalidateQueries({ queryKey: ["databases"] });
                    } catch (err) {
                      notify((err as Error).message);
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  {action}
                </button>
              ))}
            </div>
          </section>
        ))}
      </div>
      {databases.isError && (
        <div className="error">{databases.error.message}</div>
      )}
      {uri && (
        <section className="panel">
          <h2>Строка подключения · содержит пароль</h2>
          <p className="muted">
            Добавьте её как DATABASE_URL в переменные проекта, затем выполните
            Deploy.
          </p>
          <pre className="command">{uri}</pre>
          <button
            className="secondary"
            onClick={() => {
              navigator.clipboard.writeText(uri);
              notify("Скопировано");
            }}
          >
            <Copy size={15} />
            Копировать
          </button>
          <button className="text-button" onClick={() => setURI("")}>
            Скрыть
          </button>
        </section>
      )}
    </>
  );
}
