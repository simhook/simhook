import { redirect } from "next/navigation";

// The dashboard is the app. Signed-out visitors are bounced to login by the app shell.
export default function Home() {
  redirect("/dashboard");
}
