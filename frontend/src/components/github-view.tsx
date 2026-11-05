"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Github } from "lucide-react";
import { api, post } from "@/lib/api";
interface Connection {
  configured: boolean;
  slug: string;
  active: { id: string; account: string } | null;
  pending: { id: string; account: string } | null;
}
export function GitHubView() {
  const cache = useQueryClient();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const connection = useQuery({
    queryKey: ["github-connection"],
    queryFn: () => api<Connection>("/github/connection"),
  });
  const repos = useQuery({
    queryKey: ["repositories"],
    queryFn: () =>
      api<{
        repositories: {
          id: number;
          full_name: string;
          html_url: string;
          private: boolean;
          default_branch: string;
        }[];
      }>("/github/repositories"),
  });
  return (
    <section className="panel">
      <Github size={30} />
      <h2>GitHub App</h2>
      <p className="muted">
        Один раз задайте App ID, App slug и PEM-ключ на сервере. Затем
        подключите установку через GitHub и подтвердите аккаунт в панели.
        Permissions: Contents read, Metadata read. Событие: Push.
      </p>
      <pre className="command">https://ВАШ-ДОМЕН/api/v1/github/webhook</pre>
      <p className="small muted">
        Setup URL GitHub App: https://ВАШ-ДОМЕН/api/v1/github/callback
      </p>
      {connection.data?.active && (
        <div className="info-box">
          Подключён аккаунт <b>{connection.data.active.account}</b> ·
          installation {connection.data.active.id}
        </div>
      )}
      <div className="action-row">
        <button
          className="primary"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              const result = await post<{ url: string }>("/github/connect");
              window.location.assign(result.url);
            } catch (err) {
              setError((err as Error).message);
              setBusy(false);
            }
          }}
        >
          <Github size={16} />
          Connect GitHub
        </button>
      </div>
      {connection.data?.pending && (
        <div className="info-box">
          <div>
            <b>Подтвердите аккаунт GitHub: {connection.data.pending.account}</b>
            <p>
              Убедитесь, что это ваш аккаунт/организация. Redirect сам по себе
              не даёт доступ к репозиториям панели.
            </p>
            <button
              className="primary"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  await post("/github/confirm", {
                    id: connection.data?.pending?.id,
                  });
                  await cache.invalidateQueries({
                    queryKey: ["github-connection"],
                  });
                  await cache.invalidateQueries({ queryKey: ["repositories"] });
                } catch (err) {
                  setError((err as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              Подтвердить {connection.data.pending.account}
            </button>
          </div>
        </div>
      )}
      {error && (
        <div className="error" role="alert">
          {error}
        </div>
      )}
      {repos.isPending && <div className="skeleton" />}
      {repos.isError && <div className="error">{repos.error.message}</div>}
      {repos.data?.repositories.map((repo) => (
        <div className="monitor-row" key={repo.id}>
          <a href={repo.html_url} target="_blank" rel="noreferrer">
            {repo.full_name}
          </a>
          <span className="badge neutral">
            {repo.private ? "private" : "public"}
          </span>
          <code>{repo.default_branch}</code>
        </div>
      ))}
      <p className="small muted">
        Репозитории и ветки можно выбрать в форме проекта. Показаны первые 100
        репозиториев установки. Ручной Installation ID в .env также
        поддерживается.
      </p>
    </section>
  );
}
