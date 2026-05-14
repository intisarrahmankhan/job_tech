import { createFileRoute } from "@tanstack/react-router";
import { DashboardShell } from "@/components/dashboard-shell";
import { AlertCircle, CheckCircle2, Loader2, Pause, Play, RotateCw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import {
  useKillScraper,
  usePauseScraper,
  usePauseTarget,
  useResumeScraper,
  useResumeTarget,
  useRunTarget,
  useScraperLogs,
  useScraperMetrics,
  useScraperState,
  useTargets,
  type ScrapeTask,
  type ScraperLogRow,
  type TaskRollup,
} from "@/lib/api";
import { prettyCompanyName, prettyHostname } from "@/lib/pretty-name";
import { timeAgo } from "@/lib/format";

export const Route = createFileRoute("/scraper-health")({
  head: () => ({ meta: [{ title: "Scout · Scraper Health" }] }),
  component: ScraperHealth,
});

// Visual config for each ScrapeTask status. Mirrors the palette used on
// the Targeting Dashboard so the two pages feel cohesive.
const STATUS = {
  pending: {
    Icon: Pause,
    color: "text-muted-foreground",
    border: "border-border",
    bg: "bg-surface",
    label: "PENDING",
  },
  running: {
    Icon: Loader2,
    color: "text-indigo",
    border: "border-indigo/40",
    bg: "bg-indigo/10",
    label: "RUNNING",
  },
  healthy: {
    Icon: CheckCircle2,
    color: "text-lime",
    border: "border-lime/40",
    bg: "bg-lime/10",
    label: "HEALTHY",
  },
  failed: {
    Icon: AlertCircle,
    color: "text-destructive",
    border: "border-destructive/40",
    bg: "bg-destructive/10",
    label: "FAILED",
  },
} as const;

function ScraperHealth() {
  return <DashboardShell showPulse={false}>{() => <ScraperHealthBody />}</DashboardShell>;
}

function ScraperHealthBody() {
  const { data: targets = [] } = useTargets();
  const { data: scraper } = useScraperState();
  const { data: metrics } = useScraperMetrics("24h");
  const { data: logs = [] } = useScraperLogs(undefined, 25);
  const pauseGlobal = usePauseScraper();
  const resumeGlobal = useResumeScraper();
  const killScraper = useKillScraper();
  const [query, setQuery] = useState("");

  // Only DIRECT_URL targets show up here — keywords are scope refiners
  // for the global scraper and live on /targets, not on the health matrix.
  const sites = useMemo(() => targets.filter((t) => t.type === "DIRECT_URL"), [targets]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sites;
    return sites.filter((t) => {
      const label = prettyCompanyName(t.value).toLowerCase();
      const host = prettyHostname(t.value).toLowerCase();
      return label.includes(q) || host.includes(q) || t.value.toLowerCase().includes(q);
    });
  }, [sites, query]);

  const totals = useMemo(() => {
    let healthy = 0;
    let degraded = 0;
    let records = 0;
    for (const t of sites) {
      if (t.status === "healthy") healthy++;
      else if (t.status === "failed") degraded++;
      records += t.resultCount ?? 0;
    }
    return { healthy, degraded, records };
  }, [sites]);

  const isPaused = !!scraper?.paused;

  return (
    <div className="space-y-4 p-3 sm:p-4">
      {/* Header + global controls */}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div className="mb-1 font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground">
            /scraper-health
          </div>
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
            Scraper Health Matrix
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Live status of every site you target · {sites.length} pipeline
            {sites.length === 1 ? "" : "s"}
          </p>
        </div>

        {/* Global pause/start */}
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[10px] uppercase tracking-wider",
              isPaused
                ? "border-destructive/40 bg-destructive/10 text-destructive"
                : "border-lime/40 bg-lime/10 text-lime",
            )}
          >
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                isPaused ? "bg-destructive" : "bg-lime live-dot",
              )}
            />
            {isPaused ? "GLOBAL PAUSED" : "GLOBAL ACTIVE"}
          </span>

          {isPaused ? (
            <button
              type="button"
              onClick={() => resumeGlobal.mutate()}
              disabled={resumeGlobal.isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-lime/50 bg-lime/10 px-3 py-1.5 font-mono text-[11px] uppercase tracking-wider text-lime transition-colors hover:bg-lime/20 disabled:opacity-50"
            >
              {resumeGlobal.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Play className="h-3.5 w-3.5" />
              )}
              Start All
            </button>
          ) : (
            <button
              type="button"
              onClick={() => pauseGlobal.mutate()}
              disabled={pauseGlobal.isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-1.5 font-mono text-[11px] uppercase tracking-wider text-destructive transition-colors hover:bg-destructive/20 disabled:opacity-50"
            >
              {pauseGlobal.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Pause className="h-3.5 w-3.5" />
              )}
              Pause All
            </button>
          )}
          {/* Force-kill the in-flight subprocess. Useful when a previous
              run is wedged inside a Playwright wait_for_selector loop and
              "Restart" can't proceed because the running flag is stuck. */}
          {scraper?.running && (
            <button
              type="button"
              onClick={() => killScraper.mutate()}
              disabled={killScraper.isPending}
              title={`Kill PID ${scraper?.lastPid ?? "?"}`}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 px-3 py-1.5 font-mono text-[11px] uppercase tracking-wider text-destructive transition-colors hover:bg-destructive/15 disabled:opacity-50"
            >
              {killScraper.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <AlertCircle className="h-3.5 w-3.5" />
              )}
              Kill PID {scraper?.lastPid ?? "?"}
            </button>
          )}
        </div>
      </div>

      {/* Last error from the singleton manager (separate from per-target
          rows because a single global error often applies to no specific
          row, e.g. a Playwright install crash). */}
      {scraper?.lastError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3">
          <div className="font-mono text-[10px] uppercase tracking-wider text-destructive">
            last manager error
          </div>
          <div className="mt-1 break-words font-mono text-xs text-foreground">
            {scraper.lastError}
          </div>
        </div>
      )}

      {/* Stats strip */}
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-border bg-border lg:grid-cols-4">
        <Stat label="Pipelines" value={sites.length} tone="text-foreground" />
        <Stat label="Healthy" value={totals.healthy} tone="text-lime" />
        <Stat label="Degraded" value={totals.degraded} tone="text-destructive" />
        <Stat label="Records" value={totals.records.toLocaleString()} tone="text-indigo" />
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="search by company or hostname (e.g. linkedin)"
          className="w-full rounded-md border border-border bg-surface/40 py-2 pl-9 pr-3 font-mono text-sm placeholder:text-muted-foreground/60 focus:border-indigo/60 focus:outline-none focus:ring-1 focus:ring-indigo/60"
        />
      </div>

      {/* Pipeline list */}
      <section className="overflow-hidden rounded-md border border-border bg-surface/20">
        <div className="grid grid-cols-[1fr_110px_130px_80px_110px_220px] gap-3 border-b border-border bg-background/30 px-4 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          <span>pipeline</span>
          <span>status</span>
          <span>last run</span>
          <span>records</span>
          <span title="Scraper failures over the last 24h">err rate</span>
          <span className="text-right">actions</span>
        </div>

        {filtered.length === 0 ? (
          <div className="p-6 text-center font-mono text-sm text-muted-foreground">
            {sites.length === 0
              ? "// no custom links yet — add one on /targets to start scraping"
              : `// no results for "${query}"`}
          </div>
        ) : (
          filtered.map((t) => (
            <SiteRow
              key={t.id}
              target={t}
              rollup={metrics?.byTask?.[t.id]}
              globalPaused={isPaused}
            />
          ))
        )}
      </section>

      {/* Recent failure log — surfaces the captured stderr + reason from
          every scraper crash so the user has somewhere to look when the
          per-row "FAILED" badge shows up without an obvious cause. */}
      {logs.length > 0 && <FailureLog rows={logs} />}
    </div>
  );
}

