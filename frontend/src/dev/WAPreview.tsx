// WAPreview: a development-only page that runs the WhatsApp screens against
// sample answers instead of the server, so their layout can be checked
// without a backend or a real number. Served at /__wa by the dev server.

import { useState } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { User } from "@/api/types";
import { AuthMockProvider } from "@/auth/AuthContext";
import WAAlerts from "@/components/whatsapp/WAAlerts";
import { WhatsApp } from "@/pages/WhatsApp";
import { WhatsAppBot } from "@/pages/WhatsAppBot";
import { WhatsAppCallbacks } from "@/pages/WhatsAppCallbacks";
import { WhatsAppReports } from "@/pages/WhatsAppReports";
import { WhatsAppSettings } from "@/pages/WhatsAppSettings";
import { TeamPerformance } from "@/pages/TeamPerformance";
import { Preferences } from "@/pages/Preferences";
import { SoftphoneMockProvider, type SoftphoneValue } from "@/softphone/SoftphoneContext";
import { TeamsMockProvider } from "@/teams/TeamsContext";
import { WhatsAppProvider } from "@/whatsapp/WhatsAppContext";

const WA_PERMS = ["whatsapp.view", "whatsapp.view_team", "whatsapp.view_all", "whatsapp.reply", "whatsapp.note", "whatsapp.pool", "whatsapp.waiting", "whatsapp.take", "whatsapp.assign", "whatsapp.resolve", "whatsapp.template_send", "whatsapp.template_manage", "whatsapp.quick_reply_manage", "whatsapp.automation_manage", "whatsapp.bot_manage", "whatsapp.bot_publish", "whatsapp.channel_manage", "whatsapp.setting_read_receipts", "whatsapp.setting_greeting", "whatsapp.setting_distribution", "whatsapp.setting_general", "whatsapp.team_manage", "whatsapp.contact_manage", "whatsapp.callbacks", "whatsapp.reports", "whatsapp.export", "whatsapp.ai_suggest", "whatsapp.ai_manage", "whatsapp.call_survey_manage", "call.originate", "performance.view_all"];
const user: User = { id: 1, name: "Toprak Şahin Güreli", email: "toprak@example.com", active: true, roles: ["Yönetici"], roleIds: [1], permissions: WA_PERMS, mfaEnabled: false, mustChangePassword: false, sipExtension: "1001", createdAt: "2026-01-01T00:00:00Z" };

