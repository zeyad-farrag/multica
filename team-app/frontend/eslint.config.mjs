import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

/*
 * Package-boundary rules (Story 1.1 AC6 / AR22).
 *
 * `core/team/**` is the headless layer — it must run in any JS environment
 * (web, native shell). It cannot reach into framework, DOM, or UI APIs.
 *
 * `views/team/**` is the cross-platform UI layer. It can use UI libraries,
 * but it cannot bind to Next.js / React Router or pull stores directly.
 *
 * Violations fail `pnpm lint`; CI (`team-app-frontend` job) gates merges on
 * the same command.
 */
const corePatterns = [
  { group: ["next", "next/*"], message: "core/team/** must be framework-agnostic — no Next.js imports (AR22)." },
  { group: ["react-router-dom"], message: "core/team/** must be framework-agnostic — no react-router-dom (AR22)." },
  { group: ["react-dom", "react-dom/*"], message: "core/team/** is headless — no react-dom imports (AR22)." },
  { group: ["@base-ui/react", "@base-ui/react/*"], message: "core/team/** must not import UI libraries — keep components in views/team/** (AR22)." },
  { group: ["tailwindcss", "tailwindcss/*"], message: "core/team/** must not import Tailwind utilities directly (AR22)." },
  { group: ["lucide-react"], message: "core/team/** must not import icon libraries — keep them in views/team/** (AR22)." },
];

const viewsPatterns = [
  { group: ["next", "next/*"], message: "views/team/** must be framework-agnostic — no Next.js imports (AR22)." },
  { group: ["react-router-dom"], message: "views/team/** must be framework-agnostic — no react-router-dom (AR22)." },
  { group: ["**/core/team/store", "**/core/team/store/*"], message: "views/team/** must not import Zustand stores directly — subscribe via core/team/ exports (AR22)." },
];

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    files: ["packages/core/team/**/*.{ts,tsx,js,jsx,mjs,cjs}"],
    rules: {
      "no-restricted-imports": ["error", { patterns: corePatterns }],
      "no-restricted-globals": [
        "error",
        { name: "process", message: "core/team/** must not read process.env — pass config via parameters (AR22)." },
        { name: "window", message: "core/team/** must not touch window directly — use a storage/runtime adapter (AR22)." },
        { name: "localStorage", message: "core/team/** must not touch localStorage — use a storage adapter (AR22)." },
        { name: "document", message: "core/team/** must not touch document — keep DOM access in views/team/** or app/ (AR22)." },
      ],
      "no-restricted-properties": [
        "error",
        { object: "process", property: "env", message: "core/team/** must not read process.env — pass config via parameters (AR22)." },
      ],
    },
  },
  {
    files: ["packages/views/team/**/*.{ts,tsx,js,jsx,mjs,cjs}"],
    rules: {
      "no-restricted-imports": ["error", { patterns: viewsPatterns }],
    },
  },
]);

export default eslintConfig;
