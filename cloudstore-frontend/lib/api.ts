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

// hashFile computes the sha256 of the whole file, used as a fingerprint to
// detect a source file changed between init / resume / complete.
// Note: WebCrypto has no streaming API, so the file is read into memory —
// fine under our 1 GiB quota, a real system would use an incremental hasher.
export async function hashFile(file: File): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", await file.arrayBuffer());
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export const initUpload = (
  name: string,
  size: number,
  mime_type: string,
  chunk_count: number,
  source_hash: string,
  source_modified: number
) =>
  api<{ file_id: number; task_id: number }>("/api/uploads", {
    method: "POST",
    body: JSON.stringify({
      name,
      size,
      mime_type,
      chunk_count,
      source_hash,
      source_modified,
    }),
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

export type TaskInfo = {
  id: number;
  file_id: number;
  file_name: string;
  file_size: number;
  type: string;
  status: "in_progress" | "complete" | "failed" | "unknown";
  chunks_total: number;
  chunks_received: number;
  source_modified: number;
  created_at: string;
  updated_at: string;
};

export const listTasks = () => api<{ tasks: TaskInfo[] }>("/api/tasks");

// resumeUpload continues an interrupted upload: proves the source file is
// unchanged (mtime fast-path, then sha256), asks the server which chunks are
// missing (the "cursor" lives server-side as chunk files on disk), sends only
// those, then completes. A changed source fails the task server-side.
export async function resumeUpload(
  file: File,
  task: TaskInfo,
  onProgress?: (fraction: number) => void
): Promise<FileInfo> {
  // Cheap check first: if mtime differs, skip hashing — the server will see
  // the mismatch and fail the task.
  const source_hash =
    file.lastModified === task.source_modified ? await hashFile(file) : "";

  const { missing_chunks } = await api<{
    chunks_total: number;
    missing_chunks: number[];
  }>(`/api/uploads/${task.file_id}/resume`, {
    method: "POST",
    body: JSON.stringify({ source_hash, source_modified: file.lastModified }),
  });

  let done = 0;
  for (const i of missing_chunks) {
    const chunk = file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE);
    await uploadChunk(task.file_id, i, chunk);
    onProgress?.(++done / missing_chunks.length);
  }

  return completeUpload(task.file_id);
}

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
    chunkCount,
    await hashFile(file),
    file.lastModified
  );

  for (let i = 0; i < chunkCount; i++) {
    const chunk = file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE);
    await uploadChunk(file_id, i, chunk);
    onProgress?.((i + 1) / chunkCount);
  }

  return completeUpload(file_id);
}
