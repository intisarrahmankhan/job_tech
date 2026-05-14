import { useEffect, useRef, useState } from "react";
import { LogIn, LogOut, User } from "lucide-react";
import { useProfile, profileInitials } from "@/lib/profile";
import { cn } from "@/lib/utils";

/**
 * ProfileWidget renders the bottom-left avatar pill in the lateral nav.
 * Clicking opens a popover with editable name / email / bio plus a
 * sign-in / sign-out toggle. Data is persisted to localStorage via
 * `useProfile`; no backend auth is required.
 */
export function ProfileWidget() {
  const { profile, update, signIn, signOut } = useProfile();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const [draft, setDraft] = useState({ name: "", email: "", bio: "" });

  // Sync draft from store whenever the panel opens.
  useEffect(() => {
    if (open) {
      setDraft({
        name: profile.name,
        email: profile.email,
        bio: profile.bio,
      });
    }
  }, [open, profile.name, profile.email, profile.bio]);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const initials = profile.signedIn ? profileInitials(profile.name) : "?";

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        title={profile.signedIn ? `Signed in as ${profile.name || "user"}` : "Sign in"}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "grid h-10 w-10 place-items-center rounded-md border text-[11px] font-mono transition-colors",
          profile.signedIn
            ? "border-lime/40 bg-lime/10 text-lime hover:bg-lime/15"
            : "border-border text-muted-foreground hover:text-foreground hover:border-border hover:bg-surface",
        )}
      >
        {profile.signedIn ? (
          <span className="font-semibold tracking-wide">{initials}</span>
        ) : (
          <User className="h-[18px] w-[18px]" />
        )}
      </button>

      {open && (
        <div className="absolute bottom-0 left-12 z-50 w-72 rounded-md border border-border bg-popover/95 backdrop-blur-md shadow-xl">
          <div className="flex items-center gap-3 border-b border-border px-3 py-3">
            <div
              className={cn(
                "grid h-10 w-10 place-items-center rounded-md border font-mono text-sm font-semibold",
                profile.signedIn
                  ? "border-lime/40 bg-lime/10 text-lime"
                  : "border-border text-muted-foreground",
              )}
            >
              {initials}
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium truncate">
                {profile.signedIn && profile.name ? profile.name : "Anonymous Scout"}
              </div>
              <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                {profile.signedIn ? profile.email || "no email" : "not signed in"}
              </div>
            </div>
          </div>

          <div className="px-3 py-3 space-y-2.5">
            <Field
              label="Name"
              value={draft.name}
              onChange={(v) => setDraft((d) => ({ ...d, name: v }))}
              placeholder="Ada Lovelace"
            />
            <Field
              label="Email"
              value={draft.email}
              onChange={(v) => setDraft((d) => ({ ...d, email: v }))}
              placeholder="ada@scout.dev"
            />
            <div>
              <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                Bio
              </label>
              <textarea
                value={draft.bio}
                onChange={(e) => setDraft((d) => ({ ...d, bio: e.target.value }))}
                rows={3}
                placeholder="Senior backend engineer. Looking for staff roles."
                className="mt-1 w-full rounded-md border border-border bg-background/40 px-2 py-1.5 font-mono text-[11px] placeholder:text-muted-foreground/60 focus:outline-none focus:ring-1 focus:ring-indigo/60"
              />
            </div>
          </div>

          <div className="flex gap-2 border-t border-border px-3 py-2.5">
            {profile.signedIn ? (
              <>
                <button
                  type="button"
                  onClick={() => {
                    update({ name: draft.name, email: draft.email, bio: draft.bio });
                    setOpen(false);
                  }}
                  className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-md border border-indigo/50 bg-indigo/10 px-2 py-1.5 font-mono text-[11px] uppercase tracking-wider text-indigo hover:bg-indigo/20 transition-colors"
                >
                  Save
                </button>
                <button
                  type="button"
                  onClick={() => {
                    signOut();
                    setOpen(false);
                  }}
                  className="inline-flex items-center justify-center gap-1.5 rounded-md border border-border px-2 py-1.5 font-mono text-[11px] uppercase tracking-wider text-muted-foreground hover:text-destructive hover:border-destructive/40 hover:bg-destructive/10 transition-colors"
                >
                  <LogOut className="h-3 w-3" />
                  Sign Out
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={() => {
                  if (!draft.name.trim()) return;
                  signIn(draft.name, draft.email);
                  update({ bio: draft.bio });
                  setOpen(false);
                }}
                disabled={!draft.name.trim()}
                className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-md border border-lime/50 bg-lime/10 px-2 py-1.5 font-mono text-[11px] uppercase tracking-wider text-lime hover:bg-lime/20 transition-colors disabled:opacity-50"
              >
                <LogIn className="h-3 w-3" />
                Sign In
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        {label}
      </label>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="mt-1 w-full rounded-md border border-border bg-background/40 px-2 py-1.5 font-mono text-[11px] placeholder:text-muted-foreground/60 focus:outline-none focus:ring-1 focus:ring-indigo/60"
      />
    </div>
  );
}
