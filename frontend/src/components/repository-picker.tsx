"use client";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
interface Repository {
  full_name: string;
  html_url: string;
  default_branch: string;
  private: boolean;
}
export function RepositoryPicker({
  source,
  choose,
  branch,
}: {
  source: string;
  choose: (url: string, branch: string) => void;
  branch: (value: string) => void;
}) {
  const repos = useQuery({
    queryKey: ["repositories"],
    queryFn: () => api<{ repositories: Repository[] }>("/github/repositories"),
  });
  const repository =
    source.match(
      /^https:\/\/github\.com\/([^/?#]+\/[^/?#]+?)(?:\.git)?\/?$/,
    )?.[1] || "";
  const branches = useQuery({
    queryKey: ["branches", repository],
    queryFn: () =>
      api<{ name: string }[]>(
        `/github/branches?repository=${encodeURIComponent(repository)}`,
      ),
    enabled: !!repository && !repos.isError,
  });
  return (
    <div className="repository-picker">
      {repos.data && (
        <label>
          Доступные GitHub-репозитории
          <select
            value=""
            onChange={(event) => {
              const repo = repos.data.repositories.find(
                (item) => item.html_url === event.target.value,
              );
              if (repo) choose(repo.html_url, repo.default_branch);
            }}
          >
            <option value="">Выберите репозиторий…</option>
            {repos.data.repositories.map((repo) => (
              <option key={repo.full_name} value={repo.html_url}>
                {repo.full_name}
                {repo.private ? " · private" : ""}
              </option>
            ))}
          </select>
        </label>
      )}
      {repos.isError && (
        <p className="small muted">
          Для приватных репозиториев подключите GitHub App в разделе GitHub.
          Публичный HTTPS URL можно ввести вручную.
        </p>
      )}
      {branches.data && (
        <label>
          Доступные ветки
          <select
            value=""
            onChange={(event) => {
              if (event.target.value) branch(event.target.value);
            }}
          >
            <option value="">Выберите ветку или введите ниже…</option>
            {branches.data.map((item) => (
              <option key={item.name}>{item.name}</option>
            ))}
          </select>
        </label>
      )}
    </div>
  );
}