const ago = (min: number) => new Date(Date.now() - min * 60000).toISOString();
const hours = { enabled: true, days: [0, 1, 2, 3, 4].map(() => ({ open: true, from: "09:00", to: "18:00" })).concat([{ open: true, from: "10:00", to: "14:00" }, { open: false, from: "09:00", to: "18:00" }]), holidays: ["2026-10-29"] };
const settings = { readReceipts: true, greeting: { enabled: true, text: "Merhaba {musteri}, ben {unvan} {ad}. Sizinle ben ilgileniyorum.", template: "", templateLang: "", forHelpers: false }, distribution: { enabled: true, maxOpen: 8 }, waitingMinutes: 15, hours, survey: { mode: "tally", url: "https://tally.so/r/abc123", text: "Görüşmemizi değerlendirir misiniz?", template: "", templateLang: "", alertBelow: 3 }, botTimeoutMinutes: 30, humanKeywords: ["temsilci", "insan"], optOutKeywords: ["DUR"], optOutReply: "Artık size toplu mesaj göndermeyeceğiz." };
const channels = [
  { id: 1, name: "Destek Hattı", displayPhone: "+90 850 123 45 67", phoneNumberId: "1234567890", wabaId: "998877", appId: "5566", graphVersion: "", hasToken: true, hasAppSecret: true, verifyToken: "v3rify-t0ken-abc", hookPath: "/api/v1/wa/hook/k1a2b3", surveyHookPath: "/api/v1/wa/survey/k1a2b3", active: true, settings, hasSurveySecret: true, verifiedName: "Örnek Şirket", qualityRating: "GREEN", messagingLimit: "TIER_1K", lastWebhookAt: ago(2), memberIds: [1, 2, 3], createdAt: ago(9000) },
  { id: 2, name: "Satış Hattı", displayPhone: "+90 850 765 43 21", phoneNumberId: "222", wabaId: "998877", appId: "", graphVersion: "", hasToken: true, hasAppSecret: false, verifyToken: "t2", hookPath: "/api/v1/wa/hook/k2", surveyHookPath: "/api/v1/wa/survey/k2", active: true, settings: { ...settings, greeting: { ...settings.greeting, enabled: false }, hours: { ...hours, holidays: null }, humanKeywords: null, optOutKeywords: null }, hasSurveySecret: false, verifiedName: "", qualityRating: "YELLOW", messagingLimit: "TIER_250", lastError: "Erişim anahtarının süresi dolmuş.", lastErrorAt: ago(40), memberIds: [1], createdAt: ago(300) },
];
const people = [
  { id: 1, name: "Toprak Şahin Güreli", hasAvatar: false, channelIds: [1, 2], online: true, available: true, canReply: true },
  { id: 2, name: "Ayşe Kaya", hasAvatar: false, channelIds: [1], online: true, available: false, canReply: true },
  { id: 3, name: "Mehmet Demir", hasAvatar: false, channelIds: [1], online: false, available: false, canReply: true },
  { id: 4, name: "Elif Yıldız", hasAvatar: false, channelIds: [2], online: true, available: true, canReply: true },
  { id: 5, name: "Burak Şen", hasAvatar: false, channelIds: [1], online: true, available: true, canReply: false },
];
const ad = { source_url: "https://fb.me/abc", source_type: "ad", headline: "Fiber internette ilk 3 ay yarı fiyat", body: "Hemen yazın, size uygun paketi bulalım.", media_type: "image" };
const contact = (id: number, name: string, wa: string) => ({ id, waId: wa, name: "", profileName: name, display: name, tags: id === 1 ? ["vip"] : [], note: "", optedOut: false, blocked: false, source: id === 1 ? ad : undefined });
const ticket = (id: number, over: Record<string, unknown> = {}) => ({ id, number: 1000 + id, status: "open", priority: "normal", category: "", tags: [], waitingCount: 0, reopenCount: 0, createdAt: ago(60), participants: [], ...over });
const conversations = [
  { id: 1, channelId: 1, channelName: "Destek Hattı", contact: contact(1, "Zeynep Arslan", "905321112233"), ticket: ticket(1, { owner: { id: 1, name: "Toprak Şahin Güreli", hasAvatar: false }, participants: [{ id: 1, name: "Toprak Şahin Güreli", hasAvatar: false, role: "owner" }, { id: 2, name: "Ayşe Kaya", hasAvatar: false, role: "helper" }] }), last: { id: 5, direction: "in", kind: "text", preview: "Tamam teşekkürler, bekliyorum", at: ago(3), status: "received" }, unread: 1, teamReadId: 4, lastInboundAt: ago(3), windowEndsAt: new Date(Date.now() + 20 * 3600000).toISOString(), version: 10 },
  { id: 2, channelId: 1, channelName: "Destek Hattı", contact: contact(2, "Can Öztürk", "905339998877"), ticket: ticket(2, { awaitingSince: ago(22), waitingListedAt: ago(7), owner: { id: 2, name: "Ayşe Kaya", hasAvatar: false } }), last: { id: 9, direction: "in", kind: "text", preview: "Hala cevap bekliyorum", at: ago(22), status: "received" }, unread: 2, teamReadId: 0, lastInboundAt: ago(22), windowEndsAt: new Date(Date.now() + 23 * 3600000).toISOString(), version: 11 },
  { id: 3, channelId: 2, channelName: "Satış Hattı", contact: contact(3, "Elif Şahin", "905301234567"), ticket: ticket(3, { status: "bot" }), last: { id: 12, direction: "out", kind: "interactive", preview: "Hangi konuda yardım istersiniz?", at: ago(1), status: "delivered", senderName: "Karşılama" }, unread: 0, teamReadId: 12, lastInboundAt: ago(1), windowEndsAt: new Date(Date.now() + 23 * 3600000).toISOString(), version: 12 },
  { id: 4, channelId: 1, channelName: "Destek Hattı", contact: contact(4, "Burak Aydın", "905551112244"), ticket: ticket(4, { awaitingSince: ago(4) }), last: { id: 14, direction: "in", kind: "image", preview: "📷 Fotoğraf", at: ago(4), status: "received" }, unread: 1, teamReadId: 0, lastInboundAt: ago(4), windowEndsAt: new Date(Date.now() + 23 * 3600000).toISOString(), version: 13 },
];
const agent = (id: number, name: string) => ({ kind: "agent", userId: id, name, hasAvatar: false });
const messages = [
  { id: 1, conversationId: 1, direction: "in", kind: "text", body: "Merhaba, internetim yarım saattir çalışmıyor.", status: "received", sender: { kind: "customer", name: "Zeynep Arslan" }, createdAt: ago(30), referral: ad },
  { id: 2, conversationId: 1, direction: "event", kind: "text", body: "Toprak sohbeti karşıladı.", status: "received", sender: { kind: "system" }, createdAt: ago(28) },
  { id: 3, conversationId: 1, direction: "out", kind: "text", body: "Merhaba Zeynep, ben teknik destek uzmanınız Toprak. Sizinle ben ilgileniyorum.", status: "read", sender: agent(1, "Toprak Şahin Güreli"), createdAt: ago(28), sentAt: ago(28), deliveredAt: ago(28), readAt: ago(27) },
  { id: 4, conversationId: 1, direction: "note", kind: "text", body: "Bölgede arıza kaydı var, saha ekibine sordum.", status: "received", sender: agent(2, "Ayşe Kaya"), createdAt: ago(10) },
  { id: 6, conversationId: 1, direction: "out", kind: "text", body: "Bölgenizde *genel bir arıza* var, ekiplerimiz çalışıyor. Yaklaşık 1 saat içinde düzelmesi bekleniyor.", status: "delivered", sender: agent(1, "Toprak Şahin Güreli"), createdAt: ago(5), reactions: [{ emoji: "👍", ours: false }] },
  { id: 5, conversationId: 1, direction: "in", kind: "text", body: "Tamam teşekkürler, bekliyorum", status: "received", sender: { kind: "customer", name: "Zeynep Arslan" }, createdAt: ago(3) },
];
const templates = [
  { id: 1, name: "siparis_hazir", language: "tr", category: "UTILITY", status: "APPROVED", components: [{ type: "HEADER", format: "TEXT", text: "Siparişiniz hazır" }, { type: "BODY", text: "Merhaba {{1}}, {{2}} numaralı siparişiniz hazırlandı ve kargoya verildi." }, { type: "FOOTER", text: "Örnek Şirket" }, { type: "BUTTONS", buttons: [{ type: "URL", text: "Kargo takibi", url: "https://example.com/{{1}}" }] }], updatedAt: ago(3000) },
  { id: 2, name: "kampanya_ekim", language: "tr", category: "MARKETING", status: "PENDING", components: [{ type: "HEADER", format: "IMAGE" }, { type: "BODY", text: "Ekim ayına özel *%20 indirim* sizi bekliyor!" }, { type: "BUTTONS", buttons: [{ type: "QUICK_REPLY", text: "İlgileniyorum" }, { type: "QUICK_REPLY", text: "Beni listeden çıkar" }] }], updatedAt: ago(20) },
  { id: 3, name: "odeme_hatirlatma", language: "tr", category: "UTILITY", status: "REJECTED", rejectedReason: "INCORRECT_CATEGORY", components: [{ type: "BODY", text: "Merhaba {{1}}, faturanızın son ödeme tarihi yarın." }], updatedAt: ago(600) },
  { id: 5, name: "anket_cagri", language: "tr", category: "UTILITY", status: "APPROVED", components: [{ type: "BODY", text: "Merhaba {{1}}, az önce {{2}} ile yaptığınız görüşmeyi nasıl buldunuz?" }, { type: "BUTTONS", buttons: [{ type: "QUICK_REPLY", text: "Çok iyi" }, { type: "QUICK_REPLY", text: "İdare eder" }, { type: "QUICK_REPLY", text: "Kötü" }] }], updatedAt: ago(9000) },
  { id: 6, name: "ulasilamadi", language: "tr", category: "UTILITY", status: "APPROVED", fill: ["agent"], components: [{ type: "BODY", text: "VatanSoft Teknik Uzmanınız {{1}} ben. Talebinize istinaden aradım, size ulaşamadım. Müsaitliğinizde buradan geri dönüş yapabilir misiniz?" }], updatedAt: ago(100) },
  { id: 4, name: "tekrar_merhaba", language: "tr", category: "UTILITY", status: "APPROVED", quality: "GREEN", components: [{ type: "BODY", text: "Merhaba {{1}}, size tekrar ulaşmak istedik. Uygun olduğunuzda bu mesaja cevap yazabilirsiniz." }], updatedAt: ago(9000) },
];
const graph = {
  nodes: [
    { id: "start", type: "start", x: 40, y: 160, data: {} },
    { id: "hi", type: "message", x: 340, y: 150, data: { text: "Merhaba {musteri}, Örnek Şirket'e hoş geldiniz!" } },
    { id: "menu", type: "menu", x: 640, y: 110, data: { text: "Hangi konuda yardım istersiniz?", style: "buttons", var: "konu", options: [{ id: "a", label: "Arıza bildirimi" }, { id: "b", label: "Fatura" }, { id: "c", label: "Yeni abonelik" }] } },
    { id: "ask", type: "ask", x: 960, y: 20, data: { text: "Abone numaranızı yazar mısınız?", var: "abone", validate: "number" } },
    { id: "api", type: "api", x: 1260, y: 20, data: { integration: 1, map: [{ var: "durum", path: "data.durum" }] } },
    { id: "handoff", type: "handoff", x: 1560, y: 120, data: { text: "Sizi teknik ekibimize aktarıyorum.", teamId: 1 } },
    { id: "bill", type: "message", x: 960, y: 250, data: { text: "Faturanızı uygulamadan görüntüleyebilirsiniz." } },
    { id: "end", type: "end", x: 1260, y: 290, data: { text: "İyi günler dileriz!", resolve: true } },
  ],
  edges: [
    { id: "e1", from: "start", port: "next", to: "hi" },
    { id: "e2", from: "hi", port: "next", to: "menu" },
    { id: "e3", from: "menu", port: "a", to: "ask" },
    { id: "e4", from: "ask", port: "next", to: "api" },
    { id: "e5", from: "api", port: "ok", to: "handoff" },
    { id: "e6", from: "api", port: "fail", to: "handoff" },
    { id: "e7", from: "menu", port: "b", to: "bill" },
    { id: "e8", from: "bill", port: "next", to: "end" },
  ],
};
const bots = [
  { id: 1, name: "Destek karşılama", description: "Müşteriyi karşılar, konuyu sorar, doğru ekibe aktarır.", active: true, channelIds: [1], trigger: "entry", keywords: [], draft: graph, publishedVersion: 3, publishedAt: ago(2000), draftChanged: true, updatedAt: ago(15) },
  { id: 2, name: "Mesai dışı", description: "", active: false, channelIds: [], trigger: "after_hours", keywords: [], draft: { nodes: [graph.nodes[0]], edges: [] }, publishedVersion: 0, draftChanged: true, updatedAt: ago(90) },
];
const rules = [
  { id: 1, name: "Mesai dışı cevabı", active: true, channelIds: [1, 2], trigger: "outside_hours", conditions: [], actions: [{ kind: "send_text", text: "Şu an mesai dışındayız." }], cooldownMin: 720, position: 0, runs: 142, lastRunAt: ago(300), updatedAt: ago(5000) },
  { id: 2, name: "Bekleme bilgisi", active: true, channelIds: [1], trigger: "no_reply", conditions: [{ kind: "after_minutes", value: "10" }], actions: [{ kind: "send_text", text: "Biraz gecikiyoruz." }], cooldownMin: 0, position: 1, runs: 12, updatedAt: ago(200) },
  { id: 3, name: "Fiyat soruları satışa", active: false, channelIds: [2], trigger: "message_in", conditions: [{ kind: "text_contains", value: "fiyat, ücret" }, { kind: "no_owner", value: "" }], actions: [{ kind: "assign_team", teamId: 2 }, { kind: "add_tag", value: "satış" }], cooldownMin: 0, position: 2, runs: 0, updatedAt: ago(20) },
];
const teamsList = [{ id: 1, name: "Teknik Destek", color: "#0ea5e9", memberIds: [1, 2, 3] }, { id: 2, name: "Satış", color: "#f59e0b", memberIds: [1] }];
const quick = [
  { id: 1, shortcut: "kargo", title: "Kargo takip", body: "Merhaba {musteri}, kargonuzu şu linkten takip edebilirsiniz: https://example.com", channelIds: [1, 2] },
  { id: 2, shortcut: "ariza", title: "Genel arıza", body: "Bölgenizde *genel bir arıza* var, ekiplerimiz çalışıyor.", channelIds: [1] },
];
const report = {
  from: "2026-09-19", to: "2026-09-26",
  agents: [
    { user: people[0], owned: 84, helped: 12, resolved: 79, messages: 640, avgFirstReplySec: 95, avgResolveSec: 2400, waitingEntries: 2, ratings: 40, avgRating: 4.7 },
    { user: people[1], owned: 61, helped: 20, resolved: 55, messages: 480, avgFirstReplySec: 1100, avgResolveSec: 4100, waitingEntries: 9, ratings: 22, avgRating: 4.1 },
    { user: people[2], owned: 12, helped: 3, resolved: 10, messages: 70, avgFirstReplySec: 300, avgResolveSec: 1500, waitingEntries: 0, ratings: 0, avgRating: 0 },
  ],
  channels: [
    { id: 1, name: "Destek Hattı", tickets: 180, resolved: 160, botResolved: 34, inbound: 2100, outbound: 1800, failed: 3, avgFirstReplySec: 420, waitingEntries: 11, avgRating: 4.5 },
    { id: 2, name: "Satış Hattı", tickets: 40, resolved: 30, botResolved: 2, inbound: 300, outbound: 280, failed: 0, avgFirstReplySec: 200, waitingEntries: 0, avgRating: 4.8 },
  ],
  hours: [0, 0, 0, 0, 0, 0, 1, 4, 22, 58, 80, 95, 70, 64, 88, 91, 76, 60, 33, 20, 14, 9, 4, 1],
};
const callbacks = [
  { id: 1, channelName: "Destek Hattı", conversationId: 2, customer: "Can Öztürk", phone: "905339998877", note: "Fatura itirazı için aranmak istiyor", status: "open", createdAt: ago(12) },
  { id: 2, channelName: "Satış Hattı", conversationId: 3, customer: "Elif Şahin", phone: "905301234567", note: "", status: "done", doneBy: "Ayşe Kaya", doneAt: ago(60), createdAt: ago(200) },
];

