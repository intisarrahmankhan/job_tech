import { Github, Linkedin, Briefcase, Building2, Search } from "lucide-react";
import type { JobSource } from "@/lib/jobs-data";

const META: Record<
  JobSource,
  { label: string; Icon: React.ComponentType<{ className?: string }>; tone: string }
> = {
  linkedin: { label: "LinkedIn", Icon: Linkedin, tone: "text-[#60a5fa]" },
  bdjobs: { label: "BDJobs", Icon: Briefcase, tone: "text-lime" },
  glassdoor: { label: "Glassdoor", Icon: Building2, tone: "text-[#34d399]" },
  indeed: { label: "Indeed", Icon: Search, tone: "text-[#818cf8]" },
  github: { label: "GitHub", Icon: Github, tone: "text-foreground" },
};

interface SourceBadgeProps {
  source: JobSource;
  mini?: boolean;
  /** When set, the badge becomes an anchor that opens this URL in a new tab. */
  href?: string;
}

export function SourceBadge({ source, mini = false, href }: SourceBadgeProps) {
  const meta = META[source];
  // Unknown sources (e.g. arbitrary hostnames returned by the generic
  // direct-URL scraper) fall back to a neutral pill so the UI never crashes.
  const label = meta?.label ?? source;
  const Icon = meta?.Icon;
  const tone = meta?.tone ?? "text-muted-foreground";

  const miniBox = `grid h-5 w-5 place-items-center rounded border border-border bg-background/60 ${tone}`;
  const fullBox = `inline-flex items-center gap-1.5 rounded border border-border bg-background/60 px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider ${tone}`;
  const linkExtras = href
    ? "hover:border-foreground/60 hover:bg-background transition-colors cursor-pointer"
    : "";

  const inner = mini ? (
    Icon ? (
      <Icon className="h-3 w-3" />
    ) : (
      <span className="text-[8px]">{label.slice(0, 2)}</span>
    )
  ) : (
    <>
      {Icon && <Icon className="h-3 w-3" />}
      {label}
    </>
  );

  const className = `${mini ? miniBox : fullBox} ${linkExtras}`.trim();

  if (href) {
    return (
      <a
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        onClick={(e) => e.stopPropagation()}
        title={`Open ${label} posting`}
        className={className}
      >
        {inner}
      </a>
    );
  }

  return (
    <span title={label} className={className}>
      {inner}
    </span>
  );
}
