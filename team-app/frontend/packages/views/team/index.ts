/**
 * Cross-platform team-domain view components for the team-app frontend.
 *
 * BOUNDARY (AR22, Story 1.1 AC6 — enforced by ESLint `no-restricted-imports`):
 *   - NO imports from `next/*` (route files in `app/` are the only allowed Next.js touchpoint)
 *   - NO imports from `react-router-dom`
 *   - NO Zustand-store imports — stores live in `packages/core/team/store.ts`
 *
 * Views consume domain logic from `packages/core/team/` and render UI; routing
 * is injected via a `NavigationAdapter` analogue in a later story.
 */
export const TEAM_VIEWS_BOUNDARY = "packages/views/team" as const;
