import { useEffect, useRef, useState } from "react";

export type PulseKind = "scrape" | "merge" | "archive" | "alert";

export interface PulseEvent {
  id: number;
  kind: PulseKind;
  msg: string;
  /** ISO timestamp from the backend. */
  t: string;
}

const API_BASE =
  (import.meta as ImportMeta & { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ??
  "http://localhost:8000";

function wsUrl(): string {
  // Convert http(s)://host -> ws(s)://host/ws/pulse
  try {
    const u = new URL(API_BASE);
    const proto = u.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${u.host}/ws/pulse`;
  } catch {
    return "ws://localhost:8000/ws/pulse";
  }
}

const MAX_EVENTS = 32;

export interface UsePulseSocketResult {
  events: PulseEvent[];
  connected: boolean;
}

/**
 * usePulseSocket subscribes to the backend pulse WebSocket. Returns the
 * rolling buffer of recent events plus the live connection flag.
 *
 * Lifecycle:
 *   - Opens on mount; closes on unmount (handles tab navigation).
 *   - Pauses + closes when the tab becomes hidden, reconnects on visible.
 *   - Auto-reconnects with capped backoff (1s, 2s, 4s, max 8s).
 */
export function usePulseSocket(): UsePulseSocketResult {
  const [events, setEvents] = useState<PulseEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const backoffRef = useRef(1000);
  const reconnectTimerRef = useRef<number | null>(null);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;

    const connect = () => {
      if (!aliveRef.current) return;
      if (typeof document !== "undefined" && document.hidden) return;

      const ws = new WebSocket(wsUrl());
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        backoffRef.current = 1000;
      };
      ws.onmessage = (e) => {
        try {
          const ev = JSON.parse(e.data) as PulseEvent;
          setEvents((prev) => {
            const next = [ev, ...prev];
            return next.length > MAX_EVENTS ? next.slice(0, MAX_EVENTS) : next;
          });
        } catch {
          /* ignore malformed frames */
        }
      };
      ws.onclose = () => {
        setConnected(false);
        wsRef.current = null;
        if (!aliveRef.current) return;
        const delay = Math.min(backoffRef.current, 8000);
        backoffRef.current = Math.min(backoffRef.current * 2, 8000);
        reconnectTimerRef.current = window.setTimeout(connect, delay);
      };
      ws.onerror = () => {
        // ws.onclose will fire next; backoff there.
        ws.close();
      };
    };

    const handleVisibility = () => {
      if (document.hidden) {
        wsRef.current?.close();
      } else if (!wsRef.current) {
        backoffRef.current = 1000;
        connect();
      }
    };

    connect();
    document.addEventListener("visibilitychange", handleVisibility);

    return () => {
      aliveRef.current = false;
      document.removeEventListener("visibilitychange", handleVisibility);
      if (reconnectTimerRef.current !== null) {
        clearTimeout(reconnectTimerRef.current);
      }
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  return { events, connected };
}

/**
 * triggerScraperRefresh asks the backend to launch the scraper now.
 * Returns true on 202 Accepted, false if a run is already in progress.
 */
export async function triggerScraperRefresh(): Promise<boolean> {
  const res = await fetch(`${API_BASE}/api/refresh`, { method: "POST" });
  return res.status === 202;
}
