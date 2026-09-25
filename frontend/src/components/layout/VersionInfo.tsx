import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { cn } from "@/lib/utils";

// Frontend build stamp, injected by the deploy script via Vite env.
const FE = (import.meta.env.VITE_BUILD_SHA as string | undefined) ?? "dev";

// VersionInfo shows the running frontend and backend build so it is obvious
// whether the latest deploy is live. They should match each other (and the
// latest commit); a mismatch or a "dev" backend means the deploy is stale.
export default function VersionInfo({ collapsed }: { collapsed: boolean }) {
  const [be, setBe] = useState<string | null>(null);

  useEffect(() => {
    api.version().then((v) => setBe(v.version)).catch(() => setBe("?"));
  }, []);

  const match = be && be === FE;
  if (collapsed) {
    return (
      <div className="mt-1 text-center" data-tip={`Arayüz ${FE} · Sunucu ${be ?? "..."}`}>
        <span className={cn("inline-block size-1.5 rounded-full", match ? "bg-success" : "bg-warning")} />
      </div>
    );
  }
  return (
    <div className="mt-1 px-3 text-[0.625rem] leading-tight text-muted-foreground/70">
      <span className="font-mono">sürüm {FE}</span>
      {be && be !== FE && <span className="ml-1 text-warning">· sunucu {be}</span>}
      {match && <span className="ml-1 text-success">· güncel</span>}
    </div>
  );
}
