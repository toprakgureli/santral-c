// Preview: a development-only page that draws the shared pieces with
// sample data, so layout can be checked without a backend or a call.
// Served at /__preview by the dev server; never part of the build's routes.

import { useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { History, PhoneIncoming, PhoneOutgoing, Tags } from "lucide-react";
import type { TeamRow, User } from "@/api/types";
import { AuthMockProvider } from "@/auth/AuthContext";
import DailyBars from "@/components/charts/DailyBars";
import CallBar from "@/components/layout/CallBar";
import Sidebar from "@/components/layout/Sidebar";
import VoiceMixer from "@/components/layout/VoiceMixer";
import { Badge, Card } from "@/components/ui";
import { IconChip, ListRow, Toolbar } from "@/components/ui/rows";
import RangePicker, { useRange } from "@/components/RangePicker";
import { AgentCard } from "@/pages/TeamPerformance";
import { SoftphoneMockProvider, type SoftphoneValue } from "@/softphone/SoftphoneContext";
import { TeamsMockProvider } from "@/teams/TeamsContext";

const user: User = { id: 1, name: "Toprak Şahin Güreli", email: "toprak@example.com", active: true, roles: ["Yönetici"], roleIds: [1], permissions: ["cdr.view_all", "call.view_all", "call.originate", "teams.view", "performance.view_all", "escalation.list_all", "escalation.manage", "user.view", "role.view", "system.settings", "system.audit_view"], mfaEnabled: false, mustChangePassword: false, sipExtension: "1001", createdAt: "2026-01-01T00:00:00Z" };

// A fake spectrum so the bars move.
function fakeSpectrum(seed: number) {
  const bins = new Uint8Array(128);
  const t = Date.now() / 300;
  for (let i = 0; i < bins.length; i++) bins[i] = Math.max(0, Math.min(255, 140 * Math.abs(Math.sin(t + i / 4 + seed)) * (1 - i / 128) + 40 * Math.random()));
  return bins;
}

function phoneValue(over: Partial<SoftphoneValue>): SoftphoneValue {
  const noop = async () => undefined;
  return {
    status: "in-call",
    lastEnded: null,
    callId: "c1",
    lastPeer: "05304230113",
    lastUnreached: null,
    extension: "1001",
    error: null,
    muted: false,
    held: false,
    peer: "05304230113",
    endReason: null,
    callStartedAt: Date.now() - 65000,
    answeredAt: Date.now() - 60000,
    audioRef: { current: null },
    remoteGain: 1.2,
    setRemoteGain: () => undefined,
    wave: () => null,
    spectrum: (side) => fakeSpectrum(side === "remote" ? 0 : 2),
    call: noop,
    answer: noop,
    hangup: noop,
    toggleMute: () => undefined,
    toggleHold: noop,
    transfer: noop,
    sendDtmf: () => undefined,
    secondary: false,
    takeOver: () => undefined,
    ...over,
  };
}

function row(over: Partial<TeamRow> & { status: TeamRow["status"] }): TeamRow {
  const now = new Date();
  return {
    userId: 1,
    name: "Ahmet Yılmaz",
    extension: "1002",
    roles: ["Teknik Destek"],
    since: new Date(now.getTime() - 12 * 60000).toISOString(),
    shift: { startedAt: new Date(now.getTime() - 4 * 3600000).toISOString(), firstStart: new Date(now.getTime() - 4 * 3600000).toISOString(), open: true, seconds: 4 * 3600 },
    calls: { total: 31, answered: 24, short: 3, long: 21, unanswered: 7, inbound: 20, outbound: 11, inboundMissed: 5, outboundMissed: 2, inboundReal: 14, outboundReal: 7, talkSeconds: 5400, avgTalkSeconds: 257, longestSeconds: 1310, over5: 6, over10: 3, over20: 1, peers: 18, avgAnswerSeconds: 6 },
    escalations: 2,
    breakSeconds: 900,
    recent: [{ peer: "05304230113", peerName: "Mehmet Demir", direction: "inbound", disposition: "answered", startedAt: new Date(now.getTime() - 20 * 60000).toISOString(), durationSeconds: 312 }],
    ...over,
  };
}

const rows: TeamRow[] = [
  row({ status: "talking", call: { peer: "05304230113", peerName: "Mehmet Demir", direction: "inbound", startedAt: new Date(Date.now() - 95000).toISOString() } }),
  row({ status: "available", userId: 2, name: "Zeynep Kaya Uzunsoyadlıkişi", extension: "1003", roles: ["Satış", "Teknik Destek"] }),
  row({ status: "break", userId: 3, name: "Mehmet Öz", extension: "1004", escalations: 0, breakSeconds: 0 }),
  row({ status: "off", userId: 4, name: "Elif Su", extension: "1005", shift: { open: false, seconds: 0 }, calls: { total: 0, answered: 0, short: 0, long: 0, unanswered: 0, inbound: 0, outbound: 0, inboundMissed: 0, outboundMissed: 0, inboundReal: 0, outboundReal: 0, talkSeconds: 0, avgTalkSeconds: 0, longestSeconds: 0, over5: 0, over10: 0, over20: 0, peers: 0, avgAnswerSeconds: 0 }, recent: [] }),
];

const today = new Date();
const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">{title}</h2>
      {children}
    </section>
  );
}

