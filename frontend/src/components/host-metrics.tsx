"use client";
import { useQuery } from "@tanstack/react-query";
import { Activity, Cpu, HardDrive, MemoryStick, Box } from "lucide-react";
import { api } from "@/lib/api";
interface Metrics {
  available: boolean;
  cpu_percent: number;
  memory_total: number;
  memory_used: number;
  disk_available: boolean;
  disk_total: number;
  disk_used: number;
  containers: number;
}
export function HostMetrics() {
  const query = useQuery({
    queryKey: ["host-metrics"],
    queryFn: () => api<Metrics>("/monitoring/host"),
    refetchInterval: 10_000,
  });
  const data = query.data;
  if (query.isError)
    return (
      <div className="error">Метрики недоступны: {query.error.message}</div>
    );
  if (!data?.available)
    return (
      <section className="panel">
        <Activity size={22} />
        <h2>Метрики хоста</h2>
        <p className="muted">
          {query.isPending
            ? "Получаем данные агента…"
            : "В mock-режиме метрики VPS не собираются. Для настоящих данных включите Docker-agent."}
        </p>
      </section>
    );
  const gb = (bytes: number) => (bytes / 1024 ** 3).toFixed(1);
  const cards = [
    {
      name: "CPU хоста",
      value: `${data.cpu_percent.toFixed(1)}%`,
      percent: data.cpu_percent,
      icon: Cpu,
      note: "Из /proc/stat · интервал 200 ms",
    },
    {
      name: "Память хоста",
      value: `${gb(data.memory_used)} / ${gb(data.memory_total)} GB`,
      percent: (100 * data.memory_used) / data.memory_total,
      icon: MemoryStick,
      note: "MemTotal − MemAvailable",
    },
    {
      name: "Диск Docker",
      value: data.disk_available
        ? `${gb(data.disk_used)} / ${gb(data.disk_total)} GB`
        : "Недоступен",
      percent: data.disk_available
        ? (100 * data.disk_used) / data.disk_total
        : 0,
      icon: HardDrive,
      note: "Файловая система Docker data root",
    },
    {
      name: "Контейнеров",
      value: String(data.containers),
      percent: 0,
      icon: Box,
      note: "Всего на Docker Engine",
    },
  ];
  return (
    <div className="stats-grid">
      {cards.map(({ name, value, percent, icon: Icon, note }) => (
        <article className="stat-card" key={name}>
          <div>
            {name}
            <Icon size={17} />
          </div>
          <strong className="metric-value">{value}</strong>
          <div className="usage-track">
            <span
              style={{ width: `${Math.max(0, Math.min(100, percent))}%` }}
            />
          </div>
          <small>{note}</small>
        </article>
      ))}
    </div>
  );
}
