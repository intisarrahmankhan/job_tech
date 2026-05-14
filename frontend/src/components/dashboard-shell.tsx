import { useEffect, useState, type ReactNode } from "react";
import { LateralNav, MobileBottomNav } from "./lateral-nav";
import { CommandBar } from "./command-bar";
import { PulseSidebar } from "./pulse-sidebar";
import { PulseProvider } from "@/lib/pulse-provider";

interface ShellProps {
  children: (ctx: { query: string }) => ReactNode;
  showPulse?: boolean;
}

export function DashboardShell({ children, showPulse = true }: ShellProps) {
  const [query, setQuery] = useState("");
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  return (
    <PulseProvider>
      <div className="min-h-screen flex bg-background text-foreground">
        <LateralNav />
        <div className="flex-1 flex min-w-0">
          <div className="flex-1 flex flex-col min-w-0">
            <CommandBar value={query} onChange={setQuery} />
            <main className="flex-1 pb-16 md:pb-0">{mounted ? children({ query }) : null}</main>
          </div>
          {showPulse && <PulseSidebar />}
        </div>
        <MobileBottomNav />
      </div>
    </PulseProvider>
  );
}
