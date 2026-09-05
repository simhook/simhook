# simhook dashboard

The signed-in half of simhook.dev: phones, messages, webhooks, API keys, and account settings. A Next.js app that talks to the API from the browser with the session cookie and holds no credentials of its own.

Run it against a local API with `pnpm --filter web dev`; `NEXT_PUBLIC_API_URL` (see `.env.example`) says where the API is. The look is the same plain design as the site: tokens in `src/app/globals.css`, the shared bar and footer in `src/components/site-chrome.tsx`, and decision 014 in `docs/decisions.md` for the rules.
