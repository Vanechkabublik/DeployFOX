"use client";
import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ArrowRight,
  ArrowUpRight,
  Box,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  Clock3,
  Database,
  ExternalLink,
  FolderGit2,
  Github,
  Globe2,
  HardDrive,
  LayoutDashboard,
  LogOut,
  Menu,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Rocket,
  Search,
  Settings2,
  ShieldCheck,
  Terminal,
  TriangleAlert,
  X,
} from "lucide-react";
import Link from "next/link";
import { api, post, setCSRF } from "@/lib/api";
import { useUI } from "@/stores/ui";
import type { Deployment, Project, Session, System } from "@/types";
import { AuthScreen } from "./auth-screen";
import { NewProject } from "./new-project";
import { LogViewer } from "./log-viewer";
import { ProjectDetails } from "./project-details";
import { SettingsView } from "./settings-view";
import { DatabasesView } from "./databases-view";
import { HostMetrics } from "./host-metrics";
import { Badge } from "./badge";
import { DomainCheck } from "./domain-check";
import { GitHubView } from "./github-view";
import { SetupWizard } from "./setup-wizard";

const navigation = [
  { name: "Overview", label: "Обзор", icon: LayoutDashboard },
  { name: "Projects", label: "Проекты", icon: FolderGit2 },
  { name: "Deployments", label: "Деплои", icon: Rocket },
  { name: "Databases", label: "Базы данных", icon: Database },
  { name: "Domains", label: "Домены", icon: Globe2 },
  { name: "Monitoring", label: "Мониторинг", icon: Activity },
  { name: "GitHub", label: "GitHub", icon: Github },
  { name: "Backups", label: "Резервные копии", icon: HardDrive },
  { name: "Settings", label: "Настройки", icon: Settings2 },
];

