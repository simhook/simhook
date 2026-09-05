import { NextResponse, type NextRequest } from "next/server";
import { SIGNED_IN_COOKIE } from "@/lib/session-cookie";

/**
 * The dashboard's gate. The API sets a readable flag cookie on the parent
 * domain whenever a browser signs in and clears it whenever the session is
 * gone, so a request without the flag is almost certainly signed out: send it
 * to sign-in before any page loads, remembering where it was going.
 *
 * The flag only ever means "probably signed in". A visitor with the flag is
 * never trusted on its strength: the page still asks the API, which answers
 * for certain. And a visitor without the flag is never assumed signed out for
 * good: the sign-in page asks the API too, and if the API says otherwise it
 * hands the flag back, so a stale bounce corrects itself.
 *
 * Self-hosters whose dashboard and API share no parent domain cannot have the
 * flag; SIMHOOK_SESSION_FLAG=off makes this a no-op and the page-level check
 * does the routing on its own.
 */
export function proxy(request: NextRequest) {
  if (process.env.SIMHOOK_SESSION_FLAG === "off") return NextResponse.next();
  if (request.cookies.has(SIGNED_IN_COOKIE)) return NextResponse.next();
  const url = request.nextUrl.clone();
  const next = url.pathname + url.search;
  url.pathname = "/login";
  url.search = "";
  url.searchParams.set("next", next);
  url.searchParams.set("gate", "1");
  return NextResponse.redirect(url);
}

export const config = {
  matcher: [
    "/dashboard/:path*",
    "/devices/:path*",
    "/messages/:path*",
    "/sends/:path*",
    "/webhooks/:path*",
    "/api-keys/:path*",
    "/settings/:path*",
    "/verify-email",
  ],
};
