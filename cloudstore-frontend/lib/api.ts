const API_BASE = "http://localhost:8080";

export type User = {
  id: number;
  username: string;
  created_at: string;
};

export async function api<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    credentials: "include", // send/receive the session cookie
    headers: options.body ? { "Content-Type": "application/json" } : undefined,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error ?? "request failed");
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const register = (username: string, password: string) =>
  api<User>("/api/register", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });

export const login = (username: string, password: string) =>
  api<User>("/api/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });

export const logout = () => api<void>("/api/logout", { method: "POST" });

export const me = () => api<User>("/api/me");
