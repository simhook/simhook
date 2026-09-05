"use client";

import { useEffect, useRef } from "react";

/**
 * Cloudflare Turnstile, the bot check on the sign-in forms. The widget is
 * invisible unless it needs the visitor to do something, takes the form's
 * width when it does, and hands back a token the API verifies once.
 */

declare global {
  interface Window {
    turnstile?: {
      render: (el: HTMLElement, opts: Record<string, unknown>) => string;
      reset: (id: string) => void;
      remove: (id: string) => void;
    };
  }
}

const SCRIPT = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
let loading: Promise<void> | null = null;

function loadScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve();
  loading ??= new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = SCRIPT;
    s.async = true;
    s.onload = () => resolve();
    s.onerror = () => {
      loading = null;
      reject(new Error("Turnstile did not load"));
    };
    document.head.appendChild(s);
  });
  return loading;
}

export function Turnstile({
  siteKey,
  onToken,
  resetKey = 0,
}: {
  siteKey: string;
  /** A token when the check passed, "" while waiting, null when the widget could not load. */
  onToken: (token: string | null) => void;
  /** Change it to ask for a fresh token, for example after a failed submit. */
  resetKey?: number;
}) {
  const host = useRef<HTMLDivElement>(null);
  const widget = useRef<string | null>(null);
  const report = useRef(onToken);

  useEffect(() => {
    report.current = onToken;
  }, [onToken]);

  useEffect(() => {
    let gone = false;
    loadScript()
      .then(() => {
        if (gone || !host.current || !window.turnstile) return;
        widget.current = window.turnstile.render(host.current, {
          sitekey: siteKey,
          theme: "light",
          size: "flexible",
          appearance: "interaction-only",
          callback: (token: string) => report.current(token),
          "expired-callback": () => report.current(""),
          "error-callback": () => report.current(null),
        });
      })
      .catch(() => report.current(null));
    return () => {
      gone = true;
      if (widget.current && window.turnstile) window.turnstile.remove(widget.current);
      widget.current = null;
    };
  }, [siteKey]);

  useEffect(() => {
    if (resetKey && widget.current && window.turnstile) window.turnstile.reset(widget.current);
  }, [resetKey]);

  return <div ref={host} />;
}