export default function Preview() {
  const [collapsed, setCollapsed] = useState(true);
  const range = useRange("last7");
  const phone = phoneValue({});
  return (
    <MemoryRouter>
      <AuthMockProvider user={user}>
        <TeamsMockProvider>
          <SoftphoneMockProvider value={phone}>
            <div className="min-h-svh bg-background text-foreground">
              <Sidebar open collapsed={collapsed} onNavigate={() => undefined} onClose={() => undefined} onToggleCollapse={() => setCollapsed((v) => !v)} />
              <main className={collapsed ? "space-y-8 p-6 lg:pl-[calc(4.75rem+1.5rem)]" : "space-y-8 p-6 lg:pl-[calc(16.5rem+1.5rem)]"}>
                <Section title="Ses paneli · küçük (çağrı çubuğu genişliği 288px) ve büyük (ana sayfa, max-w-sm)">
                  <div className="flex flex-wrap items-start gap-6">
                    <div className="w-72 rounded-2xl border border-border bg-popover p-3 shadow-xl"><VoiceMixer /></div>
                    <div className="w-full max-w-sm rounded-2xl bg-card p-4 ring-1 ring-border/60"><VoiceMixer size="large" /></div>
                  </div>
                </Section>

                <Section title="Ekip kartları">
                  <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                    {rows.map((r) => (
                      <AgentCard key={r.userId} row={r} now={Date.now()} live multiDay={false} from={ymd(today)} to={ymd(today)} />
                    ))}
                  </div>
                </Section>

                <Section title="Satırlar, çipler, rozetler, filtre şeridi">
                  <Card title="Çağrılar" icon={History} actions={<span className="text-xs text-muted-foreground">3 kayıt</span>}>
                    <Toolbar className="mb-3">
                      <RangePicker preset={range.preset} range={range.range} onPreset={range.choose} onFrom={range.setFrom} onTo={range.setTo} />
                    </Toolbar>
                    <div className="space-y-1">
                      <ListRow icon={PhoneIncoming} tone="success" title={<span className="font-mono">05304230113 → 1001</span>} sub="Gelen · Cevaplandı · 25.09 14:12" trailing={<span className="font-mono text-sm">03:12</span>} />
                      <ListRow icon={PhoneOutgoing} tone="destructive" title={<span className="font-mono">1001 → 05551234567</span>} sub="Giden · Cevapsız · 25.09 13:58" trailing={<span className="font-mono text-sm">—</span>} onClick={() => undefined} />
                      <ListRow icon={Tags} tone="warning" title="Fatura itirazı" sub={<span>Ahmet Yılmaz · <Badge tone="amber">Faturalama</Badge></span>} trailing={<span className="text-xs text-muted-foreground">25.09.2026 12:03</span>} active>
                        <p className="pl-11 text-sm text-foreground/80">Kendiliğinden kaydedildi · 4 dk 12 sn görüşme</p>
                      </ListRow>
                    </div>
                    <div className="mt-4 flex flex-wrap items-center gap-2">
                      {(["muted", "primary", "success", "warning", "destructive", "violet", "solid"] as const).map((t) => <IconChip key={t} icon={Tags} tone={t} />)}
                      {(["slate", "green", "red", "amber", "blue", "violet"] as const).map((t) => <Badge key={t} tone={t}>{t}</Badge>)}
                    </div>
                  </Card>
                </Section>

                <Section title="Günlük grafik">
                  <div className="max-w-2xl rounded-xl border border-border/60 bg-muted/25 p-3.5">
                    <DailyBars points={Array.from({ length: 14 }).map((_, i) => ({ day: ymd(new Date(today.getTime() - (13 - i) * 86400000)), value: [4, 9, 12, 0, 7, 15, 11, 3, 8, 13, 6, 0, 10, 5][i] }))} />
                  </div>
                </Section>
              </main>
              <CallBar />
            </div>
          </SoftphoneMockProvider>
        </TeamsMockProvider>
      </AuthMockProvider>
    </MemoryRouter>
  );
}
