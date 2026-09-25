// RoomPanel: the right-hand side of a room. Three views share the same
// frame in the sidebar's style: the members list, search within the room,
// and the shared pictures, videos and files.

import { useEffect, useMemo, useState } from "react";
import { Download, FileText, Play, Search, X } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { TeamsGroupDetail, TeamsMediaItem, TeamsMessage } from "@/api/types";
import Lightbox from "@/components/teams/Lightbox";
import MembersPanel from "@/components/teams/MembersPanel";
import UserAvatar from "@/components/ui/UserAvatar";
import { attachmentUrl, extensionOf, formatDuration, formatSize, thumbUrl } from "@/lib/attachments";
import { stripMarkup } from "@/lib/markup";
import { cn } from "@/lib/utils";

export type PanelMode = "members" | "search" | "media";

export default function RoomPanel({
  mode,
  group,
  selfId,
  onChanged,
  onAdd,
  onInvite,
  onJump,
  onClose,
}: {
  mode: PanelMode;
  group: TeamsGroupDetail;
  selfId: number;
  onChanged: (g: TeamsGroupDetail) => void;
  onAdd: () => void;
  onInvite: () => void;
  onJump: (messageId: number) => void;
  onClose: () => void;
}) {
  if (mode === "members") return <MembersPanel group={group} selfId={selfId} onChanged={onChanged} onAdd={onAdd} onInvite={onInvite} />;
  return (
    <aside className="flex h-full w-72 shrink-0 flex-col border-l border-sidebar-border bg-sidebar">
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-sidebar-border px-4">
        <span className="text-sm font-semibold tracking-tight">{mode === "search" ? "Mesajlarda ara" : "Paylaşılanlar"}</span>
        <button type="button" onClick={onClose} aria-label="Kapat" className="ml-auto flex size-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-sidebar-accent hover:text-foreground" data-tip="Kapat"><X className="size-4" /></button>
      </div>
      {mode === "search" ? <SearchView group={group} onJump={onJump} /> : <MediaView group={group} onJump={onJump} />}
    </aside>
  );
}

function when(iso: string) {
  const d = new Date(iso);
  const today = d.toDateString() === new Date().toDateString();
  return today ? d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" }) : d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit", year: "2-digit" });
}

