"use client";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
interface Check {
  dns_matches: boolean;
  certificate_valid: boolean;
  port_80: boolean;
  port_443: boolean;
  error?: string;
  certificate_expires?: string;
}
export function DomainCheck({ id }: { id: string }) {
  const query = useQuery({
    queryKey: ["domain-check", id],
    queryFn: () => api<Check>(`/projects/${id}/domain-check`),
    enabled: false,
  });
  return (
    <div>
      <button
        className="text-button"
        disabled={query.isFetching}
        onClick={() => query.refetch()}
      >
        {query.isFetching ? "Проверяем…" : "Проверить DNS / HTTPS"}
      </button>
      {query.data && (
        <div className="small muted">
          DNS {query.data.dns_matches ? "✓" : "✗"} · 80{" "}
          {query.data.port_80 ? "✓" : "✗"} · 443{" "}
          {query.data.port_443 ? "✓" : "✗"} · TLS{" "}
          {query.data.certificate_valid ? "✓" : "✗"}
          {query.data.error && (
            <p
              className="small break-all"
              style={{ maxWidth: 260, whiteSpace: "normal" }}
            >
              {query.data.error}
            </p>
          )}
          {query.data.certificate_expires && (
            <p>
              До{" "}
              {new Date(query.data.certificate_expires).toLocaleDateString(
                "ru-RU",
              )}
            </p>
          )}
        </div>
      )}
      {query.isError && <p className="small muted">{query.error.message}</p>}
    </div>
  );
}
