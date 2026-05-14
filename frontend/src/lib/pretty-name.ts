// Convert a raw URL into a human-readable "company name" for the
// Scraper Health Matrix. Examples:
//   https://www.linkedin.com           -> "LinkedIn"
//   https://careers.pathao.com/jobs/12 -> "Pathao"
//   https://jobs.bdjobs.com            -> "BDJobs"
//   https://example.com                -> "Example"

const KNOWN: Record<string, string> = {
  "linkedin.com": "LinkedIn",
  "bdjobs.com": "BDJobs",
  "glassdoor.com": "Glassdoor",
  "indeed.com": "Indeed",
  "github.com": "GitHub",
  "google.com": "Google",
  "facebook.com": "Facebook",
  "meta.com": "Meta",
  "microsoft.com": "Microsoft",
  "amazon.com": "Amazon",
  "apple.com": "Apple",
  "twitter.com": "Twitter",
  "x.com": "X",
  "stackoverflow.com": "Stack Overflow",
  "stackexchange.com": "Stack Exchange",
  "ycombinator.com": "Y Combinator",
  "pathao.com": "Pathao",
  "bkash.com": "bKash",
  "shopup.com": "ShopUp",
  "chaldal.com": "Chaldal",
  "sheba.xyz": "Sheba.xyz",
};

const STRIP_SUBDOMAINS = new Set([
  "www",
  "careers",
  "career",
  "jobs",
  "job",
  "hire",
  "hiring",
  "people",
  "team",
  "talent",
  "work",
]);

/**
 * prettyCompanyName returns a display label for a target URL.
 * It first checks a curated KNOWN map, then falls back to the
 * second-level domain title-cased.
 */
export function prettyCompanyName(rawUrl: string): string {
  if (!rawUrl) return "Unknown target";
  let host = "";
  try {
    const u = new URL(rawUrl.includes("://") ? rawUrl : `https://${rawUrl}`);
    host = u.hostname.toLowerCase();
  } catch {
    return rawUrl;
  }

  // Walk subdomains looking for a known root match (e.g. careers.pathao.com -> pathao.com).
  const parts = host.split(".");
  for (let i = 0; i < parts.length - 1; i++) {
    const candidate = parts.slice(i).join(".");
    if (KNOWN[candidate]) return KNOWN[candidate];
  }

  // Strip generic subdomains like www / careers / jobs.
  const trimmed = parts.filter((p, idx) => {
    if (idx === parts.length - 1) return true;
    if (idx === parts.length - 2) return true;
    return !STRIP_SUBDOMAINS.has(p);
  });

  // Take the second-level domain (last element before the TLD).
  const slDomain = trimmed.length >= 2 ? trimmed[trimmed.length - 2] : (trimmed[0] ?? host);
  if (!slDomain) return host;
  return slDomain.charAt(0).toUpperCase() + slDomain.slice(1);
}

/** prettyHostname returns just `linkedin.com` from a full URL — used as a subtitle. */
export function prettyHostname(rawUrl: string): string {
  try {
    const u = new URL(rawUrl.includes("://") ? rawUrl : `https://${rawUrl}`);
    return u.hostname.replace(/^www\./, "");
  } catch {
    return rawUrl;
  }
}
