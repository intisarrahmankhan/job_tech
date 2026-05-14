export function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.max(1, Math.floor(diff / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

export interface Countdown {
  label: string;
  urgent: boolean;
  expired: boolean;
  /** True when the scraper never extracted a deadline — the UI should hide
   *  the EXPIRED chip and show "Open" instead so users don't see a sea of
   *  red badges on jobs that simply don't advertise a closing date. */
  unknown: boolean;
}

/**
 * countdown converts an optional ISO deadline string into a display chip.
 *
 *   - undefined / null / empty / pre-1971 zero values → "Open" (unknown=true)
 *   - past deadline → "EXPIRED" (expired=true)
 *   - future        → "Nd Mh" / "Nh Mm" / "Nm" with urgent flag at <3 days
 */
export function countdown(iso?: string | null): Countdown {
  if (!iso) {
    return { label: "Open", urgent: false, expired: false, unknown: true };
  }
  const t = new Date(iso).getTime();
  // Treat invalid dates and Go's zero-value `0001-01-01T00:00:00Z`
  // (which arrives if the backend ever forgets to use omitempty) as
  // "deadline unknown" rather than "expired ~2000 years ago".
  if (!Number.isFinite(t) || t < 0) {
    return { label: "Open", urgent: false, expired: false, unknown: true };
  }
  const diff = t - Date.now();
  if (diff <= 0) return { label: "EXPIRED", urgent: false, expired: true, unknown: false };
  const d = Math.floor(diff / 86400_000);
  const h = Math.floor((diff % 86400_000) / 3600_000);
  const m = Math.floor((diff % 3600_000) / 60_000);
  const urgent = diff < 3 * 86400_000;
  if (d > 0) return { label: `${d}d ${h}h`, urgent, expired: false, unknown: false };
  if (h > 0) return { label: `${h}h ${m}m`, urgent, expired: false, unknown: false };
  return { label: `${m}m`, urgent: true, expired: false, unknown: false };
}
