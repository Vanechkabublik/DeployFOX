"use client";
import { useEffect, useRef, useState } from "react";
import { Search, Trash2, ArrowDownToLine, Terminal, X } from "lucide-react";
export function LogViewer({
  id,
  close,
  application = false,
}: {
  id: string;
  close: () => void;
  application?: boolean;
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [follow, setFollow] = useState(true);
  const [connection, setConnection] = useState("Подключение…");
  const tail = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const stream = new EventSource(
      `/api/v1/${application ? "projects" : "deployments"}/${id}/logs`,
    );
    stream.onopen = () => setConnection("Live");
    stream.onmessage = (event) => {
      const data: { line: string; mode?: string } = JSON.parse(event.data);
      setLines((old) =>
        data.mode === "snapshot"
          ? data.line.split("\n").slice(-2000)
          : [...old, data.line].slice(-2000),
      );
    };
    stream.onerror = () => setConnection("Переподключение…");
    return () => stream.close();
  }, [id, application]);
  useEffect(() => {
    if (follow)
      tail.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [lines, follow]);
  return (
    <div className="overlay" onClick={close}>
      <section
        className="modal logs-modal"
        role="dialog"
        aria-modal="true"
        aria-label="Deployment logs"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-heading">
          <h2>
            <Terminal size={22} />{" "}
            {application ? "Application logs" : "Deployment logs"}
          </h2>
          <button className="icon-button" aria-label="Закрыть" onClick={close}>
            <X />
          </button>
        </div>
        <div className="log-toolbar">
          <div className="search">
            <Search size={15} />
            <input
              aria-label="Поиск по логам"
              placeholder="Фильтр логов…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <button
            className={follow ? "active-text icon-button" : "icon-button"}
            aria-label="Автопрокрутка"
            onClick={() => setFollow(!follow)}
          >
            <ArrowDownToLine size={18} />
          </button>
          <button
            className="icon-button"
            aria-label="Очистить отображение"
            onClick={() => setLines([])}
          >
            <Trash2 size={17} />
          </button>
          <span className="small muted">{connection}</span>
        </div>
        <div className="log-output">
          {lines
            .filter((line) => line.toLowerCase().includes(search.toLowerCase()))
            .map((line, i) => (
              <div key={i}>
                <span>{String(i + 1).padStart(3, "0")}</span>
                <code>{line}</code>
              </div>
            ))}
          {lines.length === 0 && (
            <p className="muted">Ожидаем сообщения worker…</p>
          )}
          <div ref={tail} />
        </div>
        <div className="log-footer">
          Deployment {id} · последние 2 000 строк
        </div>
      </section>
    </div>
  );
}
