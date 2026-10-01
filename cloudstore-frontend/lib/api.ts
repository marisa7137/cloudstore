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

export type FileInfo = {
  id: number;
  name: string;
  size: number;
  md5: string;
  mime_type: string;
  status: "uploading" | "complete" | "failed" | "unknown";
  created_at: string;
  updated_at: string;
};

export type FileList = {
  files: FileInfo[];
  total_size: number;
};

export const listFiles = () => api<FileList>("/api/files");

export const CHUNK_SIZE = 1024 * 1024; // 1 MiB, must stay under the gateway's 2 MiB limit

export const initUpload = (
  name: string,
  size: number,
  mime_type: string,
  chunk_count: number
) =>
  api<{ file_id: number; task_id: number }>("/api/uploads", {
    method: "POST",
    body: JSON.stringify({ name, size, mime_type, chunk_count }),
  });

// Raw binary body, so this doesn't go through the JSON helper.
async function uploadChunk(fileId: number, index: number, chunk: Blob) {
  const res = await fetch(`${API_BASE}/api/uploads/${fileId}/chunks/${index}`, {
    method: "PUT",
    credentials: "include",
    body: chunk,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error ?? "chunk upload failed");
  }
}

export const completeUpload = (fileId: number) =>
  api<FileInfo>(`/api/uploads/${fileId}/complete`, {
    method: "POST",
    body: JSON.stringify({}),
  });

// uploadFile drives the whole flow: init -> N chunks -> complete.
// onProgress is called with a 0..1 fraction after each chunk.
export async function uploadFile(
  file: File,
  onProgress?: (fraction: number) => void
): Promise<FileInfo> {
  const chunkCount = Math.max(1, Math.ceil(file.size / CHUNK_SIZE));
  const { file_id } = await initUpload(
    file.name,
    file.size,
    file.type,
    chunkCount
  );

  for (let i = 0; i < chunkCount; i++) {
    const chunk = file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE);
    await uploadChunk(file_id, i, chunk);
    onProgress?.((i + 1) / chunkCount);
  }

  return completeUpload(file_id);
}
