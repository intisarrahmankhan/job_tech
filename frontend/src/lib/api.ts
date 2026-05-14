import { useQuery, keepPreviousData, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { Job } from "./jobs-data";

export const API_BASE =
  (import.meta as ImportMeta & { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ??
  "http://localhost:8000";

export interface JobsFilters {
  q?: string;
  role?: string;
  location?: string;
  nicheId?: string;
}

export async function fetchJobs(filters: JobsFilters = {}): Promise<Job[]> {
  const params = new URLSearchParams();
  if (filters.q?.trim()) params.set("q", filters.q.trim());
  if (filters.role?.trim()) params.set("role", filters.role.trim());
  if (filters.location?.trim()) params.set("location", filters.location.trim());
  if (filters.nicheId?.trim()) params.set("nicheId", filters.nicheId.trim());

  const qs = params.toString();
  const url = `${API_BASE}/api/jobs${qs ? `?${qs}` : ""}`;

  const res = await fetch(url, { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(`GET /api/jobs failed: ${res.status}`);
  return (await res.json()) as Job[];
}

/**
 * useJobs subscribes to the backend job feed. Pass filter values that have
 * already been debounced — the hook does not debounce on its own.
 */
export function useJobs(filters: JobsFilters = {}) {
  return useQuery<Job[]>({
    queryKey: [
      "jobs",
      filters.q ?? "",
      filters.role ?? "",
      filters.location ?? "",
      filters.nicheId ?? "",
    ],
    queryFn: () => fetchJobs(filters),
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  });
}

/**
 * useDebouncedValue returns `value` but only after it has stopped changing
 * for `delayMs`. Used by the command bar so we don't fire a request on
 * every keystroke.
 */
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}

// ---------------------------------------------------------------------------
// User-defined scrape targets

export type ScrapeTaskType = "KEYWORD" | "DIRECT_URL";
export type ScrapeTaskStatus = "pending" | "running" | "healthy" | "failed";

export interface ScrapeTask {
  id: string;
  type: ScrapeTaskType;
  value: string;
  frequency: "hourly" | "6h" | "daily";
  status: ScrapeTaskStatus;
  lastRunAt?: string;
  lastError?: string;
  resultCount: number;
  isActive: boolean;
  createdAt: string;
}

export interface NewTargetInput {
  type: ScrapeTaskType;
  value: string;
  frequency?: "hourly" | "6h" | "daily";
  nicheId?: string;
}

export async function fetchTargets(): Promise<ScrapeTask[]> {
  const res = await fetch(`${API_BASE}/api/targets`);
  if (!res.ok) throw new Error(`GET /api/targets failed: ${res.status}`);
  return (await res.json()) as ScrapeTask[];
}

export async function createTarget(input: NewTargetInput): Promise<ScrapeTask> {
  const res = await fetch(`${API_BASE}/api/targets`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    let msg = `POST /api/targets failed: ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as ScrapeTask;
}

export async function runTarget(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/targets/${id}/run`, { method: "POST" });
  if (!res.ok) {
    let msg = `POST /api/targets/${id}/run failed: ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
}

export async function deleteTarget(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/targets/${id}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`DELETE failed: ${res.status}`);
}

export async function pauseTarget(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/targets/${id}/pause`, { method: "POST" });
  if (!res.ok) throw new Error(`pause failed: ${res.status}`);
}

export async function resumeTarget(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/targets/${id}/resume`, { method: "POST" });
  if (!res.ok) throw new Error(`resume failed: ${res.status}`);
}

// ---------------------------------------------------------------------------
// Global scraper kill-switch (Pause/Start on the Scraper Health Matrix).

export interface ScraperState {
  running: boolean;
  paused: boolean;
  lastRun: string;
  lastError?: string;
  /** PID of the most recently spawned Python subprocess (0 when none). */
  lastPid?: number;
}

export async function fetchScraperState(): Promise<ScraperState> {
  const res = await fetch(`${API_BASE}/api/refresh/status`, {
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`scraper state failed: ${res.status}`);
  return (await res.json()) as ScraperState;
}

export async function pauseScraper(): Promise<void> {
  const res = await fetch(`${API_BASE}/api/scraper/pause`, { method: "POST" });
  if (!res.ok) throw new Error(`pause global failed: ${res.status}`);
}

export async function resumeScraper(): Promise<void> {
  const res = await fetch(`${API_BASE}/api/scraper/resume`, { method: "POST" });
  if (!res.ok) throw new Error(`resume global failed: ${res.status}`);
}

export function useTargets() {
  return useQuery<ScrapeTask[]>({
    queryKey: ["targets"],
    queryFn: fetchTargets,
    // Poll while a target is running so the status badge updates.
    refetchInterval: (q) => (q.state.data?.some((t) => t.status === "running") ? 2_000 : 15_000),
  });
}

export function useCreateTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createTarget,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["targets"] }),
  });
}

export function useRunTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: runTarget,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["targets"] }),
  });
}

export function useDeleteTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteTarget,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["targets"] }),
  });
}

export function usePauseTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: pauseTarget,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["targets"] }),
  });
}

export function useResumeTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: resumeTarget,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["targets"] }),
  });
}

export function useScraperState() {
  return useQuery<ScraperState>({
    queryKey: ["scraper-state"],
    queryFn: fetchScraperState,
    refetchInterval: 4_000,
  });
}

export function usePauseScraper() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: pauseScraper,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["scraper-state"] }),
  });
}

export function useResumeScraper() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: resumeScraper,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["scraper-state"] }),
  });
}

// ---------------------------------------------------------------------------
// Scraper telemetry rollup (Health Matrix ERR RATE column)

export interface TaskRollup {
  taskId: string;
  total: number;
  success: number;
  failure: number;
  rejected: number;
  errRate: number;
}

export interface NicheBatch {
  nicheId: string;
  success: number;
  rejected: number;
  failure: number;
}

export interface MetricsResponse {
  byTask: Record<string, TaskRollup>;
  byNiche: Record<string, NicheBatch>;
  since: string;
}

export function useScraperMetrics(since = "24h") {
  return useQuery<MetricsResponse>({
    queryKey: ["scraper-metrics", since],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/api/scraper/metrics?since=${since}`);
      if (!res.ok) throw new Error(`metrics failed: ${res.status}`);
      return (await res.json()) as MetricsResponse;
    },
    refetchInterval: 5_000,
  });
}