const prefs: { sound: boolean; desktop: boolean; mutedUntil?: string; conversations: { id: number; mutedUntil?: string; pinnedAt?: string }[] } = { sound: true, desktop: true, conversations: [{ id: 2, mutedUntil: new Date(Date.now() + 8 * 3600000).toISOString() }, { id: 3, pinnedAt: ago(100) }] };
const muteWord = (w?: string) => (!w ? undefined : w === "off" ? "" : new Date(Date.now() + (w === "always" ? 1e12 : 3600000)).toISOString());

function answer(method: string, path: string, body: unknown): unknown {
  const p = path.replace(/^\/api\/v1\/wa/, "").split("?")[0];
  if (p === "/conversations") return { items: conversations, hidden: [], version: 20, me: 1 };
  if (/^\/conversations\/\d+\/messages$/.test(p)) return method === "GET" ? (p.includes("/1/") ? messages : messages.slice(0, 1).map((m) => ({ ...m, conversationId: Number(p.split("/")[2]) }))) : { ...(body as object), id: Date.now(), status: "queued", sender: agent(1, user.name), direction: "out", createdAt: new Date().toISOString() };
  if (/^\/conversations\/\d+$/.test(p)) return conversations.find((c) => c.id === Number(p.split("/")[2]));
  if (p === "/conversations/resolved") return [];
  if (/^\/contacts\/\d+\/history$/.test(p)) return [{ conversationId: 1, ticketId: 9, number: 998, channelName: "Destek Hattı", status: "resolved", owner: "Ayşe Kaya", createdAt: ago(20000), resolvedAt: ago(19000), rating: 5, messages: 14 }];
  if (p === "/channels") return channels;
  if (p === "/templates") return templates;
  if (p === "/teams") return teamsList;
  if (p === "/agents") return people;
  if (p === "/quick-replies") return quick;
  if (p === "/rules") return rules;
  if (p === "/bots") return bots;
  if (/^\/bots\/\d+$/.test(p)) return bots.find((b) => b.id === Number(p.split("/")[2]));
  if (/^\/bots\/\d+\/draft$/.test(p)) return { ...bots[0], draft: body, updatedAt: new Date().toISOString() };
  if (/^\/bots\/\d+\/versions$/.test(p)) return [{ version: 3, publishedBy: "Toprak Şahin Güreli", createdAt: ago(2000) }, { version: 2, publishedBy: "Ayşe Kaya", createdAt: ago(9000) }, { version: 1, publishedBy: "Toprak Şahin Güreli", createdAt: ago(20000) }];
  if (/^\/bots\/\d+\/report$/.test(p)) return { started: 320, handoffs: 190, ended: 96, timeouts: 34, nodes: { start: 320, hi: 320, menu: 318, ask: 150, api: 140, handoff: 190, bill: 101, end: 96 }, fails: { menu: 22, ask: 9 }, drops: { menu: 20, ask: 14 } };
  if (/^\/bots\/\d+\/publish$/.test(p)) return { problems: ["\"Yeni abonelik\" seçeneğinden bir kutuya ok çıkmıyor.", "Soru kutusunda anlaşılmazsa yolu bağlı değil; müşteri temsilciye aktarılır."] };
  if (p === "/bots/simulate") {
    const b = body as { start: boolean; choiceId?: string };
    if (b.start) return { outputs: [{ kind: "text", text: "Merhaba Ayşe, Örnek Şirket'e hoş geldiniz!" }, { kind: "menu", text: "Hangi konuda yardım istersiniz?", style: "buttons", options: graph.nodes[2].data.options }], nodeId: "menu", vars: { musteri: "Ayşe", numara: "+905xxxxxxxxx" }, tries: 0, done: false };
    if (b.choiceId === "opt:b") return { outputs: [{ kind: "text", text: "Faturanızı uygulamadan görüntüleyebilirsiniz." }, { kind: "text", text: "İyi günler dileriz!" }, { kind: "end", detail: "resolve" }], nodeId: "end", vars: { musteri: "Ayşe", konu: "Fatura" }, tries: 0, done: true };
    return { outputs: [{ kind: "text", text: "Abone numaranızı yazar mısınız?" }], nodeId: "ask", vars: { musteri: "Ayşe", konu: "Arıza bildirimi" }, tries: 0, done: false };
  }
  if (p === "/integrations") return [{ id: 1, name: "Abone sorgu", method: "GET", url: "https://api.example.com/abone/{abone}", body: "", timeoutSec: 8, headerNames: ["Authorization"] }];
  if (p === "/callbacks") return callbacks;
  if (p === "/events") return [{ id: 7, channelId: 1, status: "failed", attempts: 8, lastError: "medya indirilemedi: 404", receivedAt: ago(90), summary: "1 mesaj" }];
  if (p === "/reports") return report;
  if (p === "/ai/status") return { available: true };
  if (/^\/conversations\/\d+\/reads$/.test(p)) return [{ user: { id: 1, name: "Toprak Şahin Güreli", hasAvatar: false }, messageId: 99, readAt: ago(29) }, { user: { id: 2, name: "Ayşe Kaya", hasAvatar: false }, messageId: 1, readAt: ago(12) }];
  if (p === "/me") {
    const b = (body ?? {}) as { sound?: boolean; desktop?: boolean; mute?: string };
    if (method === "PUT") {
      if (b.sound !== undefined) prefs.sound = b.sound;
      if (b.desktop !== undefined) prefs.desktop = b.desktop;
      const u = muteWord(b.mute);
      if (u !== undefined) prefs.mutedUntil = u || undefined;
    }
    return prefs;
  }
  if (p.startsWith("/me/conversations/")) {
    const id = Number(p.split("/")[3]);
    const b = (body ?? {}) as { mute?: string; pin?: boolean };
    let row = prefs.conversations.find((c) => c.id === id);
    if (!row) { row = { id }; prefs.conversations.push(row); }
    const u = muteWord(b.mute);
    if (u !== undefined) row.mutedUntil = u || undefined;
    if (b.pin !== undefined) row.pinnedAt = b.pin ? new Date().toISOString() : undefined;
    return prefs;
  }
  if (p === "/lookup") return conversations.slice(0, 1);
  if (p === "/ai") return { enabled: true, model: "claude-sonnet-5", instructions: "Firmamız internet ve telefon hizmeti veriyor.", useQuickReplies: true, hasKey: true, models: [{ id: "claude-sonnet-5", label: "Dengeli (önerilen)" }, { id: "claude-haiku-4-5-20251001", label: "Hızlı ve ucuz" }, { id: "claude-opus-5-5", label: "En güçlü, daha yavaş" }] };
  if (/\/suggest$/.test(p)) return { text: "Anlayışınız için teşekkürler Zeynep Hanım. Arıza giderilince size buradan haber vereceğim, tahmini süre [süre]." };
  if (p === "/files") return { id: 9, name: "kampanya.png", mime: "image/png", size: 120000, kind: "image", url: "/api/v1/wa/files/9" };
  if (p === "/call-survey") return { enabled: true, channelId: 1, template: "anket_cagri", templateLang: "tr", params: ["{musteri}", "{temsilci}"], mode: "buttons", buttonScores: [5, 3, 1], linkUrl: "", directions: "both", minSeconds: 30, delayMinutes: 2, quietDays: 7, alertBelow: 2, thankYou: "Değerlendirmeniz için teşekkür ederiz." };
  if (p === "/call-survey/report") return { queued: 2, sent: 140, answered: 61, failed: 3, skipped: 12, average: 4.3, agents: [{ user: people[0], sent: 80, answered: 40, average: 4.6, low: 1 }, { user: people[1], sent: 60, answered: 21, average: 3.7, low: 4 }], recent: [{ id: 1, agent: "Ayşe Kaya", phone: "905321112233", score: 1, comment: "Çok beklettiler", conversationId: 1, answeredAt: ago(30) }, { id: 2, agent: "Toprak Şahin Güreli", phone: "905339998877", score: 5, comment: "", conversationId: 2, answeredAt: ago(90) }] };
  if (p.endsWith("/test")) return { ok: true, message: "Bağlantı çalışıyor. Numara: +90 850 123 45 67, ad: Örnek Şirket.", subscribed: false, webhookSeen: true };
  return method === "GET" ? [] : {};
}

