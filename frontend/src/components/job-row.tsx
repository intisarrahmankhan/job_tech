import { useMemo, useState } from "react";
import {
  Bookmark,
  BookmarkCheck,
  ChevronDown,
  ExternalLink,
  GitMerge,
  MapPin,
  Send,
  Timer,
  X,
} from "lucide-react";
import type { Job, JobSource } from "@/lib/jobs-data";
import { countdown, timeAgo } from "@/lib/format";
import { SourceBadge } from "./source-badge";
import { cn } from "@/lib/utils";

// Some merged URLs come back without a protocol (e.g. "linkedin.com/jobs/7831"
// in the demo data); the browser would treat those as relative to the current
// origin. ensureAbsoluteURL forces an https:// prefix when one is missing so
// the "Apply" button and source icons always open the real posting.
function ensureAbsoluteURL(raw: string | undefined): string | undefined {
  if (!raw) return undefined;
  const trimmed = raw.trim();
  if (!trimmed) return undefined;
  if (/^https?:\/\//i.test(trimmed)) return trimmed;
  return `https://${trimmed.replace(/^\/+/, "")}`;
}

interface JobRowProps {
  job: Job;
  saved: boolean;
  onToggleSave: () => void;
}

function scoreTone(s: number) {
  if (s >= 90) return { ring: "border-lime/60 text-lime", bar: "bg-lime" };
  if (s >= 75) return { ring: "border-indigo/60 text-indigo", bar: "bg-indigo" };
  return { ring: "border-border text-muted-foreground", bar: "bg-muted-foreground" };
}

export function JobRow({ job, saved, onToggleSave }: JobRowProps) {
  const [open, setOpen] = useState(false);
  const tone = scoreTone(job.matchScore);
  const cd = countdown(job.deadline);
  // Prefer the authoritative server-side merge_count when present; fall
  // back to the length of the merged[] array for the static demo data.
  const mergeBadgeCount =
    typeof job.mergeCount === "number" && job.mergeCount > 0
      ? job.mergeCount + 1
      : (job.merged?.length ?? 0);
  const merged = mergeBadgeCount > 1;

  // Map each source → canonical posting URL. We look at the merged[] entries
  // first (each carries its own URL), then fall back to the top-level
  // job.url for sources without a dedicated entry.
  const sourceUrls = useMemo<Record<string, string | undefined>>(() => {
    const map: Record<string, string | undefined> = {};
    for (const m of job.merged ?? []) {
      if (m.source && m.url && !map[m.source]) map[m.source] = ensureAbsoluteURL(m.url);
    }
    const fallback = ensureAbsoluteURL(job.url);
    for (const s of job.sources ?? []) {
      if (!map[s]) map[s] = fallback;
    }
    return map;
  }, [job.merged, job.sources, job.url]);

  // The Apply button uses the first available URL we know about.
  const applyURL =
    ensureAbsoluteURL(job.url) ?? ensureAbsoluteURL(job.merged?.[0]?.url) ?? undefined;

  return (
    <div
      className={cn(
        "group relative grid grid-cols-[64px_1fr_auto] gap-3 sm:gap-4 px-3 sm:px-4 py-3.5 border-b border-border transition-colors",
        "hover:bg-indigo/[0.04] hover:backdrop-blur-sm",
      )}
    >
      {/* Match score */}
      <div className="flex flex-col items-start gap-1.5">
        <div
          className={cn(
            "relative grid h-12 w-12 place-items-center rounded-md border bg-background/60 font-mono text-sm font-semibold",
            tone.ring,
          )}
        >
          {job.matchScore}
          <span className="absolute -bottom-1 left-1.5 right-1.5 h-[2px] rounded-full bg-border overflow-hidden">
            <span
              className={cn("block h-full", tone.bar)}
              style={{ width: `${job.matchScore}%` }}
            />
          </span>
        </div>
        <span className="font-mono text-[9px] uppercase tracking-wider text-muted-foreground">
          match
        </span>
      </div>

      {/* Title block */}
      <div className="min-w-0">
        <div className="flex items-start gap-2 flex-wrap">
          {applyURL ? (
            <a
              href={applyURL}
              target="_blank"
              rel="noopener noreferrer"
              title="Open job posting"
              className="text-[15px] font-semibold text-foreground leading-tight truncate hover:text-indigo hover:underline underline-offset-4 transition-colors"
            >
              {job.title}
            </a>
          ) : (
            <h3 className="text-[15px] font-semibold text-foreground leading-tight truncate">
              {job.title}
            </h3>
          )}
          <span className="font-mono text-[10px] text-muted-foreground border border-border rounded px-1.5 py-0.5 uppercase tracking-wider">
            {job.level}
          </span>
          <span className="font-mono text-[10px] text-muted-foreground border border-border rounded px-1.5 py-0.5 uppercase tracking-wider">
            {job.type}
          </span>
          {merged && (
            <button
              type="button"
              onClick={() => setOpen((v) => !v)}
              className={cn(
                "inline-flex items-center gap-1 rounded border border-indigo/50 bg-indigo/10 px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider text-indigo hover:bg-indigo/20 transition-colors",
                open && "bg-indigo/20",
              )}
              title="Merged duplicate sources"
            >
              <GitMerge className="h-3 w-3" />
              Merged ×{mergeBadgeCount}
              <ChevronDown className={cn("h-3 w-3 transition-transform", open && "rotate-180")} />
            </button>
          )}
        </div>

        <div className="mt-1.5 flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
          <span className="text-foreground/80 font-medium">{job.company}</span>
          <span className="inline-flex items-center gap-1">
            <MapPin className="h-3 w-3" />
            {job.location}
          </span>
          {job.salary && <span className="font-mono">{job.salary}</span>}
          <span className="font-mono text-[11px]">· {timeAgo(job.postedAt)}</span>
          <span className="font-mono text-[10px] text-muted-foreground/70">· {job.id}</span>
        </div>

        <div className="mt-2 flex items-center gap-1.5 flex-wrap">
          {(job.tags ?? []).slice(0, 4).map((t) => (
            <span
              key={t}
              className="rounded bg-surface px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground border border-border/60"
            >
              {t}
            </span>
          ))}
        </div>

        {open && merged && (
          <div className="mt-3 rounded-md border border-border bg-background/60 p-2.5 space-y-1.5">
            <div className="text-[10px] font-mono uppercase tracking-wider text-muted-foreground mb-1.5">
              Original sources · {job.merged!.length}
            </div>
            {job.merged!.map((m) => {
              const href = ensureAbsoluteURL(m.url);
              return (
                <div key={m.url} className="flex items-center justify-between gap-2 text-xs">
                  <div className="flex items-center gap-2 min-w-0">
                    <SourceBadge source={m.source} href={href} mini />
                    <span className="font-mono text-muted-foreground truncate">{m.url}</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <span className="font-mono text-[10px] text-muted-foreground">
                      {timeAgo(m.foundAt)}
                    </span>
                    {href ? (
                      <a
                        href={href}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="grid h-6 w-6 place-items-center rounded border border-border hover:border-indigo hover:text-indigo transition-colors"
                        title="Open this posting"
                      >
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    ) : (
                      <span className="grid h-6 w-6 place-items-center rounded border border-border text-muted-foreground/50">
                        <ExternalLink className="h-3 w-3" />
                      </span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Right rail */}
      <div className="flex flex-col items-end gap-2">
        <div className="flex items-center gap-1">
          {(job.sources ?? []).map((s) => (
            <SourceBadge key={s} source={s as JobSource} href={sourceUrls[s]} mini />
          ))}
        </div>
        <div
          className={cn(
            "inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[11px]",
            cd.expired
              ? "border-destructive/40 text-destructive"
              : cd.urgent
                ? "border-lime/40 text-lime"
                : cd.unknown
                  ? "border-border/60 text-muted-foreground/80"
                  : "border-border text-muted-foreground",
          )}
          title={
            cd.unknown
              ? "No application deadline scraped from the source page"
              : `Deadline: ${new Date(job.deadline as string).toLocaleString()}`
          }
        >
          <Timer className="h-3 w-3" />
          {cd.label}
        </div>
        <div className="flex items-center gap-1.5">
          {applyURL && (
            <a
              href={applyURL}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded-md border border-indigo/50 bg-indigo/10 px-2 py-1 text-[11px] font-mono text-indigo hover:bg-indigo/20 transition-colors"
              title="Open job posting in a new tab"
            >
              <Send className="h-3 w-3" />
              Apply
            </a>
          )}
          <button
            type="button"
            onClick={onToggleSave}
            title={saved ? "Click to unsave" : "Save this role"}
            className={cn(
              "group inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-mono transition-colors",
              saved
                ? "border-lime/40 bg-lime/10 text-lime hover:border-destructive/40 hover:bg-destructive/10 hover:text-destructive"
                : "border-border text-muted-foreground hover:text-foreground hover:border-foreground/40",
            )}
          >
            {saved ? (
              <>
                <BookmarkCheck className="h-3 w-3 group-hover:hidden" />
                <X className="hidden h-3 w-3 group-hover:inline" />
                <span className="group-hover:hidden">Saved</span>
                <span className="hidden group-hover:inline">Unsave</span>
              </>
            ) : (
              <>
                <Bookmark className="h-3 w-3" />
                Save
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

export function JobRowSkeleton() {
  return (
    <div className="grid grid-cols-[64px_1fr_auto] gap-4 px-4 py-3.5 border-b border-border">
      <div className="h-12 w-12 rounded-md skeleton" />
      <div className="space-y-2">
        <div className="h-4 w-2/3 rounded skeleton" />
        <div className="h-3 w-1/2 rounded skeleton" />
        <div className="flex gap-1.5">
          <div className="h-4 w-12 rounded skeleton" />
          <div className="h-4 w-12 rounded skeleton" />
          <div className="h-4 w-12 rounded skeleton" />
        </div>
      </div>
      <div className="space-y-2 items-end flex flex-col">
        <div className="h-5 w-20 rounded skeleton" />
        <div className="h-6 w-16 rounded skeleton" />
        <div className="h-6 w-14 rounded skeleton" />
      </div>
    </div>
  );
}