// ---------------------------------------------------------------------------
// Scraper logs (captured stderr + structured failure history).
// Drives the "Last failures" panel on /scraper-health.

export interface ScraperLogRow {
  id: string;
  taskId?: string;
  nicheId?: string;
  url?: string;
  stage?: string;
  error?: string;
  details?: string;
  exitCode: number;
  createdAt: string;
}

export function useScraperLogs(taskId?: string, limit = 25) {
  return useQuery<ScraperLogRow[]>({
    queryKey: ["scraper-logs", taskId ?? "all", limit],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (taskId) params.set("taskId", taskId);
      params.set("limit", String(limit));
      const res = await fetch(`${API_BASE}/api/scraper/logs?${params.toString()}`);
      if (!res.ok) throw new Error(`scraper logs failed: ${res.status}`);
      return (await res.json()) as ScraperLogRow[];
    },
    refetchInterval: 5_000,
  });
}

export function useKillScraper() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(`${API_BASE}/api/scraper/kill`, {
        method: "POST",
      });
      if (!res.ok) throw new Error(`kill failed: ${res.status}`);
      return (await res.json()) as { killed: boolean };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["scraper-state"] });
      qc.invalidateQueries({ queryKey: ["targets"] });
    },
  });
}

// ---------------------------------------------------------------------------
// Admin · Dry-run scraper validator (no DB writes).

export interface DryRunPassed {
  title: string;
  company: string;
  url: string;
  matchScore: number;
  threshold: number;
}

export interface DryRunFailed {
  title: string;
  company: string;
  url: string;
  hits: number;
  threshold: number;
  missing: string[];
  reason: string;
}

