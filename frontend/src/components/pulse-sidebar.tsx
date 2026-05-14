import { useMemo } from "react";
import { Activity, Archive, GitMerge, Radar, AlertTriangle } from "lucide-react";
import { timeAgo } from "@/lib/format";
import type { PulseEvent } from "@/lib/pulse-socket";
import { usePulse } from "@/lib/pulse-context";

const KIND = {
  scrape: { Icon: Radar, color: "text-indigo", label: "SCRAPE" },
  merge: { Icon: GitMerge, color: "text-lime", label: "MERGE" },
  archive: { Icon: Archive, color: "text-muted-foreground", label: "JANITOR" },
  alert: { Icon: AlertTriangle, color: "text-destructive", label: "ALERT" },
} as const;

export function PulseSidebar() {
  const { events, connected } = usePulse();

  // Aggregate counters from the rolling event buffer.
  const stats = useMemo(() => {
    let scrapes = 0;
    let merges = 0;
    let errors = 0;
    for (const e of events) {
      if (e.kind === "scrape") scrapes++;
      else if (e.kind === "merge") merges++;
      else if (e.kind === "alert") errors++;
    }
    return { scrapes, merges, errors };
  }, [events]);

  return (
    <aside className="hidden xl:flex w-[320px] shrink-0 flex-col border-l border-border bg-surface/30 sticky top-14 h-[calc(100vh-3.5rem)]">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <span className="grid h-6 w-6 place-items-center rounded border border-border bg-background">
            <Activity className="h-3.5 w-3.5 text-lime" />
          </span>
          <span className="font-mono text-[11px] uppercase tracking-wider text-foreground">
            System Pulse
          </span>
        </div>
        <span className="inline-flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              connected ? "bg-lime live-dot" : "bg-muted-foreground"
            }`}
          />
          {connected ? "LIVE" : "OFFLINE"}
        </span>
      </div>

      <div className="grid grid-cols-3 gap-px bg-border border-b border-border">
        {[
          { l: "Scrapes", v: String(stats.scrapes), tone: "text-foreground" },
          { l: "Merged", v: String(stats.merges), tone: "text-lime" },
          {
            l: "Errors",
            v: String(stats.errors),
            tone: stats.errors ? "text-destructive" : "text-foreground",
          },
        ].map((s) => (
          <div key={s.l} className="bg-surface/50 px-3 py-2.5">
            <div className={`font-mono text-base font-semibold ${s.tone}`}>{s.v}</div>
            <div className="font-mono text-[9px] uppercase tracking-wider text-muted-foreground">
              {s.l}
            </div>
          </div>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto">
        {events.length === 0 ? (
          <div className="px-4 py-10 text-center font-mono text-[11px] text-muted-foreground">
            // {connected ? "awaiting events…" : "connecting…"}
          </div>
        ) : (
          <ol className="relative">
            {events.map((p, i) => (
              <PulseRow key={p.id} event={p} isLatest={i === 0} />
            ))}
          </ol>
        )}
      </div>

      <div className="border-t border-border px-4 py-2.5 font-mono text-[10px] text-muted-foreground flex items-center justify-between">
        <span>{connected ? "ws · live" : "ws · reconnecting"}</span>
        <span>v0.4.1-beta</span>
      </div>
    </aside>
  );
}

function PulseRow({ event, isLatest }: { event: PulseEvent; isLatest: boolean }) {
  const k = KIND[event.kind] ?? KIND.scrape;
  const Icon = k.Icon;
  return (
    <li className="relative px-4 py-3 border-b border-border/60 hover:bg-background/40 transition-colors">
      <div className="flex items-start gap-2.5">
        <span
          className={`mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded border border-border bg-background ${k.color}`}
        >
          <Icon className="h-3 w-3" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 mb-0.5">
            <span className={`font-mono text-[9px] uppercase tracking-wider ${k.color}`}>
              {k.label}
            </span>
            <span className="font-mono text-[10px] text-muted-foreground/70">
              {timeAgo(event.t)}
            </span>
          </div>
          <p className="text-xs text-foreground/90 leading-snug">{event.msg}</p>
        </div>
        {isLatest && <span className="mt-1.5 h-1.5 w-1.5 rounded-full bg-lime live-dot shrink-0" />}
      </div>
    </li>
  );
}
