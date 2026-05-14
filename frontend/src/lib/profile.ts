// Lightweight client-side profile store. The project has no auth backend
// so we just persist to localStorage — enough for the user to keep their
// name, bio, niche shortcuts, and a "signed-in" flag across sessions.
import { useEffect, useState, useCallback } from "react";

const KEY = "scout:profile";

export interface ProfileData {
  signedIn: boolean;
  name: string;
  email: string;
  bio: string;
  primaryNicheId?: string;
  links: { label: string; url: string }[];
}

const EMPTY: ProfileData = {
  signedIn: false,
  name: "",
  email: "",
  bio: "",
  links: [],
};

function read(): ProfileData {
  if (typeof window === "undefined") return EMPTY;
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return EMPTY;
    const parsed = JSON.parse(raw) as Partial<ProfileData>;
    return { ...EMPTY, ...parsed, links: parsed.links ?? [] };
  } catch {
    return EMPTY;
  }
}

function write(p: ProfileData) {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(p));
    window.dispatchEvent(new StorageEvent("storage", { key: KEY }));
  } catch {
    /* ignore */
  }
}

export function useProfile() {
  const [profile, setProfile] = useState<ProfileData>(EMPTY);

  useEffect(() => {
    setProfile(read());
    const onStorage = (e: StorageEvent) => {
      if (e.key === KEY || e.key === null) setProfile(read());
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const update = useCallback((patch: Partial<ProfileData>) => {
    const next = { ...read(), ...patch };
    write(next);
    setProfile(next);
  }, []);

  const signIn = useCallback(
    (name: string, email: string) => {
      update({ signedIn: true, name: name.trim(), email: email.trim() });
    },
    [update],
  );

  const signOut = useCallback(() => {
    write(EMPTY);
    setProfile(EMPTY);
  }, []);

  return { profile, update, signIn, signOut };
}

export function profileInitials(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return "?";
  const parts = trimmed.split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";
}