let installed = false;
function installMock() {
  if (installed) return;
  installed = true;
  const real = window.fetch.bind(window);
  window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.pathname + input.search : input.url;
    if (url.startsWith("/api/v1/performance/today")) {
      await new Promise((r) => setTimeout(r, 150));
      return new Response(JSON.stringify({ scope: "all", from: "", to: "", items: perfRows }), { status: 200, headers: { "Content-Type": "application/json" } });
    }
    if (!url.startsWith("/api/v1/wa/")) return real(input, init);
    await new Promise((r) => setTimeout(r, 150));
    let body: unknown;
    try {
      body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    } catch {
      body = undefined;
    }
    const out = answer(init?.method ?? "GET", url, body);
    return new Response(out === undefined ? null : JSON.stringify(out), { status: out === undefined ? 204 : 200, headers: { "Content-Type": "application/json" } });
  };
}

function phoneValue(): SoftphoneValue {
  const noop = async () => undefined;
  return { status: "registered", lastEnded: null, callId: null, lastPeer: null, lastUnreached: null, extension: "1001", error: null, muted: false, held: false, peer: null, endReason: null, callStartedAt: null, answeredAt: null, audioRef: { current: null }, remoteGain: 1, setRemoteGain: () => undefined, wave: () => null, spectrum: () => null, call: noop, answer: noop, hangup: noop, toggleMute: () => undefined, toggleHold: noop, transfer: noop, sendDtmf: () => undefined, secondary: false, takeOver: () => undefined } as unknown as SoftphoneValue;
}