export interface DryRunResponse {
  status: "completed" | "failed";
  url?: string;
  nicheId?: string;
  nicheName?: string;
  totalJobs: number;
  passed: DryRunPassed[];
  failed: DryRunFailed[];
  stderr?: string;
  durationMs: number;
  exitCode: number;
}

export interface DryRunRequest {
  url?: string;
  nicheId: string;
  seedKeywords?: string;
}

export function useDryRunScraper() {
  return useMutation<DryRunResponse, Error, DryRunRequest>({
    mutationFn: async (body) => {
      const res = await fetch(`${API_BASE}/api/admin/scrapers/test`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const j = (await res.json().catch(() => null)) as DryRunResponse | { error: string } | null;
      if (!res.ok) {
        // Bad-Gateway responses still carry a partial DryRunResponse with
        // the captured stderr — return that so the UI can render it.
        if (j && "status" in j) return j as DryRunResponse;
        throw new Error((j && "error" in j && j.error) || `dry-run failed: ${res.status}`);
      }
      return j as DryRunResponse;
    },
  });
}

// ---------------------------------------------------------------------------
// Niche profiles (Phase 4)

export interface NicheProfile {
  id: string;
  name: string;
  description?: string;
  seedKeywords: string[];
  contextKeywords: string[];
  minContextMatches: number;
  sourceCount: number;
  createdAt: string;
}

export interface NicheSource {
  id: string;
  nicheId: string;
  url: string;
  label?: string;
  isActive: boolean;
  createdAt: string;
}

export interface NicheBody {
  name: string;
  description?: string;
  seedKeywords: string[];
  contextKeywords: string[];
  minContextMatches?: number;
}

async function jsonRequest<T>(input: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(input, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      ...(init.headers ?? {}),
    },
  });
  if (!res.ok) {
    let msg = `${init.method ?? "GET"} ${input} failed: ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const nichesApi = {
  list: () => jsonRequest<NicheProfile[]>(`${API_BASE}/api/niches`),
  create: (body: NicheBody) =>
    jsonRequest<NicheProfile>(`${API_BASE}/api/niches`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  update: (id: string, body: Partial<NicheBody>) =>
    jsonRequest<NicheProfile>(`${API_BASE}/api/niches/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  remove: (id: string) =>
    jsonRequest<{ status: string }>(`${API_BASE}/api/niches/${id}`, { method: "DELETE" }),
  run: (id: string) =>
    jsonRequest<{ status: string; targets: number }>(`${API_BASE}/api/niches/${id}/run`, {
      method: "POST",
    }),
  listSources: (id: string) => jsonRequest<NicheSource[]>(`${API_BASE}/api/niches/${id}/sources`),
  addSource: (id: string, url: string, label?: string) =>
    jsonRequest<NicheSource>(`${API_BASE}/api/niches/${id}/sources`, {
      method: "POST",
      body: JSON.stringify({ url, label }),
    }),
  removeSource: (id: string, sourceId: string) =>
    jsonRequest<{ status: string }>(`${API_BASE}/api/niches/${id}/sources/${sourceId}`, {
      method: "DELETE",
    }),
};

export function useNiches() {
  return useQuery<NicheProfile[]>({
    queryKey: ["niches"],
    queryFn: nichesApi.list,
    staleTime: 30_000,
  });
}

export function useNicheSources(id: string | null) {
  return useQuery<NicheSource[]>({
    queryKey: ["niches", id, "sources"],
    queryFn: () => nichesApi.listSources(id!),
    enabled: !!id,
  });
}

export function useCreateNiche() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: nichesApi.create,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["niches"] }),
  });
}

export function useUpdateNiche() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<NicheBody> }) =>
      nichesApi.update(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["niches"] }),
  });
}

export function useDeleteNiche() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: nichesApi.remove,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["niches"] }),
  });
}

export function useRunNiche() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: nichesApi.run,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["scraper-state"] }),
  });
}

