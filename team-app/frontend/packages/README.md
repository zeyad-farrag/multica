# Frontend packages

This directory holds platform-agnostic, reusable code for the team-app frontend. The split mirrors the Multica monorepo's package boundaries (AR22 of Story 1.1, and CLAUDE.md "Package Boundary Rules"):

- `core/team/` — headless business logic (stores, query hooks, service callers). MUST NOT import `next/*`, `react-router-dom`, `react-dom`, UI libraries (`@base-ui/react`, lucide-react, `tailwindcss/*`), `process.env`, or `window` / `localStorage` directly. Pass platform-specific things in via parameters or adapters.
- `views/team/` — cross-platform view components. MUST NOT import `next/*` or `react-router-dom`, and MUST NOT import Zustand stores directly (subscribe through `core/team/` exports).
- `app/` (in `frontend/app/`) — the only place where Next.js APIs (`next/navigation`, `next/headers`, route handlers, server actions) may be used.

These rules are enforced by `eslint.config.mjs` via `no-restricted-imports` overrides keyed on file path. Run `pnpm lint` to verify; CI (`team-app-frontend` job) fails on any violation.
