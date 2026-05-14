import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  Outlet,
  Link,
  createRootRouteWithContext,
  useRouter,
  HeadContent,
  Scripts,
} from "@tanstack/react-router";

import appCss from "../styles.css?url";

function NotFoundComponent() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="max-w-md text-center">
        <h1 className="text-7xl font-bold text-foreground">404</h1>
        <h2 className="mt-4 text-xl font-semibold text-foreground">Page not found</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <div className="mt-6">
          <Link
            to="/"
            className="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            Go home
          </Link>
        </div>
      </div>
    </div>
  );
}

function ErrorComponent({ error, reset }: { error: Error; reset: () => void }) {
  // Always log the full error to the devtools console for the dev workflow.
  // Even with no DevTools open, the rendered card below now surfaces the
  // message + stack so the user can copy it without ceremony.
  console.error(error);
  const router = useRouter();

  // Dev mode (vite serves with import.meta.env.DEV === true) shows the full
  // stack inline. In production we hide it behind a <details> so end users
  // don't see internals by default.
  const isDev = (import.meta as { env?: { DEV?: boolean } }).env?.DEV === true;
  const message = error?.message || String(error);
  const stack = error?.stack || "";

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="w-full max-w-2xl">
        <div className="text-center">
          <h1 className="text-xl font-semibold tracking-tight text-foreground">
            This page didn't load
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Something went wrong on our end. The error below should help diagnose it.
          </p>
        </div>

        {/* Visible error panel — copy-paste friendly. */}
        <div className="mt-4 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-left">
          <div className="font-mono text-[10px] uppercase tracking-wider text-destructive/80">
            error message
          </div>
          <pre className="mt-1 whitespace-pre-wrap break-words font-mono text-xs text-foreground">
            {message}
          </pre>
          {isDev && stack && (
            <details className="mt-3">
              <summary className="cursor-pointer font-mono text-[10px] uppercase tracking-wider text-muted-foreground hover:text-foreground">
                stack trace
              </summary>
              <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-relaxed text-muted-foreground">
                {stack}
              </pre>
            </details>
          )}
        </div>

        <div className="mt-6 flex flex-wrap justify-center gap-2">
          <button
            onClick={() => {
              router.invalidate();
              reset();
            }}
            className="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            Try again
          </button>
          <a
            href="/"
            className="inline-flex items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent"
          >
            Go home
          </a>
        </div>
      </div>
    </div>
  );
}

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: "JobScout" },
      { name: "description", content: "High-performance tech job aggregator for Bangladesh." },
      { name: "author", content: "JobScout" },
      { property: "og:title", content: "JobScout" },
      {
        property: "og:description",
        content: "High-performance tech job aggregator for Bangladesh.",
      },
      { property: "og:type", content: "website" },
      { name: "twitter:card", content: "summary" },
    ],
    links: [
      {
        rel: "stylesheet",
        href: appCss,
      },
    ],
  }),
  shellComponent: RootShell,
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
  errorComponent: ErrorComponent,
});

function RootShell({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  );
}

function RootComponent() {
  const { queryClient } = Route.useRouteContext();

  return (
    <QueryClientProvider client={queryClient}>
      <Outlet />
    </QueryClientProvider>
  );
}
