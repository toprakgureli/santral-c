// WhatsAppReports: how the WhatsApp work went over a period. A few headline
// figures, the busiest hours of the day, and the same figures per person
// and per number.

import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, Clock3, MessageCircle, PhoneCall, Smartphone, Star, TimerReset, UsersRound, type LucideIcon } from "lucide-react";
import { ApiError } from "@/api/client";
import RangePicker, { useRange } from "@/components/RangePicker";
import { Card } from "@/components/ui";
import { IconChip, Toolbar, type ChipTone } from "@/components/ui/rows";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WACallSurveyReport, WAChannel, WAReport } from "@/whatsapp/types";
import { prettyPhone } from "@/whatsapp/util";

function dur(sec: number): string {
  if (!sec || sec <= 0) return "–";
  if (sec < 60) return `${Math.round(sec)} sn`;
  const m = Math.round(sec / 60);
  if (m < 60) return `${m} dk`;
  const h = Math.floor(m / 60);
  return m % 60 ? `${h} sa ${m % 60} dk` : `${h} sa`;
}

export function WhatsAppReports() {
  const { preset, range, choose, setFrom, setTo } = useRange("last7");
  const [channel, setChannel] = useState(0);
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [report, setReport] = useState<WAReport | null>(null);
  const [calls, setCalls] = useState<WACallSurveyReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [sort, setSort] = useState<"owned" | "resolved" | "avgFirstReplySec" | "avgRating">("owned");

  useEffect(() => {
    waApi.channels().then(setChannels).catch(() => setChannels([]));
  }, []);
  useEffect(() => {
    if (!range.from || !range.to) return;
    setLoading(true);
    setError(null);
    waApi.reports(range.from, range.to, channel).then(setReport).catch((e) => setError(e instanceof ApiError ? e.message : "Rapor alınamadı.")).finally(() => setLoading(false));
  }, [range.from, range.to, channel]);
  useEffect(() => {
    if (!range.from || !range.to) return;
    waApi.callSurveyReport(range.from, range.to).then(setCalls).catch(() => setCalls(null));
  }, [range.from, range.to]);

  const totals = useMemo(() => {
    const ch = report?.channels ?? [];
    const sum = (f: (c: (typeof ch)[number]) => number) => ch.reduce((a, c) => a + f(c), 0);
    const tickets = sum((c) => c.tickets);
    const weighted = (f: (c: (typeof ch)[number]) => number, w: (c: (typeof ch)[number]) => number) => {
      const tw = sum((c) => (f(c) > 0 ? w(c) : 0));
      return tw ? sum((c) => (f(c) > 0 ? f(c) * w(c) : 0)) / tw : 0;
    };
    return {
      tickets,
      resolved: sum((c) => c.resolved),
      botResolved: sum((c) => c.botResolved),
      inbound: sum((c) => c.inbound),
      outbound: sum((c) => c.outbound),
      failed: sum((c) => c.failed),
      waiting: sum((c) => c.waitingEntries),
      firstReply: weighted((c) => c.avgFirstReplySec, (c) => c.tickets),
      rating: weighted((c) => c.avgRating, (c) => c.resolved),
    };
  }, [report]);

  const agents = useMemo(() => {
    const list = [...(report?.agents ?? [])];
    list.sort((a, b) => (sort === "avgFirstReplySec" ? (a[sort] || 1e12) - (b[sort] || 1e12) : b[sort] - a[sort]));
    return list;
  }, [report, sort]);

  const hours = report?.hours ?? [];
  const peak = Math.max(1, ...hours);
  const busiest = hours.length ? hours.indexOf(Math.max(...hours)) : -1;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Link to="/whatsapp" data-tip="Gelen kutusuna dön" className="flex size-9 items-center justify-center rounded-xl bg-card text-muted-foreground shadow-sm ring-1 ring-border/60 hover:text-foreground"><ArrowLeft className="size-4" /></Link>
        <h1 className="text-lg font-semibold tracking-tight">WhatsApp raporları</h1>
      </div>
      <Toolbar>
        <RangePicker preset={preset} range={range} onPreset={choose} onFrom={setFrom} onTo={setTo} />
        {channels.length > 1 && (
          <select value={channel} onChange={(e) => setChannel(Number(e.target.value))} className="h-9 rounded-xl border border-border/60 bg-card px-3 text-sm">
            <option value={0}>Tüm numaralar</option>
            {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        )}
        {loading && <span className="text-xs text-muted-foreground">Yükleniyor...</span>}
      </Toolbar>
      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className={cn("grid gap-3 sm:grid-cols-2 xl:grid-cols-4", loading && "opacity-60")}>
        <Figure icon={MessageCircle} tone="primary" label="Sohbet" value={String(totals.tickets)} sub={`${totals.inbound} gelen, ${totals.outbound} giden mesaj`} />
        <Figure icon={TimerReset} tone="success" label="Çözülen" value={String(totals.resolved)} sub={totals.botResolved ? `${totals.botResolved} tanesi chatbot'ta çözüldü` : totals.tickets ? `%${Math.round((totals.resolved / totals.tickets) * 100)} oranında` : "–"} />
        <Figure icon={Clock3} tone={totals.firstReply > 900 ? "warning" : "violet"} label="İlk cevap süresi" value={dur(totals.firstReply)} sub={totals.waiting ? `${totals.waiting} kez cevap bekleyenlere düştü` : "Kimse uzun süre beklemedi"} />
        <Figure icon={Star} tone="warning" label="Memnuniyet" value={totals.rating ? totals.rating.toFixed(1) : "–"} sub={totals.rating ? "5 üzerinden ortalama" : "Henüz puan yok"} />
      </div>

      <Card title="Saatlere göre gelen mesaj" icon={Clock3}>
        {hours.every((h) => h === 0) ? (
          <p className="py-6 text-center text-sm text-muted-foreground">Bu aralıkta mesaj yok.</p>
        ) : (
          <>
            <div className="flex h-36 items-end gap-[3px]">
              {hours.map((v, h) => (
                <div key={h} className="group relative flex h-full flex-1 flex-col justify-end" data-tip={`${String(h).padStart(2, "0")}:00 · ${v} mesaj`}>
                  <div className={cn("w-full rounded-t-md transition-colors", h === busiest ? "bg-primary" : "bg-primary/30 group-hover:bg-primary/60")} style={{ height: `${Math.max(v ? 4 : 0, (v / peak) * 100)}%` }} />
                </div>
              ))}
            </div>
            <div className="mt-1 flex gap-[3px] text-center text-[0.6rem] text-muted-foreground tabular-nums">
              {hours.map((_, h) => <span key={h} className="flex-1">{h % 3 === 0 ? String(h).padStart(2, "0") : ""}</span>)}
            </div>
            {busiest >= 0 && <p className="mt-2 text-xs text-muted-foreground">En yoğun saat {String(busiest).padStart(2, "0")}:00 ile {String((busiest + 1) % 24).padStart(2, "0")}:00 arası.</p>}
          </>
        )}
      </Card>

      <Card title="Kişiler" icon={UsersRound} actions={
        <select value={sort} onChange={(e) => setSort(e.target.value as typeof sort)} className="h-8 rounded-lg border border-border/60 bg-card px-2 text-xs">
          <option value="owned">En çok sohbet alan</option>
          <option value="resolved">En çok çözen</option>
          <option value="avgFirstReplySec">En hızlı cevap veren</option>
          <option value="avgRating">En yüksek puan</option>
        </select>
      }>
        {agents.length === 0 ? <p className="py-4 text-center text-sm text-muted-foreground">Bu aralıkta kimse sohbet almadı.</p> : (
          <div className="-mx-5 overflow-x-auto">
            <table className="w-full min-w-[46rem] text-sm">
              <thead>
                <tr className="text-left text-[0.7rem] font-medium text-muted-foreground">
                  <th className="px-5 py-2 font-medium">Kişi</th>
                  <Th tip="Sahibi olduğu sohbet">Aldığı</Th>
                  <Th tip="Yardıma katıldığı sohbet">Yardım</Th>
                  <Th>Çözdüğü</Th>
                  <Th tip="Müşteriye yazdığı mesaj">Mesaj</Th>
                  <Th tip="Sohbeti aldıktan sonra ilk cevabına kadar geçen ortalama süre">İlk cevap</Th>
                  <Th tip="Sohbetin açılışından çözülmesine kadar ortalama süre">Çözme süresi</Th>
                  <Th tip="Sohbetlerinin cevap bekleyenlere düşme sayısı">Beklettiği</Th>
                  <Th>Puan</Th>
                </tr>
              </thead>
              <tbody>
                {agents.map((a) => (
                  <tr key={a.user.id} className="border-t border-border/50">
                    <td className="px-5 py-2">
                      <span className="flex items-center gap-2.5">
                        <UserAvatar userId={a.user.id} name={a.user.name} hasAvatar={a.user.hasAvatar} version={a.user.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                        <span className="truncate font-medium">{a.user.name}</span>
                      </span>
                    </td>
                    <Td>{a.owned}</Td>
                    <Td>{a.helped}</Td>
                    <Td>{a.resolved}</Td>
                    <Td>{a.messages}</Td>
                    <Td className={cn(a.avgFirstReplySec > 900 && "text-warning")}>{dur(a.avgFirstReplySec)}</Td>
                    <Td>{dur(a.avgResolveSec)}</Td>
                    <Td className={cn(a.waitingEntries > 0 && "text-warning")}>{a.waitingEntries}</Td>
                    <Td>{a.ratings ? <span className="inline-flex items-center gap-1"><Star className="size-3 fill-warning text-warning" /> {a.avgRating.toFixed(1)} <span className="text-[0.65rem] text-muted-foreground">({a.ratings})</span></span> : "–"}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {calls && (calls.sent > 0 || calls.queued > 0 || calls.failed > 0) && <CallSurveyCard r={calls} />}

      {(report?.channels.length ?? 0) > 1 && (
        <Card title="Numaralar" icon={Smartphone}>
          <div className="-mx-5 overflow-x-auto">
            <table className="w-full min-w-[40rem] text-sm">
              <thead>
                <tr className="text-left text-[0.7rem] text-muted-foreground">
                  <th className="px-5 py-2 font-medium">Numara</th>
                  <Th>Sohbet</Th>
                  <Th>Çözülen</Th>
                  <Th>Chatbot'ta</Th>
                  <Th>Gelen</Th>
                  <Th>Giden</Th>
                  <Th tip="Meta'nın kabul etmediği mesaj">Gitmeyen</Th>
                  <Th>İlk cevap</Th>
                  <Th>Puan</Th>
                </tr>
              </thead>
              <tbody>
                {report!.channels.map((c) => (
                  <tr key={c.id} className="border-t border-border/50">
                    <td className="px-5 py-2 font-medium">{c.name}</td>
                    <Td>{c.tickets}</Td>
                    <Td>{c.resolved}</Td>
                    <Td>{c.botResolved}</Td>
                    <Td>{c.inbound}</Td>
                    <Td>{c.outbound}</Td>
                    <Td className={cn(c.failed > 0 && "text-destructive")}>{c.failed}</Td>
                    <Td>{dur(c.avgFirstReplySec)}</Td>
                    <Td>{c.avgRating ? c.avgRating.toFixed(1) : "–"}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}

function CallSurveyCard({ r }: { r: WACallSurveyReport }) {
  const rate = r.sent > 0 ? Math.round((r.answered / r.sent) * 100) : 0;
  return (
    <Card title="Çağrı sonrası anket" icon={PhoneCall}>
      <div className="mb-4 flex flex-wrap gap-2 text-sm">
        <Pill label="Gönderilen" value={String(r.sent)} />
        <Pill label="Cevaplayan" value={`${r.answered}${r.sent ? ` · %${rate}` : ""}`} />
        <Pill label="Ortalama puan" value={r.average ? r.average.toFixed(1) : "–"} tone={r.average && r.average < 3 ? "text-destructive" : "text-foreground"} />
        {r.failed > 0 && <Pill label="Gidemeyen" value={String(r.failed)} tone="text-destructive" />}
        {r.skipped > 0 && <Pill label="Atlanan" value={String(r.skipped)} hint="Yakında zaten sorulmuş ya da müşteri mesaj istemiyor" />}
        {r.queued > 0 && <Pill label="Sırada" value={String(r.queued)} />}
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <div>
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">Temsilcilere göre</p>
          {r.agents.length === 0 ? <p className="py-3 text-sm text-muted-foreground">Henüz anket yok.</p> : (
            <div className="divide-y divide-border/50">
              {r.agents.map((a) => (
                <div key={a.user.id} className="flex items-center gap-2.5 py-2">
                  <UserAvatar userId={a.user.id} name={a.user.name} hasAvatar={a.user.hasAvatar} version={a.user.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{a.user.name}</span>
                  <span className="text-xs text-muted-foreground tabular-nums">{a.answered}/{a.sent} cevap</span>
                  {a.low > 0 && <span className="rounded-full bg-destructive/10 px-1.5 py-px text-[0.65rem] font-semibold text-destructive" data-tip="2 ve altı puan">{a.low} düşük</span>}
                  <span className="flex w-12 items-center justify-end gap-1 text-sm font-semibold tabular-nums"><Star className="size-3 fill-warning text-warning" />{a.average ? a.average.toFixed(1) : "–"}</span>
                </div>
              ))}
            </div>
          )}
        </div>
        <div>
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">Son cevaplar</p>
          {r.recent.length === 0 ? <p className="py-3 text-sm text-muted-foreground">Henüz cevap yok.</p> : (
            <div className="max-h-80 space-y-1.5 overflow-y-auto">
              {r.recent.map((a) => (
                <div key={a.id} className="flex items-start gap-2.5 rounded-xl bg-muted/30 px-3 py-2">
                  <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg text-sm font-bold tabular-nums", a.score >= 4 ? "bg-success/12 text-success" : a.score === 3 ? "bg-warning/12 text-warning" : "bg-destructive/10 text-destructive")}>{a.score}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm"><span className="font-medium">{a.agent || "?"}</span> <span className="font-mono text-xs text-muted-foreground">{prettyPhone(a.phone)}</span></span>
                    {a.comment && <span className="block text-xs text-muted-foreground">{a.comment}</span>}
                    <span className="block text-[0.65rem] text-muted-foreground">{new Date(a.answeredAt).toLocaleString("tr-TR", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}</span>
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

function Pill({ label, value, tone, hint }: { label: string; value: string; tone?: string; hint?: string }) {
  return (
    <span className="flex items-center gap-1.5 rounded-full bg-muted/50 px-3 py-1" data-tip={hint}>
      <span className="text-xs text-muted-foreground">{label}</span>
      <b className={cn("tabular-nums", tone)}>{value}</b>
    </span>
  );
}

function Figure({ icon, tone, label, value, sub }: { icon: LucideIcon; tone: ChipTone; label: string; value: string; sub: string }) {
  return (
    <div className="flex items-start gap-3 rounded-2xl bg-card p-4 shadow-sm ring-1 ring-border/60">
      <IconChip icon={icon} tone={tone} size="lg" />
      <div className="min-w-0">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="text-2xl font-semibold tracking-tight tabular-nums">{value}</p>
        <p className="truncate text-[0.7rem] text-muted-foreground">{sub}</p>
      </div>
    </div>
  );
}

function Th({ tip, children }: { tip?: string; children: React.ReactNode }) {
  return <th className="px-3 py-2 text-right font-medium" data-tip={tip}>{children}</th>;
}

function Td({ className, children }: { className?: string; children: React.ReactNode }) {
  return <td className={cn("px-3 py-2 text-right tabular-nums", className)}>{children}</td>;
}
