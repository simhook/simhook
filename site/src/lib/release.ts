/**
 * The current phone app release, read at build time from the manifest the
 * app itself polls. A build with no network still produces a working page:
 * callers get null and say "latest release" instead of a version.
 */
const MANIFEST = "https://github.com/simhook/simhook/releases/latest/download/android.json";

export type Manifest = {
  version_name: string;
  version_code: number;
  sha256: string;
  size_bytes: number;
  released_at?: string;
  notes?: string | null;
};

export async function latestRelease(): Promise<Manifest | null> {
  try {
    const r = await fetch(MANIFEST, { redirect: "follow" });
    return r.ok ? ((await r.json()) as Manifest) : null;
  } catch {
    return null;
  }
}

export function sizeLabel(m: Manifest | null): string | null {
  return m ? `${(m.size_bytes / 1048576).toFixed(1)} MB` : null;
}