function date(value: string) {
  return new Date(value).toLocaleString("ru-RU", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function Console() {
  const cache = useQueryClient();
  const { page, setPage, mobile, toggleMobile } = useUI();
  const [newProject, setNewProject] = useState(false);
  const [selected, setSelected] = useState<Project | null>(null);
  const [logs, setLogs] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const setup = useQuery({
    queryKey: ["setup"],
    queryFn: () =>
      api<{ initialized: boolean; provider: string }>("/setup/status"),
  });
  const session = useQuery({
    queryKey: ["session"],
    queryFn: async () => {
      const result = await api<Session>("/auth/me");
      setCSRF(result.csrf);
      return result;
    },
  });
  const authenticated = !!session.data;
  useEffect(() => {
    if (
      authenticated &&
      new URLSearchParams(window.location.search).get("github") === "pending"
    ) {
      setPage("GitHub");
      window.history.replaceState(null, "", window.location.pathname);
    }
  }, [authenticated, setPage]);
  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => api<Project[]>("/projects"),
    enabled: authenticated,
    refetchInterval: 5000,
  });
  const deployments = useQuery({
    queryKey: ["deployments"],
    queryFn: () => api<Deployment[]>("/deployments"),
    enabled: authenticated,
    refetchInterval: 3000,
  });
  const system = useQuery({
    queryKey: ["system"],
    queryFn: () => api<System>("/system"),
    enabled: authenticated,
    refetchInterval: 5000,
  });
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 7000);
    return () => clearTimeout(timer);
  }, [notice]);
  async function refresh() {
    await Promise.all([
      cache.invalidateQueries({ queryKey: ["projects"] }),
      cache.invalidateQueries({ queryKey: ["deployments"] }),
      cache.invalidateQueries({ queryKey: ["system"] }),
    ]);
  }
  async function action(p: Project, name: string) {
    try {
      const result = await post<{ id?: string }>(
        `/projects/${p.id}/${name === "deploy" || name === "rollback" ? name : `actions/${name}`}`,
      );
      setNotice(
        name === "deploy" || name === "rollback"
          ? "Деплой добавлен в очередь"
          : "Команда выполнена",
      );
      if (result.id) setLogs(result.id);
      await refresh();
    } catch (err) {
      setNotice((err as Error).message);
    }
  }
  if (setup.isPending || session.isPending)
    return (
      <div className="boot">
        <span className="fox">◈</span>
        <div className="skeleton" />
        <p>Подключаемся к DeployFox…</p>
      </div>
    );
  if (setup.isError)
    return (
      <div className="boot">
        <TriangleAlert />
        <h2>API недоступен</h2>
        <p>{setup.error.message}</p>
        <button onClick={() => setup.refetch()}>Повторить</button>
      </div>
    );
  if (!authenticated)
    return (
      <AuthScreen
        initialized={setup.data?.initialized ?? true}
        onLogin={() => {
          cache.invalidateQueries({ queryKey: ["session"] });
          cache.invalidateQueries({ queryKey: ["setup"] });
        }}
      />
    );
  if (session.data.needs_setup)
    return (
      <SetupWizard
        email={session.data.email}
        done={() => cache.invalidateQueries({ queryKey: ["session"] })}
      />
    );
  const items = (projects.data || []).filter((p) =>
    `${p.name} ${p.source}`.toLowerCase().includes(search.toLowerCase()),
  );
  const deploys = deployments.data || [];
  const mock = session.data.provider === "mock";
  return (
    <div className="app-shell">
      <aside className={`sidebar ${mobile ? "open" : ""}`}>
        <Link className="brand" href="/" aria-label="DeployFox home">
          <span className="fox">◈</span>deployfox
          <span className="brand-dot">®</span>
        </Link>
        <div className="workspace-switch">
          <span className="workspace-icon">D</span>
          <div>
            <b>My workspace</b>
            <small>Personal server</small>
          </div>
          <ChevronDown size={14} />
        </div>
        <span className="nav-label">WORKSPACE</span>
        <nav>
          {navigation.map(({ name, label, icon: Icon }) => (
            <button
              key={name}
              className={page === name ? "nav-item selected" : "nav-item"}
              onClick={() => {
                setPage(name);
                setSelected(null);
              }}
            >
              <Icon size={18} />
              {label}
              {name === "Projects" && <span>{projects.data?.length || 0}</span>}
              {page === name && <i />}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="server-box">
            <div>
              <span className="online-dot" />
              <b>DeployFox server</b>
              <span className="small muted">01</span>
            </div>
            <small>
              {mock ? "Mock provider · local" : "Docker provider · host agent"}
            </small>
            <div className="server-bars">
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
            </div>
            <span className="small muted">
              {system.data?.running || 0} приложений работают
            </span>
          </div>
          <a
            className="help-link"
            href="https://github.com/"
            target="_blank"
            rel="noreferrer"
          >
            <CircleHelp size={17} />
            GitHub
            <ArrowUpRight size={14} />
          </a>
          <button
            className="user-profile"
            onClick={async () => {
              try {
                await post("/auth/logout");
                cache.clear();
                window.location.reload();
              } catch (err) {
                setNotice((err as Error).message);
              }
            }}
          >
            <span className="avatar">
              {session.data.email[0]?.toUpperCase()}
            </span>
            <div>
              <b>Администратор</b>
              <small>{session.data.email}</small>
            </div>
            <LogOut size={15} />
          </button>
        </div>
      </aside>
      <div className="main-wrap">
        <header className="topbar">
          <button
            className="icon-button mobile-menu"
            aria-label="Открыть меню"
            onClick={toggleMobile}
          >
            <Menu />
          </button>
          <div className="breadcrumbs">
            Workspace
            <ChevronRight size={13} />
            <b>{navigation.find((n) => n.name === page)?.label}</b>
          </div>
          <div className="topbar-right">
            <span className="system-state">
              <i /> API connected
            </span>
            <span className="version">v0.1.0</span>
            <a
              href="https://docs.docker.com/"
              target="_blank"
              rel="noreferrer"
              className="icon-button"
              aria-label="Docker documentation"
            >
              <ExternalLink size={16} />
            </a>
          </div>
        </header>
        <main className="main-content">
          {mock && (
            <div className="mock-banner">
              <Terminal size={15} />
              <b>LOCAL / MOCK</b>
              <span>
                Жизненный цикл имитируется. Пользовательские контейнеры не
                запускаются.
              </span>
            </div>
          )}
          <div className="page-heading">
            <div>
              <div className="eyebrow">YOUR DEPLOYMENT CONTROL CENTER</div>
              <h1>
                {selected
                  ? selected.name
                  : page === "Overview"
                    ? "Всё под контролем."
                    : navigation.find((n) => n.name === page)?.label}
                <span className="heading-dot">↗</span>
              </h1>
              <p>
                {page === "Overview"
                  ? "Ваш сервер. Ваши проекты. Каждый релиз на виду."
                  : "Управляйте приложениями и инфраструктурой вашего сервера."}
              </p>
            </div>
            <div className="heading-actions">
              <button
                className="secondary refresh"
                onClick={() => refresh()}
                aria-label="Обновить"
              >
                <RefreshCw size={16} />
              </button>
              <button className="primary" onClick={() => setNewProject(true)}>
                <Plus size={17} />
                Новый проект
              </button>
            </div>
          </div>
          {(projects.isError || deployments.isError || system.isError) && (
            <div className="error">
              Не удалось обновить данные:{" "}
              {(projects.error || deployments.error || system.error)?.message}
            </div>
          )}
          {selected ? (
            <ProjectDetails
              project={
                projects.data?.find((p) => p.id === selected.id) || selected
              }
              back={() => setSelected(null)}
              action={action}
              notify={setNotice}
            />
          ) : (
            <>
              {page === "Overview" && (
                <>
                  {!mock && <HostMetrics />}
                  <div className="stats-grid">
                    {[
                      {
                        title: "Всего проектов",
                        value: system.data?.projects ?? "—",
                        note: "На вашем сервере",
                        icon: FolderGit2,
                      },
                      {
                        title: "Приложений работают",
                        value: system.data?.running ?? "—",
                        note: "Состояние из базы управления",
                        icon: Box,
                      },
                      {
                        title: "В очереди",
                        value: system.data?.queued ?? "—",
                        note: "Одна сборка одновременно",
                        icon: Clock3,
                      },
                      {
                        title: "Неудачных деплоев",
                        value: system.data?.failed ?? "—",
                        note: "За всё время",
                        icon: Activity,
                      },
                    ].map(({ title, value, note, icon: Icon }, i) => (
                      <article className="stat-card" key={title}>
                        <div>
                          {title}
                          <Icon size={17} />
                        </div>
                        <strong>
                          {value}
                          <span
                            className={
                              i === 3 && Number(value) > 0
                                ? "stat-marker warn"
                                : "stat-marker"
                            }
                          >
                            {i === 1 ? "●" : "↗"}
                          </span>
                        </strong>
                        <small>{note}</small>
                      </article>
                    ))}
                  </div>
                  <div className="section-heading">
                    <h2>
                      Ваши проекты <span>{projects.data?.length || 0}</span>
                    </h2>
                    <button
                      className="text-button"
                      onClick={() => setPage("Projects")}
                    >
                      Все проекты
                      <ArrowRight size={15} />
                    </button>
                  </div>
                </>
              )}
              {(page === "Overview" || page === "Projects") && (
                <>
                  <div className="projects-toolbar">
                    <div className="search">
                      <Search size={16} />
                      <input
                        aria-label="Поиск проектов"
                        placeholder="Поиск проектов…"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                      />
                    </div>
                    <span className="small muted">{items.length} projects</span>
                  </div>
                  <div className="project-grid">
                    {projects.isPending
                      ? [1, 2, 3].map((i) => (
                          <div className="project-card skeleton" key={i} />
                        ))
                      : items
                          .slice(0, page === "Overview" ? 6 : 200)
                          .map((p) => (
                            <article className="project-card" key={p.id}>
                              <div className="project-top">
                                <span className="project-icon">
                                  {p.source_type === "git" ? (
                                    <Github size={21} />
                                  ) : (
                                    <Box size={21} />
                                  )}
                                </span>
                                <Badge status={p.status} />
                                <button
                                  className="icon-button"
                                  aria-label={`Открыть ${p.name}`}
                                  onClick={() => setSelected(p)}
                                >
                                  <MoreHorizontal size={19} />
                                </button>
                              </div>
                              <button
                                className="project-title"
                                onClick={() => setSelected(p)}
                              >
                                {p.name}
                                <ArrowUpRight size={16} />
                              </button>
                              <p className="project-source">
                                {p.source.replace("https://github.com/", "")}
                              </p>
                              <div className="project-domain">
                                <Globe2 size={13} />
                                {p.domain || "Без домена"}
                              </div>
                              <div className="project-meta">
                                <span>
                                  <FolderGit2 size={13} />
                                  {p.branch}
                                </span>
                                <span>
                                  {p.memory_mb} MB · {p.cpu} CPU
                                </span>
                              </div>
                              <div className="project-bottom">
                                <span>
                                  <i
                                    className={
                                      p.status === "RUNNING"
                                        ? "online-dot"
                                        : "offline-dot"
                                    }
                                  />
                                  {date(p.created_at)}
                                </span>
                                <button
                                  className="text-button"
                                  onClick={() => action(p, "deploy")}
                                >
                                  <Rocket size={14} />
                                  Deploy
                                </button>
                              </div>
                            </article>
                          ))}
                    <button
                      className="add-project-card"
                      onClick={() => setNewProject(true)}
                    >
                      <span>
                        <Plus size={23} />
                      </span>
                      <b>Разверните что-нибудь новое</b>
                      <small>Из репозитория или Docker-образа</small>
                    </button>
                  </div>
                </>
              )}
              {(page === "Overview" || page === "Deployments") && (
                <section className="deployments-section">
                  <div className="section-heading">
                    <h2>
                      Последние деплои
                      <span className="live-tag">
                        <i />
                        LIVE
                      </span>
                    </h2>
                    <span className="small muted">
                      Обновляется автоматически
                    </span>
                  </div>
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>ПРОЕКТ / DEPLOYMENT</th>
                          <th>СТАТУС</th>
                          <th>ВЕРСИЯ</th>
                          <th>СОЗДАН</th>
                          <th />
                        </tr>
                      </thead>
                      <tbody>
                        {deploys
                          .slice(0, page === "Overview" ? 5 : 100)
                          .map((d) => (
                            <tr key={d.id}>
                              <td>
                                <div className="table-project">
                                  <span className="tiny-icon">
                                    <Rocket size={15} />
                                  </span>
                                  <div>
                                    <b>{d.project_name}</b>
                                    <small>#{d.id.slice(0, 8)}</small>
                                  </div>
                                </div>
                              </td>
                              <td>
                                <Badge status={d.status} />
                              </td>
                              <td>
                                <code className="commit">
                                  {d.commit_sha.slice(0, 7) || "—"}
                                </code>
                              </td>
                              <td className="muted">{date(d.created_at)}</td>
                              <td>
                                <button
                                  className="text-button"
                                  onClick={() => setLogs(d.id)}
                                >
                                  <Terminal size={15} />
                                  Логи
                                </button>
                                {["QUEUED", "CLONING", "BUILDING"].includes(
                                  d.status,
                                ) && (
                                  <button
                                    className="icon-button"
                                    aria-label="Отменить деплой"
                                    onClick={async () => {
                                      try {
                                        await post(
                                          `/deployments/${d.id}/cancel`,
                                        );
                                        setNotice("Отмена запрошена");
                                      } catch (err) {
                                        setNotice((err as Error).message);
                                      }
                                    }}
                                  >
                                    <X size={14} />
                                  </button>
                                )}
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                    {deploys.length === 0 && (
                      <div className="empty-state">
                        <Rocket size={24} />
                        <b>Здесь начинается история релизов</b>
                        <p>
                          Создайте проект и нажмите Deploy. Логи появятся
                          автоматически.
                        </p>
                      </div>
                    )}
                  </div>
                </section>
              )}
              {page === "Domains" && (
                <section className="panel">
                  <h2>Маршруты приложений</h2>
                  <p className="muted">
                    Домен указывается при создании проекта. Направьте A-запись
                    на публичный IP VPS. После успешного healthcheck агент
                    обновит Caddy.
                  </p>
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>ДОМЕН</th>
                          <th>ПРОЕКТ</th>
                          <th>СТАТУС ПРИЛОЖЕНИЯ</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(projects.data || [])
                          .filter((p) => p.domain)
                          .map((p) => (
                            <tr key={p.id}>
                              <td>
                                <a
                                  href={`https://${p.domain}`}
                                  target="_blank"
                                  rel="noreferrer"
                                >
                                  {p.domain} ↗
                                </a>
                              </td>
                              <td>{p.name}</td>
                              <td>
                                <Badge status={p.status} />
                                <DomainCheck id={p.id} />
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                  <p className="small muted">
                    Проверка DNS/HTTPS выполняется по кнопке и требует SERVER_IP
                    в .env. RUNNING обозначает состояние приложения.
                  </p>
                </section>
              )}
              {page === "Monitoring" && (
                <section className="panel">
                  <HostMetrics />
                  <Activity className="active-text" size={30} />
                  <h2>Состояние приложений</h2>
                  <p className="muted">
                    Статусы обновляются каждые 5 секунд, метрики агента — каждые
                    10 секунд.
                  </p>
                  {(projects.data || []).map((p) => (
                    <div className="monitor-row" key={p.id}>
                      <b>{p.name}</b>
                      <Badge status={p.status} />
                      <span className="muted">
                        Лимит: {p.memory_mb} MB / {p.cpu} CPU
                      </span>
                    </div>
                  ))}
                </section>
              )}
              {page === "GitHub" && <GitHubView />}
              {page === "Databases" && <DatabasesView notify={setNotice} />}
              {page === "Backups" && (
                <section className="panel">
                  <HardDrive size={30} className="active-text" />
                  <h2>Резервные копии управляющей базы</h2>
                  <p className="muted">
                    Запускаются на сервере. Архив содержит PostgreSQL и
                    конфигурацию. Секреты в копии конфигурации требуют закрытого
                    хранения.
                  </p>
                  <pre className="command">
                    bash infrastructure/scripts/backup.sh
                  </pre>
                  <p>
                    Восстановление:{" "}
                    <code>infrastructure/scripts/restore.sh</code>
                  </p>
                  <div className="info-box">
                    <ShieldCheck size={18} />
                    <span>
                      Копии пользовательских volumes и управляемых баз пока не
                      реализованы. Копию нужно экспортировать с VPS и проверить
                      восстановление.
                    </span>
                  </div>
                </section>
              )}
              {page === "Settings" && (
                <SettingsView email={session.data.email} notify={setNotice} />
              )}
            </>
          )}
          <footer className="page-footer">
            <span>
              <span className="fox">◈</span> Built to ship. Made to be yours.
            </span>
            <span>DeployFox · self-hosted deployment platform</span>
          </footer>
        </main>
      </div>
      {notice && (
        <div className="toast" role="status">
          <span>{notice}</span>
          <button
            aria-label="Закрыть уведомление"
            onClick={() => setNotice("")}
          >
            <X size={16} />
          </button>
        </div>
      )}
      {newProject && (
        <NewProject
          close={() => setNewProject(false)}
          saved={() => {
            setNewProject(false);
            refresh();
            setNotice("Проект создан. Можно запускать Deploy.");
          }}
        />
      )}
      {logs && <LogViewer id={logs} close={() => setLogs("")} />}
    </div>
  );
}
