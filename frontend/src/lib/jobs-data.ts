export type JobSource = "linkedin" | "bdjobs" | "glassdoor" | "indeed" | "github";

export interface Job {
  id: string;
  /** Full job UUID from the backend; used as the persistent save key. */
  jobUuid?: string;
  title: string;
  company: string;
  location: string;
  salary?: string;
  postedAt: string; // ISO
  /** RFC3339 deadline. Optional: many career pages don't list one; we
   *  prefer "no badge" over a misleading "EXPIRED" chip. */
  deadline?: string | null;
  matchScore: number;
  sources: JobSource[];
  tags: string[];
  type: "Remote" | "Hybrid" | "Onsite";
  level: "Junior" | "Mid" | "Senior" | "Lead";
  merged?: { source: JobSource; url: string; foundAt: string }[];
  /** Number of times this record absorbed a duplicate (server-side merge). */
  mergeCount?: number;
  /** Server flag — kept active jobs only on the global feed. */
  isActive?: boolean;
  url?: string;
}

const now = Date.now();
const days = (n: number) => new Date(now + n * 86400_000).toISOString();
const hoursAgo = (n: number) => new Date(now - n * 3600_000).toISOString();

export const JOBS: Job[] = [
  {
    id: "JOB-7F3A21",
    title: "Senior Flutter Engineer",
    company: "Pathao",
    location: "Dhaka, Bangladesh",
    salary: "৳ 180k–240k",
    postedAt: hoursAgo(3),
    deadline: days(11),
    matchScore: 96,
    sources: ["linkedin", "bdjobs", "glassdoor"],
    tags: ["Flutter", "Dart", "Firebase", "BLoC"],
    type: "Hybrid",
    level: "Senior",
    merged: [
      { source: "linkedin", url: "linkedin.com/jobs/7831", foundAt: hoursAgo(3) },
      { source: "bdjobs", url: "bdjobs.com/3214", foundAt: hoursAgo(2) },
      { source: "glassdoor", url: "glassdoor.com/j-9921", foundAt: hoursAgo(1) },
    ],
  },
  {
    id: "JOB-9C8B14",
    title: "Backend Engineer · Go",
    company: "ShopUp",
    location: "Dhaka, Bangladesh",
    salary: "৳ 220k–300k",
    postedAt: hoursAgo(7),
    deadline: days(6),
    matchScore: 91,
    sources: ["linkedin", "indeed"],
    tags: ["Go", "PostgreSQL", "Kubernetes", "gRPC"],
    type: "Remote",
    level: "Senior",
    merged: [
      { source: "linkedin", url: "linkedin.com/jobs/8812", foundAt: hoursAgo(7) },
      { source: "indeed", url: "indeed.com/j-1132", foundAt: hoursAgo(5) },
    ],
  },
  {
    id: "JOB-2E1D77",
    title: "DevOps Engineer",
    company: "bKash",
    location: "Dhaka, Bangladesh",
    salary: "৳ 160k–220k",
    postedAt: hoursAgo(11),
    deadline: days(3),
    matchScore: 88,
    sources: ["bdjobs"],
    tags: ["AWS", "Terraform", "Ansible", "Prometheus"],
    type: "Onsite",
    level: "Mid",
  },
  {
    id: "JOB-5A4490",
    title: "Senior Frontend Engineer (React)",
    company: "Chaldal",
    location: "Dhaka, Bangladesh",
    salary: "৳ 200k–260k",
    postedAt: hoursAgo(14),
    deadline: days(9),
    matchScore: 84,
    sources: ["linkedin", "github"],
    tags: ["React", "TypeScript", "Next.js", "Redux"],
    type: "Hybrid",
    level: "Senior",
    merged: [
      { source: "linkedin", url: "linkedin.com/jobs/7012", foundAt: hoursAgo(14) },
      { source: "github", url: "github.com/jobs/2210", foundAt: hoursAgo(12) },
    ],
  },
  {
    id: "JOB-1B6C30",
    title: "Mobile Engineer · Kotlin",
    company: "Sheba.xyz",
    location: "Dhaka, Bangladesh",
    salary: "৳ 140k–190k",
    postedAt: hoursAgo(18),
    deadline: days(14),
    matchScore: 79,
    sources: ["bdjobs", "linkedin"],
    tags: ["Kotlin", "Android", "Jetpack", "Coroutines"],
    type: "Onsite",
    level: "Mid",
    merged: [
      { source: "bdjobs", url: "bdjobs.com/4421", foundAt: hoursAgo(18) },
      { source: "linkedin", url: "linkedin.com/jobs/4422", foundAt: hoursAgo(16) },
    ],
  },
  {
    id: "JOB-8D2F09",
    title: "Data Engineer",
    company: "Therap (BD)",
    location: "Dhaka, Bangladesh",
    salary: "৳ 170k–230k",
    postedAt: hoursAgo(22),
    deadline: days(20),
    matchScore: 76,
    sources: ["linkedin"],
    tags: ["Airflow", "Spark", "Snowflake", "Python"],
    type: "Hybrid",
    level: "Senior",
  },
  {
    id: "JOB-4C8800",
    title: "Junior Flutter Developer",
    company: "Doctorola",
    location: "Chattogram, Bangladesh",
    salary: "৳ 60k–90k",
    postedAt: hoursAgo(28),
    deadline: days(2),
    matchScore: 71,
    sources: ["bdjobs", "indeed"],
    tags: ["Flutter", "Firebase", "REST"],
    type: "Onsite",
    level: "Junior",
    merged: [
      { source: "bdjobs", url: "bdjobs.com/9911", foundAt: hoursAgo(28) },
      { source: "indeed", url: "indeed.com/j-7711", foundAt: hoursAgo(26) },
    ],
  },
  {
    id: "JOB-6F7712",
    title: "Site Reliability Engineer",
    company: "Pridesys IT",
    location: "Remote · Bangladesh",
    salary: "৳ 200k–280k",
    postedAt: hoursAgo(34),
    deadline: days(17),
    matchScore: 69,
    sources: ["linkedin", "github", "glassdoor"],
    tags: ["Kubernetes", "Grafana", "Istio", "Go"],
    type: "Remote",
    level: "Lead",
    merged: [
      { source: "linkedin", url: "linkedin.com/jobs/2002", foundAt: hoursAgo(34) },
      { source: "github", url: "github.com/jobs/8819", foundAt: hoursAgo(33) },
      { source: "glassdoor", url: "glassdoor.com/j-7780", foundAt: hoursAgo(30) },
    ],
  },
  {
    id: "JOB-0A1199",
    title: "Full-Stack Engineer (Node + React)",
    company: "Brain Station 23",
    location: "Dhaka, Bangladesh",
    salary: "৳ 130k–180k",
    postedAt: hoursAgo(40),
    deadline: days(8),
    matchScore: 64,
    sources: ["bdjobs"],
    tags: ["Node.js", "React", "MongoDB", "AWS"],
    type: "Hybrid",
    level: "Mid",
  },
];

