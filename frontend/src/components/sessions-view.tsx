"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
interface SessionRecord {
  id: string;
  created_at: string;
  expires_at: string;
  current: boolean;
}
export function SessionsView({
  notify,
}: {
  notify: (message: string) => void;
}) {
  const cache = useQueryClient();
  const query = useQuery({
    queryKey: ["sessions"],
    queryFn: () => api<SessionRecord[]>("/auth/sessions"),
  });
  return (
    <section className="panel">
      <h2>Активные сессии</h2>
      <p className="muted">
        Вход действует 24 часа. Вы можете завершить неизвестные сессии.
      </p>
      {query.data?.map((session) => (
        <div className="monitor-row" key={session.id}>
          <span>{session.current ? "Текущая сессия" : "Другая сессия"}</span>
          <span className="small muted">
            Создана {new Date(session.created_at).toLocaleString("ru-RU")}
          </span>
          <button
            className="secondary"
            onClick={async () => {
              try {
                await api(`/auth/sessions/${session.id}`, { method: "DELETE" });
                if (session.current) window.location.reload();
                else {
                  await cache.invalidateQueries({ queryKey: ["sessions"] });
                  notify("Сессия завершена");
                }
              } catch (err) {
                notify((err as Error).message);
              }
            }}
          >
            Завершить
          </button>
        </div>
      ))}
      {query.isError && <div className="error">{query.error.message}</div>}
    </section>
  );
}
