import { createFileRoute } from "@tanstack/react-router";
import { DashboardShell } from "@/components/dashboard-shell";
import { JobMatrix } from "@/components/job-matrix";
import { useDebouncedValue, useJobs, useSavedJobs } from "@/lib/api";
import { useMemo } from "react";

export const Route = createFileRoute("/saved")({
  head: () => ({ meta: [{ title: "Scout · Saved Jobs" }] }),
  component: SavedJobs,
});

function SavedJobs() {
  return <DashboardShell>{({ query }) => <SavedJobsBody query={query} />}</DashboardShell>;
}

function SavedJobsBody({ query }: { query: string }) {
  const debouncedQuery = useDebouncedValue(query, 300);
  const {
    data: jobs = [],
    isLoading,
    isFetching,
    refetch,
  } = useJobs({
    q: debouncedQuery,
  });
  const { data: rows = [] } = useSavedJobs();

  // Backend stores the full UUID; the display id ("JOB-abcdef") is what
  // the in-memory fallback uses. Match on either so saves persist across
  // a database restart.
  const saved = useMemo(() => {
    const ids = new Set(rows.map((r) => r.jobId));
    return jobs.filter((j) => {
      if (j.jobUuid && ids.has(j.jobUuid)) return true;
      return ids.has(j.id);
    });
  }, [jobs, rows]);

  return (
    <div className="p-3 sm:p-4 space-y-4">
      <div>
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground mb-1">
          /saved-jobs
        </div>
        <h1 className="text-xl sm:text-2xl font-semibold tracking-tight">Saved Positions</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {saved.length} bookmarked {saved.length === 1 ? "role" : "roles"} · synced to Postgres
        </p>
      </div>
      <JobMatrix
        jobs={saved}
        query={debouncedQuery}
        loading={isLoading || isFetching}
        onRefresh={() => refetch()}
        emptyLabel="no saved jobs yet — bookmark roles from the global feed"
      />
    </div>
  );
}
