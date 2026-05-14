import { createContext, useContext } from "react";
import type { UsePulseSocketResult } from "./pulse-socket";

/**
 * Pulse context wiring. Lives in its own non-component file so Vite
 * Fast Refresh stays happy: the rule "a file may either export
 * components OR non-components, never both" means we keep the JSX
 * provider in `pulse-provider.tsx` and the context object + the
 * `usePulse` hook here.
 */

const defaultValue: UsePulseSocketResult = { events: [], connected: false };

export const PulseContext = createContext<UsePulseSocketResult>(defaultValue);

/**
 * Subscribe to the shared pulse WebSocket state. Returns the rolling
 * event buffer plus the live connection flag.
 */
export function usePulse(): UsePulseSocketResult {
  return useContext(PulseContext);
}
