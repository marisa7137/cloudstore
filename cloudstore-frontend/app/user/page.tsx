"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { me, logout, uploadFile, type User } from "@/lib/api";

export default function UserPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [progress, setProgress] = useState<number | null>(null); // null = idle
  const [uploadMsg, setUploadMsg] = useState<string | null>(null);


  useEffect(() => {
    me()
      .then(setUser)
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
      setUploadMsg(`Uploaded ${info.name} (${info.size} bytes, md5 ${info.md5})`);
    } catch (err) {
      setUploadMsg(`Upload failed: ${(err as Error).message}`);
    } finally {
      setProgress(null);
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
    <main className="flex min-h-screen items-center justify-center">
      <div className="w-96 space-y-4">
        <h1 className="text-2xl font-bold">Welcome, {user.username}</h1>
        <p className="text-sm text-gray-500">
          User #{user.id} · joined {new Date(user.created_at).toLocaleDateString()}
        </p>
        <p className="text-sm text-gray-500">
          Your files will show up here once the storage service is built.
        </p>

        <input
          ref={fileInput}
          type="file"
          className="hidden"
          onChange={onFileChosen}
        />
        <div className="flex gap-2">
          <button
            onClick={() => fileInput.current?.click()}
            disabled={progress !== null}
            className="rounded bg-blue-600 px-4 py-2 text-white disabled:opacity-50"
          >
            {progress !== null
              ? `Uploading... ${Math.round(progress * 100)}%`
              : "Upload file"}
          </button>
          <button
            onClick={onLogout}
            className="rounded bg-gray-800 px-4 py-2 text-white"
          >
            Log out
          </button>
        </div>
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
    </main>
  );
}
