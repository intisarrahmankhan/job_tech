import { useState, useRef, useEffect } from "react";
import { Bell, ChevronRight, Command } from "lucide-react";
import { usePulse } from "@/lib/pulse-context";

interface CommandBarProps {
  value: string;
  onChange: (v: string) => void;
}

export function CommandBar({ value, onChange }: CommandBarProps) {
  const [focused, setFocused] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const { connected } = usePulse();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        inputRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border bg-background/85 px-3 sm:px-4 backdrop-blur-md">
      <div className="hidden sm:flex items-center gap-1.5 font-mono text-[11px] text-muted-foreground">
        <span className="text-lime">scout</span>
        <ChevronRight className="h-3 w-3" />
      </div>

      <div
        className={`group relative flex flex-1 items-center gap-2 rounded-md border bg-surface/60 px-3 h-9 font-mono text-sm transition-all ${
          focused ? "border-indigo ring-1 ring-indigo/30" : "border-border"
        }`}
      >
        <span className="text-lime select-none">$</span>
        <input
          ref={inputRef}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          spellCheck={false}
          autoComplete="off"
          placeholder="Search jobs by title, company, or tag…  (Ctrl+K)"
          className="flex-1 bg-transparent outline-none placeholder:text-muted-foreground/60 text-foreground"
        />
        <kbd className="hidden sm:inline-flex items-center gap-1 rounded border border-border bg-background px-1.5 py-0.5 text-[10px] text-muted-foreground">
          <Command className="h-2.5 w-2.5" />K
        </kbd>
      </div>

      <button
        type="button"
        title="Notifications"
        className="relative grid h-9 w-9 place-items-center rounded-md border border-border bg-surface/60 text-muted-foreground hover:text-foreground hover:bg-surface transition-colors"
      >
        <Bell className="h-4 w-4" />
        <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-lime live-dot" />
      </button>

      <div
        className="hidden sm:flex h-9 items-center gap-2 rounded-md border border-border bg-surface/60 px-2.5"
        title={connected ? "Backend reachable" : "Backend offline — start the Go API"}
      >
        <span
          className={`h-1.5 w-1.5 rounded-full ${
            connected ? "bg-lime live-dot" : "bg-destructive"
          }`}
        />
        <span className="font-mono text-[11px] text-muted-foreground">
          {connected ? "ONLINE" : "OFFLINE"}
        </span>
      </div>
    </header>
  );
}
