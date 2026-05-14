import { useMemo, useState } from "react";
import type { Job } from "@/lib/jobs-data";
import { JobRow, JobRowSkeleton } from "./job-row";
import { ArrowDownUp, Filter, RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { useSaveToggle } from "@/lib/api";

type SortKey = "match" | "deadline" | "recent";

const SORT_LABEL: Record<SortKey, string> = {
  match: "match score",
  deadline: "deadline",
  recent: "recently posted",
};

interface Props {
  jobs: Job[];
  query: string;
  emptyLabel?: string;
  /** Set by the parent when an API request is in-flight; drives the skeleton. */
  loading?: boolean;
  /** Manual refetch trigger for the "refresh" button. */
  onRefresh?: () => void;
}

export function JobMatrix({
  jobs,
  query,
  emptyLabel = "No jobs match your query",
  loading = false,
  onRefresh,
}: Props) {
  const [sort, setSort] = useState<SortKey>("match");
  const [sourceFilter, setSourceFilter] = useState<string | "all">("all");
  const { isSaved, toggle } = useSaveToggle();

  // Text search (`query`) is performed server-side; here we only apply the
  // client-side `source` filter and the chosen sort order.
  const filtered = useMemo(() => {
    let arr = jobs;
    if (sourceFilter !== "all") {
      arr = arr.filter((j) => j.sources?.includes(sourceFilter as Job["sources"][number]));
    }
    arr = arr.slice().sort((a, b) => {
      if (sort === "match") return b.matchScore - a.matchScore;
      if (sort === "deadline") {
        // Sort soonest-first; jobs with no deadline land at the bottom so
        // they don't pollute the top of the list.
        const at = a.deadline ? new Date(a.deadline).getTime() : Number.POSITIVE_INFINITY;
        const bt = b.deadline ? new Date(b.deadline).getTime() : Number.POSITIVE_INFINITY;
        return at - bt;
      }
      return new Date(b.postedAt).getTime() - new Date(a.postedAt).getTime();
    });
    return arr;
  }, [jobs, sourceFilter, sort]);

  // Keep `query` referenced for callers that still pass it; the empty-state
  // message implicitly reflects what the user typed.
  void query;

  const refresh = () => onRefresh?.();

  const sources: ("all" | string)[] = [
    "all",
    "linkedin",
    "bdjobs",
    "glassdoor",
    "indeed",
    "github",
  ];

  return (
    <section className="border-x border-b border-border bg-surface/20">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 px-3 sm:px-4 py-2.5 border-b border-border bg-background/50">
        <div className="flex items-center gap-1.5 mr-2">
          <Filter className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            source
          </span>
        </div>
        <div className="flex items-center gap-1 flex-wrap">
          {sources.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setSourceFilter(s)}
              className={cn(
                "rounded border px-2 py-1 font-mono text-[10px] uppercase tracking-wider transition-colors",
                sourceFilter === s
                  ? "border-indigo bg-indigo/10 text-indigo"
                  : "border-border text-muted-foreground hover:text-foreground hover:border-foreground/40",
              )}
            >
              {s}
            </button>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            onClick={() => {
              const order: SortKey[] = ["match", "deadline", "recent"];
              setSort(order[(order.indexOf(sort) + 1) % order.length]);
            }}
            className="inline-flex items-center gap-1.5 rounded border border-border px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground hover:text-foreground hover:border-foreground/40 transition-colors"
          >
            <ArrowDownUp className="h-3 w-3" />
            sort: {SORT_LABEL[sort]}
          </button>
          <button
            type="button"
            onClick={refresh}
            className="inline-flex items-center gap-1.5 rounded border border-border px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground hover:text-lime hover:border-lime/40 transition-colors"
          >
            <RefreshCw className={cn("h-3 w-3", loading && "animate-spin")} />
            refresh
          </button>
        </div>
      </div>

      {/* Header row */}
      <div className="grid grid-cols-[64px_1fr_auto] gap-4 px-4 py-2 border-b border-border bg-background/30 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        <span>score</span>
        <span>position · company · meta</span>
        <span className="text-right">sources · deadline · save</span>
      </div>

      {/* Rows */}
      <div>
        {loading ? (
          Array.from({ length: 5 }).map((_, i) => <JobRowSkeleton key={i} />)
        ) : filtered.length === 0 ? (
          <div className="px-4 py-16 text-center">
            <p className="font-mono text-xs text-muted-foreground">// {emptyLabel}</p>
          </div>
        ) : (
          filtered.map((j) => {
            const key = j.jobUuid ?? j.id;
            return (
              <JobRow
                key={key}
                job={j}
                saved={isSaved(key)}
                onToggleSave={() => toggle(key, j.id)}
              />
            );
          })
        )}
      </div>

      <div className="px-4 py-2.5 font-mono text-[10px] text-muted-foreground border-t border-border bg-background/30 flex items-center justify-between">
        <span>{loading ? "// indexing..." : `${filtered.length} of ${jobs.length} positions`}</span>
        <span>last sync · just now</span>
      </div>
    </section>
  );
}
