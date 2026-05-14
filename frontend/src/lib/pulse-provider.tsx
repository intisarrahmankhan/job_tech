import { useEffect, useRef, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { PulseContext } from "./pulse-context";
import { usePulseSocket } from "./pulse-socket";

/**
 * PulseProvider opens the single shared WebSocket connection for the
 * entire dashboard and exposes its state to every descendant via the
 * `usePulse` hook re-exported from `./pulse-context`. Mounting it once
 * at the shell level prevents the misbehaviour where two sibling
 * consumers (CommandBar + PulseSidebar) would each spin up their own
 * connection.
 *
 * It also bridges incoming pulse events into the React Query cache so a
 * scraper completing or merging a job triggers an immediate refetch of
 * `["jobs"]`, `["targets"]`, and `["scraper-state"]` — without this,
 * the user would have to manually reload to see new rows in the feed.
 */
export function PulseProvider({ children }: { children: ReactNode }) {
  const socket = usePulseSocket();
  const qc = useQueryClient();
  const lastSeenIdRef = useRef<number>(0);

  useEffect(() => {
    if (socket.events.length === 0) return;
    const newest = socket.events[0];
    if (newest.id <= lastSeenIdRef.current) return;
    lastSeenIdRef.current = newest.id;

    // Map kind → cache invalidation set. We deliberately stay coarse:
    // any "merge" event nukes the jobs cache, any "scrape" event nukes
    // the targets + state cache. The granular per-task SWR-like polling
    // already on each hook handles the in-between updates.
    if (newest.kind === "merge") {
      qc.invalidateQueries({ queryKey: ["jobs"] });
      qc.invalidateQueries({ queryKey: ["scraper-metrics", "24h"] });
    } else if (newest.kind === "scrape") {
      qc.invalidateQueries({ queryKey: ["targets"] });
      qc.invalidateQueries({ queryKey: ["scraper-state"] });
      qc.invalidateQueries({ queryKey: ["scraper-metrics", "24h"] });
      // A "done" event in the message flushes jobs too so newly-saved
      // rows appear without waiting for the 30 s staleTime.
      if (/done|complete|merge/i.test(newest.msg)) {
        qc.invalidateQueries({ queryKey: ["jobs"] });
      }
    } else if (newest.kind === "alert") {
      qc.invalidateQueries({ queryKey: ["scraper-logs", "all", 25] });
      qc.invalidateQueries({ queryKey: ["scraper-state"] });
    }
  }, [socket.events, qc]);

  return <PulseContext.Provider value={socket}>{children}</PulseContext.Provider>;
}
