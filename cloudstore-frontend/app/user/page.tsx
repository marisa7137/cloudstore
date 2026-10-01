"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  me,
  logout,
  uploadFile,
  resumeUpload,
  listTasks,
  type User,
  type TaskInfo,
} from "@/lib/api";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n;
  let i = -1;
  do {
    v /= 1024;
    i++;
  } while (v >= 1024 && i < units.length - 1);
  return `${v.toFixed(1)} ${units[i]}`;
}

const statusColors: Record<string, string> = {
  complete: "text-green-600",
  in_progress: "text-yellow-600",
  failed: "text-red-600",
};

export default function UserPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const resumeInput = useRef<HTMLInputElement>(null);
  const [resumeTarget, setResumeTarget] = useState<TaskInfo | null>(null);
  const [progress, setProgress] = useState<number | null>(null); // null = idle
  const [uploadMsg, setUploadMsg] = useState<string | null>(null);
  const [tasks, setTasks] = useState<TaskInfo[]>([]);

  function refreshTasks() {
    listTasks()
      .then((r) => setTasks(r.tasks))
      .catch(() => {}); // storage may be down; keep the page usable
  }

  useEffect(() => {
    me()
      .then((u) => {
        setUser(u);
        refreshTasks();
      })
      .catch(() => router.push("/login")); // not logged in
  }, [router]);

  async function onLogout() {
    await logout();
    router.push("/login");
  }

  async function onFileChosen(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-selecting the same file
    if (!file) return;

    setUploadMsg(null);
    setProgress(0);
    try {
      const info = await uploadFile(file, setProgress);
      setUploadMsg(`Uploaded ${info.name} (${formatBytes(info.size)}, md5 ${info.md5})`);
    } catch (err) {
      setUploadMsg(`Upload failed: ${(err as Error).message}`);
    } finally {
      setProgress(null);
      refreshTasks();
    }
  }

  function onResumeClick(task: TaskInfo) {
    setResumeTarget(task);
    resumeInput.current?.click();
  }

  async function onResumeFileChosen(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    const task = resumeTarget;
    setResumeTarget(null);
    if (!file || !task) return;

    // The browser cannot keep a file handle across sessions, so the user
    // re-picks the file; make sure it is the same one before resuming.
    if (file.name !== task.file_name || file.size !== task.file_size) {
      setUploadMsg(
        `Selected file does not match "${task.file_name}" (${formatBytes(task.file_size)})`
      );
      return;
    }

    setUploadMsg(null);
    setProgress(0);
    try {
      const info = await resumeUpload(file, task.file_id, setProgress);
      setUploadMsg(`Resumed and completed ${info.name} (md5 ${info.md5})`);
    } catch (err) {
      setUploadMsg(`Resume failed: ${(err as Error).message}`);
    } finally {
      setProgress(null);
      refreshTasks();
    }
  }

  if (!user) {
    return (
      <main className="flex min-h-screen items-center justify-center">
        <p>Loading...</p>
      </main>
    );
  }

  return (
    <main className="mx-auto min-h-screen max-w-3xl space-y-6 p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Welcome, {user.username}</h1>
          <p className="text-sm text-gray-500">
            User #{user.id} · joined{" "}
            {new Date(user.created_at).toLocaleDateString()}
          </p>
        </div>
        <button
          onClick={onLogout}
          className="rounded bg-gray-800 px-4 py-2 text-white"
        >
          Log out
        </button>
      </div>

      <input
        ref={fileInput}
        type="file"
        className="hidden"
        onChange={onFileChosen}
      />
      <input
        ref={resumeInput}
        type="file"
        className="hidden"
        onChange={onResumeFileChosen}
      />

      <div className="space-y-2">
        <button
          onClick={() => fileInput.current?.click()}
          disabled={progress !== null}
          className="rounded bg-blue-600 px-4 py-2 text-white disabled:opacity-50"
        >
          {progress !== null
            ? `Uploading... ${Math.round(progress * 100)}%`
            : "Upload file"}
        </button>
        {progress !== null && (
          <div className="h-2 w-full overflow-hidden rounded bg-gray-200">
            <div
              className="h-full bg-blue-600 transition-all"
              style={{ width: `${progress * 100}%` }}
            />
          </div>
        )}
        {uploadMsg && <p className="text-sm text-gray-700">{uploadMsg}</p>}
      </div>

      <section className="space-y-2">
        <h2 className="text-lg font-semibold">Tasks</h2>
        {tasks.length === 0 ? (
          <p className="text-sm text-gray-500">No tasks yet.</p>
        ) : (
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="border-b text-left text-gray-500">
                <th className="py-2 pr-4">File</th>
                <th className="py-2 pr-4">Size</th>
                <th className="py-2 pr-4">Type</th>
                <th className="py-2 pr-4">Progress</th>
                <th className="py-2 pr-4">Status</th>
                <th className="py-2 pr-4">Updated</th>
                <th className="py-2"></th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((t) => (
                <tr key={t.id} className="border-b">
                  <td className="py-2 pr-4 font-medium">{t.file_name}</td>
                  <td className="py-2 pr-4">{formatBytes(t.file_size)}</td>
                  <td className="py-2 pr-4">{t.type}</td>
                  <td className="py-2 pr-4">
                    {t.chunks_received}/{t.chunks_total} chunks
                  </td>
                  <td className={`py-2 pr-4 ${statusColors[t.status] ?? ""}`}>
                    {t.status}
                  </td>
                  <td className="py-2 pr-4">
                    {new Date(t.updated_at).toLocaleString()}
                  </td>
                  <td className="py-2">
                    {t.status === "in_progress" && (
                      <button
                        onClick={() => onResumeClick(t)}
                        disabled={progress !== null}
                        className="rounded bg-yellow-600 px-3 py-1 text-white disabled:opacity-50"
                      >
                        Resume
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
