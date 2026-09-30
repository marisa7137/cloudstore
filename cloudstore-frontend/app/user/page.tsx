"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { me, logout, type User } from "@/lib/api";

export default function UserPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    me()
      .then(setUser)
      .catch(() => router.push("/login")); // not logged in
  }, [router]);

  async function onLogout() {
    await logout();
    router.push("/login");
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
        <button
          onClick={onLogout}
          className="rounded bg-gray-800 px-4 py-2 text-white"
        >
          Log out
        </button>
      </div>
    </main>
  );
}