const PAGES = [
  { path: "/whatsapp/1", label: "Gelen kutusu" },
  { path: "/whatsapp/settings?tab=devices", label: "Cihazlar" },
  { path: "/whatsapp/settings?tab=device-settings", label: "Cihaz ayarları" },
  { path: "/whatsapp/settings?tab=templates", label: "Şablonlar" },
  { path: "/whatsapp/settings?tab=rules", label: "Kurallar" },
  { path: "/whatsapp/settings?tab=bots", label: "Chatbot'lar" },
  { path: "/whatsapp/settings?tab=quick", label: "Hazır yanıtlar" },
  { path: "/whatsapp/settings?tab=teams", label: "Ekipler" },
  { path: "/whatsapp/settings?tab=integrations", label: "Dış sistemler" },
  { path: "/whatsapp/settings?tab=events", label: "İşlenemeyenler" },
  { path: "/whatsapp/bots/1", label: "Chatbot akışı" },
  { path: "/whatsapp/reports", label: "Raporlar" },
  { path: "/whatsapp/callbacks", label: "Geri arama" },
  { path: "/whatsapp/settings?tab=ai", label: "Yapay zekâ" },
  { path: "/whatsapp/settings?tab=call-survey", label: "Çağrı anketi" },
  { path: "/performance", label: "Ekip performansı" },
  { path: "/preferences", label: "Ayarlarım" },
];