export function useAddNicheSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, url, label }: { id: string; url: string; label?: string }) =>
      nichesApi.addSource(id, url, label),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ["niches", vars.id, "sources"] });
      qc.invalidateQueries({ queryKey: ["niches"] });
      qc.invalidateQueries({ queryKey: ["targets"] });
    },
  });
}

// ---------------------------------------------------------------------------
// Persisted saved-jobs — survives cache clears when profile email is set.

function getUserKey(): string {
  // 1. Profile email (set by the sidebar profile widget → surviving cache
  //    clears on a new device, if the user signs back in with the same email).
  try {
    const raw = window.localStorage.getItem("scout:profile");
    if (raw) {
      const p = JSON.parse(raw) as { email?: string };
      if (p.email?.trim()) return p.email.trim().toLowerCase();
    }
  } catch {
    /* ignore */
  }
  // 2. Opaque anonymous UUID; loses saved jobs on cache clear, which is
  //    the intentional tradeoff for not requiring real auth.
  const KEY = "scout:userKey";
  try {
    const existing = window.localStorage.getItem(KEY);
    if (existing) return existing;
    const next = crypto.randomUUID();
    window.localStorage.setItem(KEY, next);
    return next;
  } catch {
    return "anonymous";
  }
}

const USER_HEADERS = () => ({
  "X-Scout-User": getUserKey(),
});

export interface SavedJobRow {
  id: string;
  userKey: string;
  jobId: string;
  createdAt: string;
}

export async function fetchSavedJobs(): Promise<SavedJobRow[]> {
  const res = await fetch(`${API_BASE}/api/saved-jobs`, {
    headers: { Accept: "application/json", ...USER_HEADERS() },
  });
  if (!res.ok) throw new Error(`GET /saved-jobs failed: ${res.status}`);
  return (await res.json()) as SavedJobRow[];
}

export async function saveJob(jobUuid: string, displayId?: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/saved-jobs`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...USER_HEADERS() },
    body: JSON.stringify({ jobId: jobUuid, displayId }),
  });
  if (!res.ok && res.status !== 409) {
    let msg = `save failed: ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
}

export async function unsaveJob(jobUuid: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/saved-jobs/${encodeURIComponent(jobUuid)}`, {
    method: "DELETE",
    headers: USER_HEADERS(),
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(`unsave failed: ${res.status}`);
  }
}

export function useSavedJobs() {
  return useQuery<SavedJobRow[]>({
    queryKey: ["saved-jobs"],
    queryFn: fetchSavedJobs,
    staleTime: 60_000,
  });
}

/**
 * useSaveToggle gives the rest of the app a simple `isSaved(job)` +
 * `toggle(job)` pair. We still keep a mirror in localStorage so the UI
 * feels instant even before the backend round-trips, but the backend is
 * the source of truth the next time the user reloads.
 */
export function useSaveToggle() {
  const qc = useQueryClient();
  const { data: rows = [] } = useSavedJobs();

  const savedSet = new Set(rows.map((r) => r.jobId));

  const toggle = async (jobUuid: string, displayId?: string) => {
    const isSaved = savedSet.has(jobUuid);
    qc.setQueryData<SavedJobRow[]>(["saved-jobs"], (cur = []) =>
      isSaved
        ? cur.filter((r) => r.jobId !== jobUuid)
        : [
            ...cur,
            { id: jobUuid, userKey: "", jobId: jobUuid, createdAt: new Date().toISOString() },
          ],
    );
    try {
      if (isSaved) {
        await unsaveJob(jobUuid);
      } else {
        await saveJob(jobUuid, displayId);
      }
    } finally {
      qc.invalidateQueries({ queryKey: ["saved-jobs"] });
    }
  };

  return {
    isSaved: (jobUuid: string) => savedSet.has(jobUuid),
    toggle,
  };
}

export function useDeleteNicheSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, sourceId }: { id: string; sourceId: string }) =>
      nichesApi.removeSource(id, sourceId),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ["niches", vars.id, "sources"] });
      qc.invalidateQueries({ queryKey: ["niches"] });
    },
  });
}
