import { createFileRoute } from "@tanstack/react-router";
import { DashboardShell } from "@/components/dashboard-shell";
import { useState } from "react";
import { CheckCircle2, Loader2, XCircle, AlertTriangle, FlaskConical } from "lucide-react";
import { useDryRunScraper, useNiches, type DryRunResponse } from "@/lib/api";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/admin/scrapers")({
  head: () => ({ meta: [{ title: "Scout · Admin · Dry-Run Scraper" }] }),
  component: AdminScrapersPage,
});

function AdminScrapersPage() {
  return <DashboardShell showPulse={false}>{() => <AdminScrapersBody />}</DashboardShell>;
}

function AdminScrapersBody() {
  const { data: niches = [] } = useNiches();
  const dry = useDryRunScraper();

  const [url, setUrl] = useState("https://realpython.github.io/fake-jobs/");
  const [seedKeywords, setSeedKeywords] = useState("");
  const [nicheId, setNicheId] = useState<string>("");

  const result = dry.data;
  const isError = !!dry.error || result?.status === "failed";

  return (
    <div className="p-3 sm:p-4 space-y-4">
      {/* Header */}
      <div>
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground mb-1">
          /admin/scrapers
        </div>
        <h1 className="text-xl sm:text-2xl font-semibold tracking-tight inline-flex items-center gap-2">
          <FlaskConical className="h-5 w-5 text-indigo" />
          Dry-Run Scraper
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Run a target end-to-end <em>without persisting anything</em>. Use this to validate a
          niche's keyword filter, debug selectors on a new site, or confirm a stuck scraper actually
          works after a kill-restart.
        </p>
      </div>

      {/* Form */}
      <section className="rounded-md border border-border bg-surface/30 p-4 space-y-3">
        <div className="grid grid-cols-1 lg:grid-cols-[2fr_1fr] gap-3">
          <div>
            <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              Target URL
            </label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/jobs"
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus:outline-none focus:ring-1 focus:ring-indigo/60 focus:border-indigo/60"
            />
          </div>
          <div>
            <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              Niche
            </label>
            <select
              value={nicheId}
              onChange={(e) => setNicheId(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus:outline-none focus:ring-1 focus:ring-indigo/60 focus:border-indigo/60"
            >
              <option value="">— select niche —</option>
              {niches.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div>
          <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            Seed keywords (optional, comma-separated)
          </label>
          <input
            type="text"
            value={seedKeywords}
            onChange={(e) => setSeedKeywords(e.target.value)}
            placeholder="python, react, postgres"
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus:outline-none focus:ring-1 focus:ring-indigo/60 focus:border-indigo/60"
          />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            disabled={dry.isPending || !nicheId || (!url && !seedKeywords)}
            onClick={() =>
              dry.mutate({
                url: url || undefined,
                nicheId,
                seedKeywords: seedKeywords || undefined,
              })
            }
            className="inline-flex items-center gap-1.5 rounded-md border border-indigo/50 bg-indigo/10 px-3 py-1.5 font-mono text-[11px] uppercase tracking-wider text-indigo hover:bg-indigo/20 transition-colors disabled:opacity-50"
          >
            {dry.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <FlaskConical className="h-3.5 w-3.5" />
            )}
            Run dry test
          </button>
          {result && (
            <span className="font-mono text-[10px] text-muted-foreground">
              {result.durationMs}ms · exit {result.exitCode} · niche {result.nicheName ?? "?"}
            </span>
          )}
        </div>
      </section>

      {/* Result */}
      {dry.error && !result && (
        <ResultBanner tone="error" title="Request failed" subtitle={dry.error.message} />
      )}

      {result && <DryRunResult result={result} isError={isError} />}
    </div>
  );
}

function DryRunResult({ result, isError }: { result: DryRunResponse; isError: boolean }) {
  const passed = result.passed ?? [];
  const failed = result.failed ?? [];
  return (
    <div className="space-y-4">
      <ResultBanner
        tone={isError ? "error" : passed.length > 0 ? "ok" : "warn"}
        title={
          isError
            ? "Scraper run failed"
            : `Found ${result.totalJobs} jobs · ${passed.length} would persist · ${failed.length} would be rejected`
        }
        subtitle={
          isError
            ? "Stderr captured below for debugging."
            : passed.length === 0
              ? "Every candidate failed the niche-context filter — adjust thresholds or seed keywords."
              : "Validation matches what the live ingest pipeline would do. No DB writes occurred."
        }
      />

      {/* Passed list */}
      {passed.length > 0 && (
        <Section title="Would persist" tone="ok">
          {passed.map((p) => (
            <ResultRow
              key={p.url + p.title}
              title={p.title}
              company={p.company}
              url={p.url}
              right={
                <span className="font-mono text-[10px] text-lime">
                  {p.matchScore}/{p.threshold} keywords
                </span>
              }
            />
          ))}
        </Section>
      )}

      {/* Failed list */}
      {failed.length > 0 && (
        <Section title="Would reject" tone="warn">
          {failed.map((f) => (
            <ResultRow
              key={f.url + f.title}
              title={f.title}
              company={f.company}
              url={f.url}
              right={
                <div className="text-right">
                  <div className="font-mono text-[10px] text-destructive">
                    {f.hits}/{f.threshold} keywords
                  </div>
                  <div className="font-mono text-[9px] text-muted-foreground max-w-[200px] truncate">
                    missing: {f.missing.join(", ")}
                  </div>
                </div>
              }
            />
          ))}
        </Section>
      )}

      {/* Stderr (always shown for power-users) */}
      {result.stderr && (
        <Section title="stderr" tone="muted" defaultCollapsed={!isError}>
          <pre className="m-0 max-h-[400px] overflow-auto bg-background/50 p-3 font-mono text-[10px] leading-relaxed text-muted-foreground whitespace-pre-wrap break-words">
            {result.stderr}
          </pre>
        </Section>
      )}
    </div>
  );
}

function ResultBanner({
  tone,
  title,
  subtitle,
}: {
  tone: "ok" | "warn" | "error";
  title: string;
  subtitle?: string;
}) {
  const palette =
    tone === "ok"
      ? { Icon: CheckCircle2, color: "text-lime", border: "border-lime/40", bg: "bg-lime/10" }
      : tone === "warn"
        ? {
            Icon: AlertTriangle,
            color: "text-[#eab308]",
            border: "border-[#eab308]/40",
            bg: "bg-[#eab308]/10",
          }
        : {
            Icon: XCircle,
            color: "text-destructive",
            border: "border-destructive/40",
            bg: "bg-destructive/10",
          };
  const Icon = palette.Icon;
  return (
    <div className={cn("flex items-start gap-3 rounded-md border p-3", palette.border, palette.bg)}>
      <Icon className={cn("h-4 w-4 mt-0.5 shrink-0", palette.color)} />
      <div className="min-w-0">
        <div className={cn("text-sm font-semibold", palette.color)}>{title}</div>
        {subtitle && <div className="text-xs text-muted-foreground mt-0.5">{subtitle}</div>}
      </div>
    </div>
  );
}

function Section({
  title,
  tone,
  children,
  defaultCollapsed,
}: {
  title: string;
  tone: "ok" | "warn" | "muted";
  children: React.ReactNode;
  defaultCollapsed?: boolean;
}) {
  const [open, setOpen] = useState(!defaultCollapsed);
  const accent =
    tone === "ok"
      ? "text-lime border-lime/30"
      : tone === "warn"
        ? "text-destructive border-destructive/30"
        : "text-muted-foreground border-border";
  return (
    <section className="rounded-md border border-border bg-surface/20 overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "w-full flex items-center justify-between border-b px-4 py-2 font-mono text-[10px] uppercase tracking-wider",
          accent,
        )}
      >
        <span>{title}</span>
        <span className="text-muted-foreground">{open ? "▾" : "▸"}</span>
      </button>
      {open && <div>{children}</div>}
    </section>
  );
}

function ResultRow({
  title,
  company,
  url,
  right,
}: {
  title: string;
  company: string;
  url: string;
  right: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[1fr_auto] items-center gap-3 px-4 py-2.5 border-b border-border/60 last:border-b-0">
      <div className="min-w-0">
        <div className="text-sm font-medium truncate">{title}</div>
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="font-mono text-[10px] text-muted-foreground hover:text-indigo truncate inline-block max-w-full"
        >
          {company} · {url}
        </a>
      </div>
      <div className="shrink-0">{right}</div>
    </div>
  );
}
