import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { DashboardShell } from "@/components/dashboard-shell";
import { JobMatrix } from "@/components/job-matrix";
import { useDebouncedValue, useJobs, useNiches } from "@/lib/api";
import { triggerScraperRefresh } from "@/lib/pulse-socket";
import type { Job } from "@/lib/jobs-data";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/")({
  head: () => ({
    meta: [
      { title: "Scout · Global Job Feed" },
      {
        name: "description",
        content:
          "Aggregated tech job feed across LinkedIn, BDJobs, Glassdoor, Indeed and GitHub for Bangladesh.",
      },
    ],
  }),
  component: GlobalFeed,
});

function GlobalFeed() {
  return <DashboardShell>{({ query }) => <GlobalFeedBody query={query} />}</DashboardShell>;
}

function GlobalFeedBody({ query }: { query: string }) {
  // Debounce 300ms — the API isn't called until the user pauses typing.
  const debouncedQuery = useDebouncedValue(query, 300);
  const [nicheId, setNicheId] = useState<string | undefined>(undefined);
  const {
    data: jobs = [],
    isLoading,
    isFetching,
    isError,
    refetch,
  } = useJobs({ q: debouncedQuery, nicheId });

  return (
    <div className="p-3 sm:p-4 space-y-4">
      <SectionHeader jobs={jobs} isLoading={isLoading} isError={isError} query={debouncedQuery} />
      <NicheFilterPills value={nicheId} onChange={setNicheId} />
      <BentoStats jobs={jobs} />
      <JobMatrix
        jobs={jobs}
        query={debouncedQuery}
        loading={isLoading || isFetching}
        onRefresh={async () => {
          // Fire-and-forget scraper trigger; refetch jobs after a short
          // pause so any newly merged rows show up. The pulse sidebar will
          // surface the live progress events.
          await triggerScraperRefresh().catch(() => {});
          setTimeout(() => refetch(), 1500);
        }}
        emptyLabel={
          debouncedQuery ? `No jobs match "${debouncedQuery}"` : "No jobs match your query"
        }
      />
    </div>
  );
}

function SectionHeader({
  jobs,
  isLoading,
  isError,
  query,
}: {
  jobs: Job[];
  isLoading: boolean;
  isError: boolean;
  query: string;
}) {
  const subtitle = isLoading
    ? "loading live feed…"
    : isError
      ? "backend offline — retrying"
      : query
        ? `${jobs.length} match${jobs.length === 1 ? "" : "es"} for "${query}"`
        : `${jobs.length} aggregated positions · 5 sources · merged & deduplicated`;
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground mb-1">
          /global-feed
        </div>
        <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">
          Job Matrix · Bangladesh Tech
        </h1>
        <p className="text-sm text-muted-foreground mt-1">{subtitle}</p>
      </div>
      <div className="font-mono text-[11px] text-muted-foreground">{new Date().toUTCString()}</div>
    </div>
  );
}

function NicheFilterPills({
  value,
  onChange,
}: {
  value: string | undefined;
  onChange: (id: string | undefined) => void;
}) {
  const { data: niches = [] } = useNiches();
  if (niches.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        Niche:
      </span>
      <NichePill label="All" active={!value} onClick={() => onChange(undefined)} />
      {niches.map((n) => (
        <NichePill
          key={n.id}
          label={n.name}
          active={value === n.id}
          onClick={() => onChange(n.id)}
        />
      ))}
    </div>
  );
}

function NichePill({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-md border px-2.5 py-1 font-mono text-[10px] uppercase tracking-wider transition-colors",
        active
          ? "border-indigo/60 bg-indigo/15 text-indigo"
          : "border-border text-muted-foreground hover:text-foreground hover:border-foreground/40",
      )}
    >
      {label}
    </button>
  );
}

function BentoStats({ jobs }: { jobs: Job[] }) {
  const avgMatch = jobs.length
    ? Math.round(jobs.reduce((a, j) => a + (j.matchScore ?? 0), 0) / jobs.length)
    : 0;
  const cells = [
    { l: "Active Roles", v: jobs.length, sub: "live feed", tone: "text-foreground" },
    { l: "Avg Match", v: avgMatch + "%", sub: "vs profile", tone: "text-lime" },
    {
      l: "Closing < 3d",
      // Only count jobs with an actual deadline that's still in the future
      // and within the next 3 days. Jobs without a scraped deadline don't
      // belong in this metric.
      v: jobs.filter((j) => {
        if (!j.deadline) return false;
        const diff = new Date(j.deadline).getTime() - Date.now();
        return diff > 0 && diff < 3 * 86400_000;
      }).length,
      sub: "urgent",
      tone: "text-indigo",
    },
    {
      l: "Merged Dupes",
      v: jobs.filter((j) => (j.merged?.length ?? 0) > 1).length,
      sub: "cross-source",
      tone: "text-foreground",
    },
  ];
  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-px bg-border rounded-md overflow-hidden border border-border">
      {cells.map((c) => (
        <div key={c.l} className="bg-surface/40 px-4 py-3.5">
          <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            {c.l}
          </div>
          <div className={`mt-1 font-mono text-2xl font-semibold ${c.tone}`}>{c.v}</div>
          <div className="font-mono text-[10px] text-muted-foreground mt-0.5">{c.sub}</div>
        </div>
      ))}
    </div>
  );
}