function FailureLog({ rows }: { rows: ScraperLogRow[] }) {
  const [openId, setOpenId] = useState<string | null>(null);
  return (
    <section className="overflow-hidden rounded-md border border-border bg-surface/20">
      <div className="border-b border-border bg-background/30 px-4 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        recent scraper failures · {rows.length} entries
      </div>
      {rows.map((r) => {
        const open = openId === r.id;
        return (
          <div key={r.id} className="border-b border-border/60 last:border-b-0">
            <button
              type="button"
              onClick={() => setOpenId(open ? null : r.id)}
              className="grid w-full grid-cols-[110px_1fr_80px_auto] items-center gap-3 px-4 py-2 text-left transition-colors hover:bg-background/40"
            >
              <span className="font-mono text-[10px] text-muted-foreground">
                {new Date(r.createdAt).toLocaleString()}
              </span>
              <span className="truncate font-mono text-xs text-foreground">
                {r.error || "(no message)"}
              </span>
              <span className="font-mono text-[10px] text-destructive">exit {r.exitCode}</span>
              <span className="font-mono text-[10px] text-muted-foreground">
                {open ? "▾" : "▸"}
              </span>
            </button>
            {open && r.details && (
              <pre className="m-0 max-h-[400px] overflow-auto whitespace-pre-wrap break-words bg-background/50 px-4 py-3 font-mono text-[10px] leading-relaxed text-muted-foreground">
                {r.details}
              </pre>
            )}
          </div>
        );
      })}
    </section>
  );
}

