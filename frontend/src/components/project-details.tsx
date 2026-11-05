"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  ArrowLeft,
  Play,
  RefreshCw,
  Rocket,
  RotateCcw,
  Square,
  Trash2,
} from "lucide-react";
import { api } from "@/lib/api";
import type { Project } from "@/types";
import { Badge } from "./badge";
import { LogViewer } from "./log-viewer";
import { ProjectMetrics } from "./project-metrics";
import { NewProject } from "./new-project";
export function ProjectDetails({
  project,
  back,
  action,
  notify,
}: {
  project: Project;
  back: () => void;
  action: (p: Project, action: string) => Promise<void>;
  notify: (message: string) => void;
}) {
  const cache = useQueryClient();
  const [input, setInput] = useState("");
  const [remove, setRemove] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [showLogs, setShowLogs] = useState(false);
  const [editing, setEditing] = useState(false);
  const env = useQuery({
    queryKey: ["environment", project.id],
    queryFn: () => api<string[]>(`/projects/${project.id}/environment`),
  });
  return (
    <>
      {editing && (
        <NewProject
          initial={project}
          close={() => setEditing(false)}
          saved={() => {
            setEditing(false);
            cache.invalidateQueries({ queryKey: ["projects"] });
            notify(
              "Настройки сохранены. Выполните Deploy для применения. До успешного деплоя старый домен остаётся зарезервирован.",
            );
          }}
        />
      )}
      {showLogs && (
        <LogViewer
          id={project.id}
          application
          close={() => setShowLogs(false)}
        />
      )}
      <button className="text-button" onClick={back}>
        <ArrowLeft size={16} />
        Назад к проектам
      </button>
      <section className="panel project-detail">
        <div className="detail-heading">
          <h2>{project.name}</h2>
          <Badge status={project.status} />
        </div>
        <p className="muted">{project.source}</p>
        <button className="secondary" onClick={() => setEditing(true)}>
          Настройки проекта
        </button>
        <ProjectMetrics id={project.id} active={!!project.active_container} />
        <button
          className="secondary"
          disabled={!project.active_container}
          onClick={() => setShowLogs(true)}
        >
          Application logs
        </button>
        <div className="action-row">
          {[
            { key: "deploy", label: "Deploy", icon: Rocket },
            { key: "restart", label: "Restart", icon: RefreshCw },
            { key: "stop", label: "Stop", icon: Square },
            { key: "start", label: "Start", icon: Play },
            { key: "rollback", label: "Rollback", icon: RotateCcw },
          ].map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              disabled={busy}
              className={key === "deploy" ? "primary" : "secondary"}
              onClick={async () => {
                setBusy(true);
                try {
                  await action(project, key);
                } finally {
                  setBusy(false);
                }
              }}
            >
              <Icon size={15} />
              {label}
            </button>
          ))}
        </div>
        <div className="detail-grid">
          <div>
            <span>Домен</span>
            <b>{project.domain || "Не задан"}</b>
          </div>
          <div>
            <span>Healthcheck</span>
            <b>
              {project.service_type === "worker"
                ? "Процесс работает 5 секунд без рестарта"
                : `HTTP :${project.port}${project.health_path}`}
            </b>
          </div>
          <div>
            <span>Ресурсы</span>
            <b>
              {project.memory_mb} MB / {project.cpu} CPU
            </b>
          </div>
          <div>
            <span>Auto deploy</span>
            <b>
              {project.auto_deploy ? `Включён · ${project.branch}` : "Выключен"}
            </b>
          </div>
        </div>
        <p className="small muted break-all">
          Образ: {project.active_image || "Ещё не развёрнут"}
        </p>
      </section>
      <section className="panel">
        <h2>Environment variables</h2>
        <p className="muted">
          Значения зашифрованы. API возвращает только имена. После изменения
          выполните Deploy, чтобы применить новую конфигурацию.
        </p>
        <div className="env-list">
          {env.data
            ?.filter((key) => !remove.includes(key))
            .map((key) => (
              <div className="env-row" key={key}>
                <code>{key}</code>
                <span className="muted">••••••••••••</span>
                <button
                  className="icon-button"
                  aria-label={`Удалить ${key}`}
                  onClick={() => setRemove([...remove, key])}
                >
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
        </div>
        <label>
          Добавить или заменить · KEY=VALUE
          <textarea
            rows={5}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={"NODE_ENV=production\nAPI_URL=https://api.example.com"}
            spellCheck={false}
          />
        </label>
        <button
          className="primary"
          disabled={busy}
          onClick={async () => {
            const values: Record<string, string> = {};
            for (const line of input.split("\n")) {
              if (!line.trim() || line.trim().startsWith("#")) continue;
              const split = line.indexOf("=");
              if (split < 1) {
                notify("Каждая строка должна иметь формат KEY=VALUE");
                return;
              }
              values[line.slice(0, split).trim()] = line.slice(split + 1);
            }
            setBusy(true);
            try {
              await api(`/projects/${project.id}/environment`, {
                method: "PUT",
                body: JSON.stringify({ values, delete: remove }),
              });
              setInput("");
              setRemove([]);
              await cache.invalidateQueries({
                queryKey: ["environment", project.id],
              });
              notify("Сохранено. Выполните Deploy для применения.");
            } catch (err) {
              notify((err as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          Сохранить переменные
        </button>
      </section>
      <section className="panel danger-panel">
        <h2>Удаление проекта</h2>
        <p className="muted">
          Будут удалены контейнеры проекта, маршрут и история деплоев. Volumes и
          образы сохраняются. Введите точное имя для подтверждения.
        </p>
        <button
          className="danger-button"
          onClick={async () => {
            const name = window.prompt(
              `Введите ${project.name} для подтверждения удаления`,
            );
            if (name !== project.name) return;
            try {
              await api(`/projects/${project.id}`, {
                method: "DELETE",
                headers: { "X-Confirm-Delete": name },
              });
              cache.invalidateQueries({ queryKey: ["projects"] });
              back();
              notify("Проект удалён");
            } catch (err) {
              notify((err as Error).message);
            }
          }}
        >
          <Trash2 size={15} />
          Удалить проект
        </button>
      </section>
    </>
  );
}
