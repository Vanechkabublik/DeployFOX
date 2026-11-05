"use client";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
interface Metrics {
  available: boolean;
  stats: { CPUPerc: string; MemUsage: string; MemPerc: string };
  state: {
    status: string;
    started_at: string;
    restarts: number;
    health: string;
  };
}
export function ProjectMetrics({
  id,
  active,
}: {
  id: string;
  active: boolean;
}) {
  const query = useQuery({
    queryKey: ["project-metrics", id],
    queryFn: () => api<Metrics>(`/projects/${id}/metrics`),
    enabled: active,
    refetchInterval: 10_000,
  });
  const data = query.data;
  if (query.isError)
    return (
      <p className="small muted">Метрики контейнера временно недоступны.</p>
    );
  if (!data?.available) return null;
  const uptime =
    data.state.status === "running"
      ? Math.max(
          0,
          Math.floor(
            (query.dataUpdatedAt - new Date(data.state.started_at).getTime()) /
              60_000,
          ),
        )
      : 0;
  return (
    <div className="detail-grid">
      <div>
        <span>CPU</span>
        <b>{data.stats.CPUPerc}</b>
      </div>
      <div>
        <span>RAM</span>
        <b>{data.stats.MemUsage}</b>
      </div>
      <div>
        <span>Uptime</span>
        <b>
          {uptime} минут · {data.state.restarts} restarts
        </b>
      </div>
      <div>
        <span>Docker health</span>
        <b>
          {data.state.health === "none"
            ? "В образе не задан · HTTP проверяется при деплое"
            : data.state.health}
        </b>
      </div>
    </div>
  );
}
