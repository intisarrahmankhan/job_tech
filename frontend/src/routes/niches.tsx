import { createFileRoute } from "@tanstack/react-router";
import { DashboardShell } from "@/components/dashboard-shell";
import { AlertCircle, Layers, Loader2, Play, Plus, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import {
  useAddNicheSource,
  useCreateNiche,
  useDeleteNiche,
  useDeleteNicheSource,
  useNiches,
  useNicheSources,
  useRunNiche,
  useUpdateNiche,
  type NicheProfile,
} from "@/lib/api";
import { prettyCompanyName, prettyHostname } from "@/lib/pretty-name";

export const Route = createFileRoute("/niches")({
  head: () => ({ meta: [{ title: "Scout · Niche Manager" }] }),
  component: NichesPage,
});

function NichesPage() {
  return <DashboardShell>{() => <NichesBody />}</DashboardShell>;
}

function NichesBody() {
  const { data: niches = [], isLoading } = useNiches();
  const [activeId, setActiveId] = useState<string | null>(null);

  const active = useMemo(
    () => niches.find((n) => n.id === activeId) ?? niches[0] ?? null,
    [niches, activeId],
  );

  return (
    <div className="p-3 sm:p-4 space-y-4">
      {/* Header */}
      <div>
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground mb-1">
          /niche-manager
        </div>
        <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">Niche Manager</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Group seed keywords, context filters, and curated career pages by domain. Jobs that don't
          match a niche's context keywords are rejected before they hit your feed.
        </p>
      </div>

      <div className="grid lg:grid-cols-[280px_1fr] gap-4">
        <NicheList
          niches={niches}
          activeId={active?.id ?? null}
          onSelect={(id) => setActiveId(id)}
          loading={isLoading}
        />
        {active ? (
          <NicheDetails niche={active} />
        ) : (
          <div className="border border-border rounded-md bg-surface/20 p-8 text-center font-mono text-sm text-muted-foreground">
            // create a niche on the left to begin scoping
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------

function NicheList({
  niches,
  activeId,
  onSelect,
  loading,
}: {
  niches: NicheProfile[];
  activeId: string | null;
  onSelect: (id: string) => void;
  loading: boolean;
}) {
  const create = useCreateNiche();
  const remove = useDeleteNiche();
  const [name, setName] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    try {
      const res = await create.mutateAsync({
        name: trimmed,
        seedKeywords: [],
        contextKeywords: [],
      });
      setName("");
      onSelect(res.id);
    } catch {
      /* surfaced via mutation.error */
    }
  };

  return (
    <section className="space-y-2">
      <form
        onSubmit={submit}
        className="flex gap-1.5 rounded-md border border-border bg-surface/40 p-1.5"
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="New niche (e.g. Computer Science)"
          className="flex-1 bg-transparent px-2 py-1.5 font-mono text-[11px] placeholder:text-muted-foreground/60 focus:outline-none"
        />
        <button
          type="submit"
          disabled={create.isPending || !name.trim()}
          className="inline-flex items-center gap-1 rounded border border-indigo/50 bg-indigo/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-indigo hover:bg-indigo/20 transition-colors disabled:opacity-50"
        >
          {create.isPending ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <Plus className="h-3 w-3" />
          )}
          Add
        </button>
      </form>

      {create.error && (
        <div className="font-mono text-[10px] text-destructive flex items-center gap-1.5">
          <AlertCircle className="h-3 w-3" /> {(create.error as Error).message}
        </div>
      )}

      <div className="border border-border rounded-md bg-surface/20 overflow-hidden">
        {loading ? (
          <div className="p-4 font-mono text-xs text-muted-foreground">// loading niches…</div>
        ) : niches.length === 0 ? (
          <div className="p-4 font-mono text-xs text-muted-foreground">// no niches yet</div>
        ) : (
          niches.map((n) => {
            const active = n.id === activeId;
            return (
              <div
                key={n.id}
                className={cn(
                  "group flex items-center justify-between gap-2 px-3 py-2.5 border-b border-border last:border-b-0 cursor-pointer transition-colors",
                  active ? "bg-indigo/10" : "hover:bg-surface/40",
                )}
                onClick={() => onSelect(n.id)}
              >
                <div className="min-w-0 flex items-center gap-2">
                  <Layers
                    className={cn(
                      "h-3.5 w-3.5 shrink-0",
                      active ? "text-indigo" : "text-muted-foreground",
                    )}
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate">{n.name}</div>
                    <div className="font-mono text-[10px] text-muted-foreground">
                      {n.contextKeywords.length} ctx · {n.sourceCount} src · need{" "}
                      {n.minContextMatches}
                    </div>
                  </div>
                </div>
                <button
                  type="button"
                  title="Delete niche"
                  onClick={(e) => {
                    e.stopPropagation();
                    if (confirm(`Delete niche "${n.name}"? This also removes its sources.`)) {
                      remove.mutate(n.id);
                    }
                  }}
                  className="opacity-0 group-hover:opacity-100 grid h-6 w-6 place-items-center rounded border border-transparent text-muted-foreground hover:text-destructive hover:border-destructive/40 hover:bg-destructive/10 transition-colors"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            );
          })
        )}
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------

function NicheDetails({ niche }: { niche: NicheProfile }) {
  const update = useUpdateNiche();
  const run = useRunNiche();
  const [runMsg, setRunMsg] = useState<string | null>(null);

  const onRun = async () => {
    setRunMsg(null);
    try {
      const res = await run.mutateAsync(niche.id);
      setRunMsg(
        `Dispatched ${res.targets} target${res.targets === 1 ? "" : "s"} · watch System Pulse`,
      );
    } catch (e) {
      setRunMsg((e as Error).message || "Run failed");
    }
  };

  return (
    <section className="space-y-4">
      <div className="border border-border rounded-md bg-surface/20 p-4">
        <div className="flex items-center justify-between gap-2 mb-3">
          <div className="flex items-center gap-2 min-w-0">
            <Layers className="h-4 w-4 text-indigo shrink-0" />
            <h2 className="text-lg font-semibold truncate">{niche.name}</h2>
          </div>
          <button
            type="button"
            onClick={onRun}
            disabled={run.isPending}
            className="inline-flex items-center gap-1.5 rounded-md border border-lime/50 bg-lime/10 px-3 py-1.5 font-mono text-[11px] uppercase tracking-wider text-lime hover:bg-lime/20 transition-colors disabled:opacity-50"
            title="Dispatch every seed keyword + niche link for this niche"
          >
            {run.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="h-3.5 w-3.5" />
            )}
            Run Niche
          </button>
        </div>
        {runMsg && (
          <div className="mb-3 font-mono text-[10px] text-muted-foreground border border-border rounded px-2 py-1 bg-background/40">
            {runMsg}
          </div>
        )}

        <KeywordEditor
          label="Seed Keywords"
          hint="Search terms used to seed scraping (e.g. Architect, AutoCAD)."
          value={niche.seedKeywords}
          onSave={(seedKeywords) =>
            update.mutate({
              id: niche.id,
              body: { name: niche.name, seedKeywords, contextKeywords: niche.contextKeywords },
            })
          }
        />
        <KeywordEditor
          label="Context Keywords"
          hint="Job text must contain at least N of these to be saved under this niche."
          value={niche.contextKeywords}
          onSave={(contextKeywords) =>
            update.mutate({
              id: niche.id,
              body: { name: niche.name, seedKeywords: niche.seedKeywords, contextKeywords },
            })
          }
        />

        <MinContextMatchesField niche={niche} />
      </div>

      <NicheSources niche={niche} />
    </section>
  );
}

// ---------------------------------------------------------------------------

/**
 * MinContextMatchesField uses a local draft so typing "12" doesn't fire
 * a mutation for "1" mid-keystroke (which would race with React Query's
 * refetch and snap the input back to 1). It commits on blur or Enter,
 * shows a saved/dirty/saving indicator, and rejects values < 1.
 */
function MinContextMatchesField({ niche }: { niche: NicheProfile }) {
  const update = useUpdateNiche();
  const [draft, setDraft] = useState<string>(String(niche.minContextMatches));
  const [savedAt, setSavedAt] = useState<number>(0);

  // Whenever the server-side value changes (e.g. after another tab edits
  // it), reset the draft — but only when the user isn't actively editing.
  useEffect(() => {
    setDraft(String(niche.minContextMatches));
  }, [niche.id, niche.minContextMatches]);

  const parsed = parseInt(draft, 10);
  const valid = Number.isFinite(parsed) && parsed >= 1;
  const dirty = valid && parsed !== niche.minContextMatches;

  const commit = () => {
    if (!valid) {
      setDraft(String(niche.minContextMatches));
      return;
    }
    if (!dirty) return;
    update.mutate(
      {
        id: niche.id,
        body: {
          name: niche.name,
          description: niche.description,
          seedKeywords: niche.seedKeywords,
          contextKeywords: niche.contextKeywords,
          minContextMatches: parsed,
        },
      },
      { onSuccess: () => setSavedAt(Date.now()) },
    );
  };

  return (
    <div className="mt-3 flex items-center gap-2">
      <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        Min context matches
      </label>
      <input
        type="number"
        min={1}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            (e.currentTarget as HTMLInputElement).blur();
          } else if (e.key === "Escape") {
            setDraft(String(niche.minContextMatches));
            (e.currentTarget as HTMLInputElement).blur();
          }
        }}
        className={cn(
          "w-16 rounded-md border bg-background/40 px-2 py-1 font-mono text-[11px] focus:outline-none focus:ring-1",
          valid
            ? dirty
              ? "border-indigo/60 focus:ring-indigo/60"
              : "border-border focus:ring-indigo/60"
            : "border-destructive/60 focus:ring-destructive/60",
        )}
      />
      <span className="font-mono text-[10px] text-muted-foreground/70">
        {update.isPending
          ? "saving…"
          : !valid
            ? "must be ≥ 1"
            : dirty
              ? "press Enter or blur to save"
              : Date.now() - savedAt < 2_000
                ? "saved ✓"
                : `live · ${niche.minContextMatches} required`}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------

function KeywordEditor({
  label,
  hint,
  value,
  onSave,
}: {
  label: string;
  hint: string;
  value: string[];
  onSave: (next: string[]) => void;
}) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const v = draft.trim();
    if (!v) return;
    if (value.map((x) => x.toLowerCase()).includes(v.toLowerCase())) {
      setDraft("");
      return;
    }
    onSave([...value, v]);
    setDraft("");
  };
  const remove = (kw: string) => onSave(value.filter((x) => x !== kw));

  return (
    <div className="mt-3">
      <div className="flex items-baseline justify-between gap-2 mb-1.5">
        <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          {label}
        </label>
        <span className="font-mono text-[10px] text-muted-foreground/70">{hint}</span>
      </div>
      <div className="flex flex-wrap gap-1.5 rounded-md border border-border bg-background/40 p-2 min-h-10">
        {value.map((kw) => (
          <span
            key={kw}
            className="inline-flex items-center gap-1 rounded border border-indigo/40 bg-indigo/10 px-1.5 py-0.5 font-mono text-[10px] text-indigo"
          >
            {kw}
            <button
              type="button"
              onClick={() => remove(kw)}
              className="text-indigo/70 hover:text-destructive"
              aria-label={`Remove ${kw}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          placeholder={value.length === 0 ? "press enter to add" : "+ add"}
          className="flex-1 min-w-[100px] bg-transparent font-mono text-[11px] placeholder:text-muted-foreground/60 focus:outline-none"
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------

function NicheSources({ niche }: { niche: NicheProfile }) {
  const { data: sources = [] } = useNicheSources(niche.id);
  const add = useAddNicheSource();
  const remove = useDeleteNicheSource();
  const [url, setUrl] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const u = url.trim();
    if (!u) return;
    try {
      await add.mutateAsync({ id: niche.id, url: u });
      setUrl("");
    } catch {
      /* error surfaced below */
    }
  };

  return (
    <div className="border border-border rounded-md bg-surface/20 p-4">
      <h3 className="text-sm font-medium mb-1">Niche-specific Links</h3>
      <p className="font-mono text-[10px] text-muted-foreground mb-3">
        Career pages added here scrape only when this niche runs and inherit its context filter.
      </p>

      <form
        onSubmit={submit}
        className="flex gap-1.5 rounded-md border border-border bg-background/40 p-1.5"
      >
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://careers.example.com/engineering"
          className="flex-1 bg-transparent px-2 py-1.5 font-mono text-[11px] placeholder:text-muted-foreground/60 focus:outline-none"
        />
        <button
          type="submit"
          disabled={add.isPending || !url.trim()}
          className="inline-flex items-center gap-1 rounded border border-indigo/50 bg-indigo/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-indigo hover:bg-indigo/20 transition-colors disabled:opacity-50"
        >
          {add.isPending ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <Plus className="h-3 w-3" />
          )}
          Add Link
        </button>
      </form>

      {add.error && (
        <div className="mt-1.5 font-mono text-[10px] text-destructive flex items-center gap-1.5">
          <AlertCircle className="h-3 w-3" /> {(add.error as Error).message}
        </div>
      )}

      <div className="mt-3 space-y-1.5">
        {sources.length === 0 ? (
          <div className="font-mono text-[11px] text-muted-foreground">// no links yet</div>
        ) : (
          sources.map((s) => (
            <div
              key={s.id}
              className="flex items-center justify-between gap-2 rounded border border-border bg-background/40 px-2 py-1.5"
            >
              <div className="min-w-0">
                <div className="text-sm truncate">{s.label || prettyCompanyName(s.url)}</div>
                <a
                  href={s.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-mono text-[10px] text-muted-foreground hover:text-indigo truncate inline-block max-w-full"
                >
                  {prettyHostname(s.url)}
                </a>
              </div>
              <button
                type="button"
                onClick={() => remove.mutate({ id: niche.id, sourceId: s.id })}
                className="grid h-6 w-6 place-items-center rounded border border-transparent text-muted-foreground hover:text-destructive hover:border-destructive/40 hover:bg-destructive/10 transition-colors"
                title="Remove link"
              >
                <Trash2 className="h-3 w-3" />
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
