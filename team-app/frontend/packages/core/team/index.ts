/**
 * Headless team-domain logic for the team-app frontend.
 *
 * BOUNDARY (AR22, Story 1.1 AC6 — enforced by ESLint `no-restricted-imports`):
 *   - NO imports from `next/*`
 *   - NO imports from `react-router-dom`
 *   - NO imports from `react-dom`
 *   - NO UI library imports (`@base-ui/react`, `tailwindcss/*`, lucide-react, ...)
 *   - NO `process.env` reads — pass config in via parameters
 *   - NO direct `window` / `localStorage` reads — use a storage adapter (lands in Story 1.5+)
 *
 * This package is reusable across web (Next.js) and any future native shell.
 * Anything that violates the rules above belongs in `packages/views/team/`
 * or `app/` instead.
 *
 * Mutation policy reminder for future contributors:
 *   - Optimistic mutations by default (TanStack Query v5 onMutate / onError rollback).
 *   - Pessimistic only for gate-mediated ops (approval, autofill writes).
 */
export const TEAM_CORE_BOUNDARY = "packages/core/team" as const;
