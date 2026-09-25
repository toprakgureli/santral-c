// Audit trail: who did what, when, from where. Read-only; needs
// system.audit_view.

import { useEffect, useRef, useState } from "react";
import { FileClock, ScrollText, Search } from "lucide-react";
import { ListRow, Toolbar } from "../components/ui/rows";
import { api } from "../api/client";
import type { AuditEntry } from "../api/types";
import { Badge, Card, EmptyState, Input, Pagination, Select, Skeleton } from "../components/ui";
import { cn, formatDateTime } from "../lib/utils";

const PER_PAGE = 50;

const ACTION_LABELS: Record<string, string> = {
  "user.created": "Kullanıcı oluşturuldu",
  "user.updated": "Kullanıcı güncellendi",
  "user.activated": "Kullanıcı aktifleştirildi",
  "user.deactivated": "Kullanıcı pasife alındı",
  "user.password_reset": "Şifre sıfırlandı",
  "user.roles_updated": "Roller değiştirildi",
  "role.created": "Rol oluşturuldu",
  "role.updated": "Rol güncellendi",
  "role.deleted": "Rol silindi",
  "contact.created": "Kişi oluşturuldu",
  "contact.updated": "Kişi güncellendi",
  "contact.deleted": "Kişi silindi",
  "contact.phone_added": "Kişiye numara eklendi",
  "contact.phone_removed": "Kişiden numara silindi",
  "settings.updated": "Sistem ayarı değişti",
  "security.ip_unbanned": "IP banı kaldırıldı",
  "shift.started": "Mesai başlatıldı",
  "shift.ended": "Mesai bitirildi",
};

const MODULES: { key: string; label: string }[] = [
  { key: "", label: "Tüm işlemler" },
  { key: "user.", label: "Kullanıcı" },
  { key: "role.", label: "Rol" },
  { key: "contact.", label: "Kişiler" },
  { key: "settings.", label: "Ayarlar" },
  { key: "security.", label: "Güvenlik" },
  { key: "shift.", label: "Mesai" },
];

const TARGET_LABELS: Record<string, string> = {
  user: "Kullanıcı",
  role: "Rol",
  contact: "Kişi",
  settings: "Ayar",
  ip_ban: "IP banı",
  shift: "Mesai",
};

function tone(action: string): "slate" | "green" | "red" | "amber" | "blue" {
  if (action.endsWith(".deleted") || action.endsWith(".deactivated")) return "red";
  if (action.endsWith(".created") || action.endsWith(".activated")) return "green";
  if (action.startsWith("settings.") || action.startsWith("security.")) return "amber";
  return "blue";
}

export function Audit() {
  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);

  useEffect(() => {
    const id = ++seq.current;
    setLoading(true);
    const timer = window.setTimeout(() => {
      api
        .auditLogs({ page, perPage: PER_PAGE, action: action || undefined, query: query.trim() || undefined })
        .then((r) => {
          if (id !== seq.current) return;
          setItems(r.items);
          setTotal(r.total);
        })
        .catch(() => {
          if (id !== seq.current) return;
          setItems([]);
          setTotal(0);
        })
        .finally(() => {
          if (id === seq.current) setLoading(false);
        });
    }, query ? 350 : 0);
    return () => window.clearTimeout(timer);
  }, [page, action, query]);

  return (
    <Card title="Denetim Kayıtları" icon={FileClock} actions={<span className="text-xs text-muted-foreground">{total} kayıt</span>}>
      <Toolbar className="mb-4">
        <Select value={action} onChange={(e) => { setAction(e.target.value); setPage(1); }} className="w-auto min-w-40">
          {MODULES.map((m) => (
            <option key={m.key} value={m.key}>{m.label}</option>
          ))}
        </Select>
        <div className="relative ml-auto w-full sm:w-72">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={query} onChange={(e) => { setQuery(e.target.value); setPage(1); }} placeholder="Kullanıcı, e-posta, hedef no veya IP" className="pl-9" />
        </div>
      </Toolbar>

      {loading && items.length === 0 ? (
        <div className="space-y-2">
          {[0, 1, 2, 3, 4].map((row) => (
            <Skeleton key={row} className="h-12 w-full rounded-xl" />
          ))}
        </div>
      ) : !items.length ? (
        <EmptyState icon={<ScrollText />} title="Kayıt bulunamadı" description="Filtreleri değiştirip tekrar deneyin." />
      ) : (
        <div className={cn("space-y-1 transition-opacity", loading && "opacity-60")}>
          {items.map((e) => {
            const detail = JSON.stringify(e.detail ?? {});
            const t = tone(e.action);
            return (
              <ListRow
                key={e.id}
                icon={ScrollText}
                tone={t === "red" ? "destructive" : t === "green" ? "success" : t === "amber" ? "warning" : "primary"}
                title={<span className="flex items-center gap-2"><Badge tone={t}>{ACTION_LABELS[e.action] ?? e.action}</Badge><span>{e.actorName || "Sistem"}</span></span>}
                sub={
                  <span>
                    {e.actorEmail && <span>{e.actorEmail} · </span>}
                    {(TARGET_LABELS[e.targetType] ?? e.targetType) || "—"}
                    {e.targetId && <span className="tabular-nums"> #{e.targetId}</span>}
                    {e.ip && <span className="tabular-nums"> · {e.ip}</span>}
                  </span>
                }
                trailing={<span className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">{formatDateTime(e.createdAt)}</span>}
              >
                {detail !== "{}" && (
                  <details className="group pl-11">
                    <summary className="max-w-xl cursor-pointer truncate font-mono text-[0.6875rem] text-muted-foreground group-open:whitespace-normal">{detail}</summary>
                    <pre className="mt-1 max-w-xl overflow-x-auto rounded-xl bg-muted/60 p-2 font-mono text-[0.6875rem] leading-relaxed whitespace-pre-wrap">{JSON.stringify(e.detail, null, 2)}</pre>
                  </details>
                )}
              </ListRow>
            );
          })}
        </div>
      )}

      <Pagination page={page} perPage={PER_PAGE} total={total} onChange={setPage} />
    </Card>
  );
}