export default function WAPreview() {
  installMock();
  const [page, setPage] = useState(() => new URLSearchParams(window.location.search).get("p") ?? PAGES[0].path);
  const go = (p: string) => {
    setPage(p);
    window.history.replaceState(null, "", `/__wa?p=${encodeURIComponent(p)}`);
  };
  return (
    <AuthMockProvider user={user}>
      <TeamsMockProvider>
        <SoftphoneMockProvider value={phoneValue()}>
          <MemoryRouter key={page} initialEntries={[page]}>
            <WhatsAppProvider>
              <div className="min-h-svh bg-background">
                <div className="flex h-16 items-center gap-1 overflow-x-auto border-b border-border/60 bg-card px-4">
                  {PAGES.map((p) => (
                    <button key={p.path} type="button" onClick={() => go(p.path)} className={`shrink-0 rounded-lg px-2.5 py-1.5 text-xs font-medium ${page === p.path ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent"}`}>{p.label}</button>
                  ))}
                </div>
                <main className="px-4 py-6 md:px-6 lg:px-8">
                  <Routes>
                    <Route path="/whatsapp/settings" element={<WhatsAppSettings />} />
                    <Route path="/whatsapp/bots/:id" element={<WhatsAppBot />} />
                    <Route path="/whatsapp/reports" element={<WhatsAppReports />} />
                    <Route path="/whatsapp/callbacks" element={<WhatsAppCallbacks />} />
                    <Route path="/whatsapp" element={<WhatsApp />} />
                    <Route path="/whatsapp/:id" element={<WhatsApp />} />
                    <Route path="/performance" element={<TeamPerformance />} />
                    <Route path="/preferences" element={<Preferences />} />
                  </Routes>
                </main>
              </div>
              <WAAlerts />
            </WhatsAppProvider>
          </MemoryRouter>
        </SoftphoneMockProvider>
      </TeamsMockProvider>
    </AuthMockProvider>
  );
}

