import { Link, useRouterState } from "@tanstack/react-router";
import { Activity, Bookmark, Crosshair, FlaskConical, Layers, Radar, Terminal } from "lucide-react";
import { cn } from "@/lib/utils";
import { ProfileWidget } from "./profile-widget";

const items = [
  { to: "/", label: "Global Feed", icon: Radar },
  { to: "/niches", label: "Niches", icon: Layers },
  { to: "/targets", label: "Targeting", icon: Crosshair },
  { to: "/scraper-health", label: "Scraper Health", icon: Activity },
  { to: "/admin/scrapers", label: "Dry-Run", icon: FlaskConical },
  { to: "/saved", label: "Saved Jobs", icon: Bookmark },
];

export function LateralNav() {
  const path = useRouterState({ select: (s) => s.location.pathname });

  return (
    <aside className="hidden md:flex h-screen w-14 shrink-0 flex-col items-center border-r border-border bg-surface/50 backdrop-blur-sm sticky top-0">
      <Link
        to="/"
        className="mt-3 grid h-9 w-9 place-items-center rounded-md border border-border bg-background"
      >
        <Terminal className="h-4 w-4 text-lime" />
      </Link>
      <nav className="mt-6 flex flex-col items-center gap-1">
        {items.map(({ to, label, icon: Icon }) => {
          const active = path === to;
          return (
            <Link
              key={to}
              to={to}
              title={label}
              className={cn(
                "group relative grid h-10 w-10 place-items-center rounded-md border border-transparent text-muted-foreground transition-colors hover:text-foreground hover:border-border hover:bg-surface",
                active && "text-foreground border-border bg-surface",
              )}
            >
              {active && (
                <span className="absolute -left-[1px] top-1/2 h-5 w-[2px] -translate-y-1/2 rounded-r bg-indigo" />
              )}
              <Icon className="h-[18px] w-[18px]" />
              <span className="pointer-events-none absolute left-12 z-50 whitespace-nowrap rounded-md border border-border bg-popover px-2 py-1 text-xs opacity-0 shadow-lg transition-opacity group-hover:opacity-100 font-mono">
                {label}
              </span>
            </Link>
          );
        })}
      </nav>
      <div className="mt-auto mb-3">
        <ProfileWidget />
      </div>
    </aside>
  );
}

export function MobileBottomNav() {
  const path = useRouterState({ select: (s) => s.location.pathname });
  return (
    <nav className="md:hidden fixed bottom-0 inset-x-0 z-40 flex border-t border-border bg-surface/90 backdrop-blur-md">
      {items.map(({ to, label, icon: Icon }) => {
        const active = path === to;
        return (
          <Link
            key={to}
            to={to}
            className={cn(
              "flex-1 flex flex-col items-center gap-1 py-2.5 text-[10px] font-mono uppercase tracking-wider",
              active ? "text-lime" : "text-muted-foreground",
            )}
          >
            <Icon className="h-4 w-4" />
            {label.split(" ")[0]}
          </Link>
        );
      })}
    </nav>
  );
}
