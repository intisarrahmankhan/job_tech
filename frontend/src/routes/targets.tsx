import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  CheckCircle2,
  Clock,
  Crosshair,
  Link2,
  Loader2,
  Plus,
  RefreshCw,
  Trash2,
  XCircle,
} from "lucide-react";
import { DashboardShell } from "@/components/dashboard-shell";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useCreateTarget,
  useDeleteTarget,
  useNiches,
  useTargets,
  type ScrapeTask,
  type ScrapeTaskStatus,
  type ScrapeTaskType,
} from "@/lib/api";
import { timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/targets")({
  head: () => ({
    meta: [
      { title: "Scout · Targeting Dashboard" },
      {
        name: "description",
        content: "Manage user-defined keyword and direct-URL scrape targets.",
      },
    ],
  }),
  component: TargetsPage,
});

function TargetsPage() {
  return <DashboardShell showPulse>{() => <TargetsBody />}</DashboardShell>;
}

function TargetsBody() {
  const { data: targets = [], isLoading, refetch, isFetching } = useTargets();

  const keywordTargets = targets.filter((t) => t.type === "KEYWORD");
  const urlTargets = targets.filter((t) => t.type === "DIRECT_URL");

  return (
    <div className="p-3 sm:p-4 space-y-4">
      <PageHeader
        count={targets.length}
        loading={isLoading || isFetching}
        onRefresh={() => refetch()}
      />

      <Tabs defaultValue="keywords" className="w-full">
        <TabsList className="bg-surface/40 border border-border">
          <TabsTrigger value="keywords" className="font-mono text-[11px] uppercase tracking-wider">
            <Crosshair className="h-3.5 w-3.5 mr-1.5" />
            Keywords <span className="ml-1.5 text-muted-foreground">· {keywordTargets.length}</span>
          </TabsTrigger>
          <TabsTrigger value="urls" className="font-mono text-[11px] uppercase tracking-wider">
            <Link2 className="h-3.5 w-3.5 mr-1.5" />
            Custom Links <span className="ml-1.5 text-muted-foreground">· {urlTargets.length}</span>
          </TabsTrigger>
        </TabsList>

        <TabsContent value="keywords" className="mt-4">
          <AddKeywordForm />
          <TargetList
            targets={keywordTargets}
            loading={isLoading}
            emptyLabel="// no keywords yet — add a role to start scraping"
          />
        </TabsContent>

        <TabsContent value="urls" className="mt-4">
          <AddUrlForm />
          <TargetList
            targets={urlTargets}
            loading={isLoading}
            emptyLabel="// no custom links yet — paste a career-page URL"
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function PageHeader({
  count,
  loading,
  onRefresh,
}: {
  count: number;
  loading: boolean;
  onRefresh: () => void;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground mb-1">
          /targeting
        </div>
        <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">Targeting Dashboard</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {count} target{count === 1 ? "" : "s"} configured · keywords expand search, links scrape
          one page
        </p>
      </div>
      <button
        type="button"
        onClick={onRefresh}
        className="inline-flex items-center gap-1.5 rounded border border-border px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground hover:text-foreground hover:border-foreground/40 transition-colors"
      >
        <RefreshCw className={cn("h-3 w-3", loading && "animate-spin")} />
        sync
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Add forms

function AddKeywordForm() {
  const [value, setValue] = useState("");
  const [nicheId, setNicheId] = useState<string>("");
  const create = useCreateTarget();
  const submit = async () => {
    const v = value.trim();
    if (!v) return;
    try {
      await create.mutateAsync({ type: "KEYWORD", value: v, nicheId: nicheId || undefined });
      setValue("");
    } catch {
      /* error surfaced via create.error */
    }
  };

  return (
    <FormShell
      placeholder='e.g. "SQA Engineer", "Flutter", "DevOps"'
      value={value}
      onChange={setValue}
      onSubmit={submit}
      pending={create.isPending}
      error={create.error?.message}
      type="KEYWORD"
      nicheId={nicheId}
      onNicheChange={setNicheId}
    />
  );
}

function AddUrlForm() {
  const [value, setValue] = useState("");
  const [nicheId, setNicheId] = useState<string>("");
  const [localError, setLocalError] = useState<string | null>(null);
  const create = useCreateTarget();

  const submit = async () => {
    const v = value.trim();
    setLocalError(null);
    if (!isValidHttpUrl(v)) {
      setLocalError("Enter a valid http(s):// URL.");
      return;
    }
    try {
      await create.mutateAsync({ type: "DIRECT_URL", value: v, nicheId: nicheId || undefined });
      setValue("");
    } catch {
      /* error surfaced via create.error */
    }
  };

  return (
    <FormShell
      placeholder="https://careers.example.com/jobs/123"
      value={value}
      onChange={(v) => {
        setValue(v);
        if (localError) setLocalError(null);
      }}
      onSubmit={submit}
      pending={create.isPending}
      error={localError ?? create.error?.message}
      type="DIRECT_URL"
      nicheId={nicheId}
      onNicheChange={setNicheId}
    />
  );
}

function FormShell({
  value,
  onChange,
  onSubmit,
  placeholder,
  pending,
  error,
  type,
  nicheId,
  onNicheChange,
}: {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  placeholder: string;
  pending: boolean;
  error?: string | null;
  type: ScrapeTaskType;
  nicheId: string;
  onNicheChange: (id: string) => void;
}) {
  const { data: niches = [] } = useNiches();
  return (
    <div className="rounded-md border border-border bg-surface/30 p-3">
      <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground mb-2">
        {type === "KEYWORD" ? "// add new keyword" : "// add new URL"}
      </div>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit();
        }}
      >
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          spellCheck={false}
          autoComplete="off"
          inputMode={type === "DIRECT_URL" ? "url" : "text"}
          className={cn(
            "flex-1 min-w-[260px] rounded-md border bg-background/60 px-3 h-9 font-mono text-sm outline-none transition-colors",
            error
              ? "border-destructive/60 focus:border-destructive"
              : "border-border focus:border-indigo",
          )}
        />
        <select
          value={nicheId}
          onChange={(e) => onNicheChange(e.target.value)}
          className="rounded-md border border-border bg-background/60 px-2 h-9 font-mono text-[11px] text-muted-foreground focus:outline-none focus:border-indigo"
          title="Bind this target to a niche so its context filter applies"
        >
          <option value="">No niche</option>
          {niches.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </select>
        <button
          type="submit"
          disabled={pending || !value.trim()}
          className={cn(
            "inline-flex items-center gap-1.5 rounded-md border px-3 h-9 font-mono text-[11px] uppercase tracking-wider transition-colors",
            pending || !value.trim()
              ? "border-border bg-surface text-muted-foreground cursor-not-allowed"
              : "border-indigo/50 bg-indigo/10 text-indigo hover:bg-indigo/20",
          )}
        >
          {pending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Plus className="h-3.5 w-3.5" />
          )}
          Add
        </button>
      </form>
      {error && <p className="mt-2 font-mono text-[11px] text-destructive">{`! ${error}`}</p>}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Target list + status

function TargetList({
  targets,
  loading,
  emptyLabel,
}: {
  targets: ScrapeTask[];
  loading: boolean;
  emptyLabel: string;
}) {
  if (loading) {
    return (
      <ul className="mt-3 space-y-1.5">
        {Array.from({ length: 3 }).map((_, i) => (
          <li key={i} className="h-14 rounded-md border border-border skeleton" />
        ))}
      </ul>
    );
  }
  if (targets.length === 0) {
    return (
      <div className="mt-6 rounded-md border border-dashed border-border bg-surface/20 px-4 py-10 text-center">
        <p className="font-mono text-xs text-muted-foreground">{emptyLabel}</p>
      </div>
    );
  }
  return (
    <ul className="mt-3 space-y-1.5">
      {targets.map((t) => (
        <TargetRow key={t.id} target={t} />
      ))}
    </ul>
  );
}

function TargetRow({ target }: { target: ScrapeTask }) {
  const del = useDeleteTarget();
  const Icon = target.type === "KEYWORD" ? Crosshair : Link2;

  return (
    <li className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-surface/30 px-3 py-2.5 hover:bg-surface/50 transition-colors">
      <span className="grid h-7 w-7 shrink-0 place-items-center rounded border border-border bg-background text-indigo">
        <Icon className="h-3.5 w-3.5" />
      </span>

      <div className="min-w-0 flex-1">
        <div className="truncate text-sm text-foreground">{target.value}</div>
        <div className="flex flex-wrap items-center gap-2 mt-0.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          <span>{target.type === "KEYWORD" ? "keyword" : "url"}</span>
          <span>·</span>
          <span>every {target.frequency}</span>
          <span>·</span>
          <span>{target.resultCount} found</span>
          {target.lastRunAt && (
            <>
              <span>·</span>
              <span>last scraped {timeAgo(target.lastRunAt)}</span>
            </>
          )}
        </div>
      </div>

      <StatusBadge status={target.status} />

      <button
        type="button"
        onClick={() => del.mutate(target.id)}
        title="Remove target"
        className="grid h-8 w-8 place-items-center rounded border border-border text-muted-foreground hover:text-destructive hover:border-destructive/50 transition-colors"
      >
        {del.isPending ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Trash2 className="h-3.5 w-3.5" />
        )}
      </button>
    </li>
  );
}

function StatusBadge({ status }: { status: ScrapeTaskStatus }) {
  const map: Record<ScrapeTaskStatus, { Icon: typeof CheckCircle2; cls: string; label: string }> = {
    pending: { Icon: Clock, cls: "border-border text-muted-foreground", label: "pending" },
    running: { Icon: Loader2, cls: "border-indigo/50 bg-indigo/10 text-indigo", label: "running" },
    healthy: { Icon: CheckCircle2, cls: "border-lime/50 bg-lime/10 text-lime", label: "healthy" },
    failed: {
      Icon: XCircle,
      cls: "border-destructive/50 bg-destructive/10 text-destructive",
      label: "failed",
    },
  };
  const { Icon, cls, label } = map[status];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[10px] uppercase tracking-wider",
        cls,
      )}
    >
      <Icon className={cn("h-3 w-3", status === "running" && "animate-spin")} />
      {label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Utilities

function isValidHttpUrl(raw: string): boolean {
  try {
    const u = new URL(raw);
    return (u.protocol === "http:" || u.protocol === "https:") && !!u.host;
  } catch {
    return false;
  }
}