function perfRow(id: number, name: string, status: string, long: number, un: number, sh: number, talk: number, shift: number, extra: Record<string, unknown> = {}) {
  return {
    userId: id, name, extension: String(1000 + id), roles: ["Teknik Destek"], status, since: ago(12),
    shift: { firstStart: ago(240), open: status !== "off", seconds: shift },
    calls: { total: long + un + sh, answered: long + sh, short: sh, long, unanswered: un, inbound: long, outbound: un, inboundMissed: un, outboundMissed: 0, inboundReal: Math.round(long * 0.6), outboundReal: long - Math.round(long * 0.6), talkSeconds: talk, avgTalkSeconds: long ? Math.round(talk / long) : 0, longestSeconds: 1310, over5: 3, over10: 1, over20: 0, peers: long, avgAnswerSeconds: 6 },
    escalations: 2, breakSeconds: 900,
    recent: [{ peer: "05304230113", peerName: "Mehmet Demir", direction: "inbound", disposition: "answered", startedAt: ago(20), durationSeconds: 312 }],
    ...extra,
  };
}
const perfRows = [
  perfRow(1, "Toprak Şahin Güreli", "talking", 31, 4, 3, 5400, 14400, { call: { peer: "05304230113", peerName: "Mehmet Demir", direction: "inbound", startedAt: ago(2) }, wa: { owned: 12, resolved: 10, messages: 140, avgFirstReplySec: 95, ratings: 6, avgRating: 4.7 }, survey: { answered: 9, average: 4.6 } }),
  perfRow(2, "Ayşe Kaya", "available", 24, 14, 6, 3900, 14000, { wa: { owned: 8, resolved: 5, messages: 60, avgFirstReplySec: 700, ratings: 2, avgRating: 3.5 } }),
  perfRow(3, "Mehmet Demir", "break", 18, 3, 1, 3000, 12000),
  perfRow(4, "Elif Su Uzunsoyadlıkişi", "off", 0, 0, 0, 0, 0),
];