export const PULSE_FEED = [
  { id: 1, t: hoursAgo(0.05), kind: "scrape", msg: "Scraper found 12 new roles on LinkedIn" },
  { id: 2, t: hoursAgo(0.1), kind: "merge", msg: "Deduplication engine merged 3 posts" },
  { id: 3, t: hoursAgo(0.4), kind: "archive", msg: "Janitor archived 5 expired jobs" },
  { id: 4, t: hoursAgo(0.8), kind: "scrape", msg: "BDJobs sync completed · 47 records" },
  { id: 5, t: hoursAgo(1.2), kind: "merge", msg: "Cross-source match: Pathao × 3 sources" },
  { id: 6, t: hoursAgo(1.6), kind: "alert", msg: "Glassdoor rate-limit cooldown 30s" },
  { id: 7, t: hoursAgo(2.1), kind: "scrape", msg: "GitHub Jobs feed indexed · 9 records" },
  { id: 8, t: hoursAgo(2.7), kind: "archive", msg: "Janitor archived 2 closed reqs" },
  { id: 9, t: hoursAgo(3.4), kind: "merge", msg: "Deduplication engine merged 1 post" },
  { id: 10, t: hoursAgo(4.1), kind: "scrape", msg: "Indeed BD sync completed · 22 records" },
] as const;