function SearchView({ group, onJump }: { group: TeamsGroupDetail; onJump: (id: number) => void }) {
  const [q, setQ] = useState("");
  const [items, setItems] = useState<TeamsMessage[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const needle = q.trim();
    if (needle.length < 2) {
      setItems(null);
      return;
    }
    const t = window.setTimeout(() => {
      setBusy(true);
      api
        .teamsSearch(group.id, needle)
        .then((r) => {
          setItems(r);
          setError(null);
        })
        .catch((e) => setError(e instanceof ApiError ? e.message : "Arama yapılamadı."))
        .finally(() => setBusy(false));
    }, 300);
    return () => window.clearTimeout(t);
  }, [q, group.id]);

  const words = useMemo(() => q.trim().split(/\s+/).filter(Boolean), [q]);
  const mark = (text: string) => {
    if (!words.length) return text;
    const re = new RegExp(`(${words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|")})`, "giu");
    return text.split(re).map((part, i) => (re.test(part) ? <mark key={i} className="rounded bg-warning/40 px-0.5 text-foreground">{part}</mark> : <span key={i}>{part}</span>));
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b border-sidebar-border p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Kelime yaz..."
            className="h-9 w-full rounded-lg bg-muted/50 pl-8 pr-2 text-sm outline-none placeholder:text-muted-foreground/60 focus:bg-card focus:ring-2 focus:ring-ring/20"
          />
        </div>
        <p className="mt-1.5 text-[0.65rem] text-muted-foreground">{items ? `${items.length} sonuç${items.length === 50 ? " (ilk 50)" : ""}` : "En az iki karakter, birden çok kelime hepsini arar."}</p>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {error && <p className="px-2 text-xs text-destructive">{error}</p>}
        {busy && !items && <p className="px-2 py-3 text-xs text-muted-foreground">Aranıyor...</p>}
        {items?.length === 0 && <p className="px-2 py-6 text-center text-xs text-muted-foreground">Eşleşen mesaj yok.</p>}
        {items?.map((m) => (
          <button key={m.id} type="button" onClick={() => onJump(m.id)} className="mb-0.5 flex w-full items-start gap-2.5 rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-sidebar-accent/60">
            {m.sender && <UserAvatar userId={m.sender.id} name={m.sender.name} hasAvatar={m.sender.hasAvatar} version={m.sender.avatarVersion} className="mt-0.5 size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />}
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-2">
                <span className="truncate text-xs font-medium">{m.sender?.name}</span>
                <span className="ml-auto shrink-0 text-[0.65rem] tabular-nums text-muted-foreground">{when(m.createdAt)}</span>
              </span>
              <span className="line-clamp-2 text-xs text-muted-foreground">{mark(stripMarkup(m.body))}</span>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

const TABS: { key: "image" | "video" | "file"; label: string }[] = [
  { key: "image", label: "Görseller" },
  { key: "video", label: "Videolar" },
  { key: "file", label: "Dosyalar" },
];

function MediaView({ group, onJump }: { group: TeamsGroupDetail; onJump: (id: number) => void }) {
  const [tab, setTab] = useState<"image" | "video" | "file">("image");
  const [items, setItems] = useState<TeamsMediaItem[]>([]);
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [gallery, setGallery] = useState<number | null>(null);

  const load = (before?: number) => {
    setBusy(true);
    api
      .teamsMedia(group.id, tab, before)
      .then((r) => {
        setItems((cur) => (before ? [...cur, ...r.items] : r.items));
        setMore(r.more);
        setError(null);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Liste alınamadı."))
      .finally(() => setBusy(false));
  };

  useEffect(() => {
    setItems([]);
    setGallery(null);
    load();
  }, [group.id, tab]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex gap-1 border-b border-sidebar-border p-2">
        {TABS.map((t) => (
          <button key={t.key} type="button" onClick={() => setTab(t.key)} className={cn("flex-1 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors", tab === t.key ? "bg-sidebar-accent text-sidebar-accent-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60")}>
            {t.label}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {error && <p className="px-2 text-xs text-destructive">{error}</p>}
        {!busy && items.length === 0 && <p className="px-2 py-8 text-center text-xs text-muted-foreground">{tab === "image" ? "Henüz görsel paylaşılmamış." : tab === "video" ? "Henüz video paylaşılmamış." : "Henüz dosya paylaşılmamış."}</p>}
        {tab !== "file" ? (
          <div className="grid grid-cols-3 gap-1">
            {items.map((a, i) => (
              <button key={a.id} type="button" onClick={() => setGallery(i)} onContextMenu={(e) => { e.preventDefault(); onJump(a.messageId); }} data-tip={`${a.name} · ${formatSize(a.size)} · ${a.sender} · ${when(a.createdAt)}\nSağ tık: mesaja git`} className="group relative aspect-square overflow-hidden rounded-lg bg-muted/60">
                {thumbUrl(a) ? <img src={thumbUrl(a) ?? undefined} alt={a.name} loading="lazy" className="size-full object-cover transition-transform group-hover:scale-105" /> : <span className="flex size-full items-center justify-center text-muted-foreground"><Play className="size-5" /></span>}
                {a.kind === "video" && <span className="pointer-events-none absolute inset-0 flex items-center justify-center"><span className="flex size-7 items-center justify-center rounded-full bg-black/55"><Play className="size-3 fill-white text-white" /></span></span>}
                {a.kind === "video" && a.durationMs ? <span className="pointer-events-none absolute right-1 bottom-1 rounded bg-black/65 px-1 text-[0.6rem] text-white tabular-nums">{formatDuration(a.durationMs)}</span> : null}
              </button>
            ))}
          </div>
        ) : (
          <ul className="space-y-1">
            {items.map((a) => (
              <li key={a.id} className="flex items-center gap-2.5 rounded-xl px-2 py-1.5 hover:bg-sidebar-accent/60">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><FileText className="size-4" /></span>
                <button type="button" onClick={() => onJump(a.messageId)} className="min-w-0 flex-1 text-left" data-tip="Mesaja git">
                  <span className="block truncate text-xs font-medium">{a.name}</span>
                  <span className="block truncate text-[0.65rem] text-muted-foreground">{extensionOf(a.name, a.mime)} · {formatSize(a.size)} · {a.sender} · {when(a.createdAt)}</span>
                </button>
                <a href={attachmentUrl(a.id, true)} download={a.name} data-tip="İndir" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><Download className="size-4" /></a>
              </li>
            ))}
          </ul>
        )}
        {more && (
          <div className="mt-2 text-center">
            <button type="button" disabled={busy} onClick={() => load(items[items.length - 1]?.id)} className="rounded-full border border-border/70 px-3 py-1 text-xs text-muted-foreground hover:bg-accent disabled:opacity-50">Daha fazla</button>
          </div>
        )}
      </div>
      {gallery !== null && <Lightbox items={items} index={gallery} onIndex={setGallery} onClose={() => setGallery(null)} />}
    </div>
  );
}
