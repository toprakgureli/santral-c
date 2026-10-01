import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { cn } from "@/lib/utils";

// The panel's build tag, given by the deploy script through Vite env. The
// panel's files are public, so the tag is a random one the deploy also gives
// the backend, never the commit; the commit comes from the backend, which
// tells it only to signed-in users.
const FE = (import.meta.env.VITE_BUILD_ID as string | undefined) ?? "dev";

type Server = { version: string; build: string };

// VersionInfo shows the version the server runs and whether this open panel
// is the same build. A mismatch means the page is older than the deploy (or
// the deploy is stale) and a reload brings the latest.
export default function VersionInfo({ collapsed }: { collapsed: boolean }) {
  const [be, setBe] = useState<Server | null>(null);

  useEffect(() => {
    api
      .version()
      .then((v) => setBe({ version: v.version, build: v.build ?? v.version }))
      .catch(() => setBe({ version: "?", build: "?" }));
  }, []);

  const match = be !== null && be.build === FE;
  const label = be?.version ?? "...";
  if (collapsed) {
    return (
      <div className="mt-1 text-center" data-tip={`Sürüm ${label}${be && !match ? " · sayfayı yenile" : ""}`}>
        <span className={cn("inline-block size-1.5 rounded-full", match ? "bg-success" : "bg-warning")} />
      </div>
    );
  }
  return (
    <div className="mt-1 px-3 text-[0.625rem] leading-tight text-muted-foreground/70">
      <span className="font-mono">sürüm {label}</span>
      {be && !match && <span className="ml-1 text-warning">· sayfayı yenile</span>}
      {match && <span className="ml-1 text-success">· güncel</span>}
    </div>
  );
}
