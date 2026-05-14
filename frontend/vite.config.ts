import { defineConfig } from "vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import tsConfigPaths from "vite-tsconfig-paths";
import tailwindcss from "@tailwindcss/vite";

// JobScout — TanStack Start + React 19 + Tailwind v4.
// `src/server.ts` is our SSR entry; it is referenced via the `start.entry`
// option below. The TanStack Start plugin handles routing, SSR, manifest,
// and prod build; we add Tailwind, the React plugin, and tsconfig path
// resolution (`@/*` → `src/*`) explicitly.
export default defineConfig({
  plugins: [
    tsConfigPaths({ projects: ["./tsconfig.json"] }),
    tailwindcss(),
    tanstackStart({
      // `src/start.ts` exports `startInstance` (createStart + middleware).
      // `src/server.ts` is the Cloudflare worker fetch entry (wired via wrangler.jsonc).
      start: { entry: "start" },
    }),
    viteReact(),
  ],
});
