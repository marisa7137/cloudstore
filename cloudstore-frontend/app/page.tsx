import { redirect } from "next/navigation";

export default function Home() {
  // /user bounces back to /login when not authenticated
  redirect("/user");
}
