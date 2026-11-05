let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
      ...options?.headers,
    },
    credentials: "same-origin",
  });
  const body = await response.json();
  if (!response.ok)
    throw new Error(body.error || `Request failed (${response.status})`);
  return body as T;
}
export function post<T>(path: string, body?: unknown) {
  return api<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}
