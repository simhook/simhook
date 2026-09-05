/**
 * The readable flag cookie the API sets next to the httpOnly session cookie.
 * It carries no secret and only ever means "probably signed in"; the API is
 * the only writer of both cookies and the only judge of a session.
 */
export const SIGNED_IN_COOKIE = "simhook_signed_in";

/** Whether this browser currently holds the flag. */
export function hasSignedInCookie(): boolean {
  try {
    return document.cookie.split(";").some((c) => c.trim().startsWith(`${SIGNED_IN_COOKIE}=`));
  } catch {
    return false;
  }
}

/**
 * Where to go after signing in. Only a path on this site will do: anything
 * that could leave it (a scheme, a protocol-relative address) or land back on
 * a sign-in page falls back to the dashboard.
 */
export function safeNext(next: string | null | undefined, fallback = "/dashboard"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  const path = next.split(/[?#]/, 1)[0];
  if (path === "/login" || path === "/register" || path === "/reset-password" || path === "/") return fallback;
  return next;
}