// ---------------------------------------------------------------------------

function Stat({ label, value, tone }: { label: string; value: string | number; tone: string }) {
  return (
    <div className="bg-surface/40 px-4 py-3.5">
      <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      <div className={`mt-1 font-mono text-2xl font-semibold ${tone}`}>{value}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------

function SiteRow({
  target,
  rollup,
  globalPaused,
}: {
  target: ScrapeTask;
  rollup?: TaskRollup;
  /** True when the global Pause All kill-switch is on; per-row Restart
   *  must be disabled so the UI matches the backend (which rejects the
   *  request with 409). Per-row Pause/Resume remain available so users
   *  can still curate the active set while globally paused. */
  globalPaused: boolean;
}) {
  const run = useRunTarget();
  const pause = usePauseTarget();
  const resume = useResumeTarget();

  const company = prettyCompanyName(target.value);
  const host = prettyHostname(target.value);

  // Effective status: if the user has paused this target, override the
  // last-run status so the matrix shows it as paused regardless of the
  // last scrape outcome.
  const effective = !target.isActive ? ("paused" as const) : target.status;
  const meta =
    effective === "paused"
      ? {
          Icon: Pause,
          color: "text-muted-foreground",
          border: "border-border",
          bg: "bg-surface",
          label: "PAUSED",
        }
      : STATUS[effective];

  const Icon = meta.Icon;
  const isRunning = target.status === "running";
  const busy = run.isPending || pause.isPending || resume.isPending || isRunning;

  return (
    <div className="grid grid-cols-[1fr_110px_130px_80px_110px_220px] items-center gap-3 border-b border-border px-4 py-3 transition-colors last:border-b-0 hover:bg-indigo/[0.04]">
      {/* Pipeline column */}
      <div className="flex min-w-0 items-center gap-2.5">
        <span
          className={cn(
            "grid h-7 w-7 shrink-0 place-items-center rounded border",
            meta.border,
            meta.bg,
            meta.color,
          )}
        >
          <Icon className={cn("h-3.5 w-3.5", isRunning && "animate-spin")} />
        </span>
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">{company}</div>
          <a
            href={target.value.startsWith("http") ? target.value : `https://${target.value}`}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-block max-w-full truncate font-mono text-[10px] text-muted-foreground hover:text-indigo"
          >
            {host}
            {target.lastError && (
              <span className="ml-1.5 text-destructive">· {target.lastError}</span>
            )}
          </a>
        </div>
      </div>

      {/* Status badge */}
      <span
        className={cn(
          "inline-flex w-fit items-center justify-center rounded border px-2 py-0.5 font-mono text-[10px] tracking-wider",
          meta.border,
          meta.color,
        )}
      >
        {meta.label}
      </span>

      {/* Last run */}
      <span className="font-mono text-xs text-muted-foreground">
        {target.lastRunAt ? timeAgo(target.lastRunAt) : "—"}
      </span>

      {/* Records */}
      <span className="font-mono text-xs">{target.resultCount}</span>

      {/* Error rate (failed / total over the last 24h, from scraper_metrics) */}
      <ErrRateCell rollup={rollup} />

      {/* Actions */}
      <div className="flex justify-end gap-1.5">
        <ActionButton
          icon={busy && run.isPending ? Loader2 : RotateCw}
          spin={busy && run.isPending}
          label="Restart"
          onClick={() => run.mutate(target.id)}
          // Disable when globally paused so the UI stops looking like a
          // local override; the backend would reject anyway with 409.
          disabled={!target.isActive || busy || globalPaused}
          title={
            globalPaused
              ? "Scraping is globally paused — click START ALL up top to resume"
              : !target.isActive
                ? "This target is paused — Resume it first"
                : busy
                  ? "Scraper is already running"
                  : "Run this target now"
          }
          tone="indigo"
        />
        {target.isActive ? (
          <ActionButton
            icon={pause.isPending ? Loader2 : Pause}
            spin={pause.isPending}
            label="Pause"
            onClick={() => pause.mutate(target.id)}
            disabled={busy}
            tone="muted"
          />
        ) : (
          <ActionButton
            icon={resume.isPending ? Loader2 : Play}
            spin={resume.isPending}
            label="Resume"
            onClick={() => resume.mutate(target.id)}
            disabled={busy}
            tone="lime"
          />
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------

// ErrRateCell renders the failed-runs percentage for a single pipeline.
// Green if 0%, yellow below 20%, red above. A plain em-dash when we have
// no telemetry for this target yet.
function ErrRateCell({ rollup }: { rollup?: TaskRollup }) {
  if (!rollup || rollup.total === 0) {
    return <span className="font-mono text-xs text-muted-foreground">—</span>;
  }
  const pct = rollup.errRate;
  const tone = pct === 0 ? "text-lime" : pct < 20 ? "text-[#eab308]" : "text-destructive";
  return (
    <div className="flex flex-col leading-tight">
      <span className={cn("font-mono text-xs", tone)}>{pct.toFixed(1)}%</span>
      <span
        className="font-mono text-[10px] text-muted-foreground"
        title={`${rollup.failure} failed · ${rollup.rejected} filtered · ${rollup.total} total`}
      >
        {rollup.failure}/{rollup.total}
      </span>
    </div>
  );
}

interface ActionButtonProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  spin?: boolean;
  tone: "indigo" | "lime" | "muted";
  /** Native tooltip text — used to explain why a button is disabled
   *  (e.g. "Scraping is globally paused"). */
  title?: string;
}
function ActionButton({
  icon: Icon,
  label,
  onClick,
  disabled,
  spin,
  tone,
  title,
}: ActionButtonProps) {
  const palette =
    tone === "indigo"
      ? "border-border text-muted-foreground hover:text-indigo hover:border-indigo/40"
      : tone === "lime"
        ? "border-lime/40 text-lime hover:bg-lime/15"
        : "border-border text-muted-foreground hover:text-foreground hover:border-foreground/40";
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={cn(
        "inline-flex items-center gap-1 rounded border px-2 py-1 font-mono text-[10px] uppercase tracking-wider transition-colors",
        palette,
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      <Icon className={cn("h-3 w-3", spin && "animate-spin")} />
      {label}
    </button>
  );
}
