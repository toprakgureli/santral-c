// WhatsApp endpoints.

import { download, request } from "@/api/client";
import type {
  BotGraph,
  BotStats,
  SimResult,
  WAAgent,
  WAAISettings,
  WAMute,
  WAPrefs,
  WACallSurveyReport,
  WACallSurveySettings,
  WAFile,
  WABot,
  WACallback,
  WAChannel,
  WAChannelCheck,
  WAConversation,
  WAContact,
  WAEventRow,
  WAHistoryItem,
  WAIntegration,
  WAListResult,
  WAMessage,
  WAQuickReply,
  WAReadInfo,
  WAReport,
  WARule,
  WASearchHit,
  WASettings,
  WATeam,
  WATemplate,
} from "@/whatsapp/types";

const q = (params: Record<string, string | number | undefined>) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "" && v !== 0) s.set(k, String(v));
  const out = s.toString();
  return out ? `?${out}` : "";
};

const json = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });

export const waApi = {
  // inbox
  conversations: (since = 0) => request<WAListResult>("/wa/conversations" + q({ since })),
  resolved: (before?: string, search?: string) => request<WAConversation[]>("/wa/conversations/resolved" + q({ before, q: search })),
  conversation: (id: number) => request<WAConversation>(`/wa/conversations/${id}`),
  messages: (id: number, p: { before?: number; after?: number; around?: number } = {}) => request<WAMessage[]>(`/wa/conversations/${id}/messages` + q(p)),
  send: (id: number, body: { clientId: string; kind?: string; body?: string; replyTo?: number; templateId?: number; params?: unknown; targetId?: number; emoji?: string }) =>
    request<WAMessage>(`/wa/conversations/${id}/messages`, json("POST", body)),
  sendMedia: (id: number, file: File, caption: string, clientId: string, replyTo?: number) => {
    const f = new FormData();
    f.set("file", file);
    f.set("caption", caption);
    f.set("clientId", clientId);
    if (replyTo) f.set("replyTo", String(replyTo));
    return request<WAMessage>(`/wa/conversations/${id}/media`, { method: "POST", body: f });
  },
  note: (id: number, body: string) => request<WAMessage>(`/wa/conversations/${id}/notes`, json("POST", { body })),
  read: (id: number, messageId: number) => request<void>(`/wa/conversations/${id}/read`, json("POST", { messageId })),
  unread: (id: number) => request<void>(`/wa/conversations/${id}/unread`, json("POST")),
  typing: (id: number) => request<void>(`/wa/conversations/${id}/typing`, json("POST")),
  reads: (id: number) => request<WAReadInfo[]>(`/wa/conversations/${id}/reads`),
  greet: (id: number) => request<void>(`/wa/conversations/${id}/greet`, json("POST")),
  take: (id: number) => request<void>(`/wa/conversations/${id}/take`, json("POST")),
  assign: (id: number, body: { userId?: number; teamId?: number; note?: string }) => request<void>(`/wa/conversations/${id}/assign`, json("POST", body)),
  resolve: (id: number) => request<void>(`/wa/conversations/${id}/resolve`, json("POST")),
  reopen: (id: number) => request<void>(`/wa/conversations/${id}/reopen`, json("POST")),
  updateTicket: (id: number, body: { status?: string; priority?: string; category?: string; tags?: string[] }) => request<void>(`/wa/conversations/${id}/ticket`, json("PATCH", body)),
  retry: (messageId: number) => request<void>(`/wa/messages/${messageId}/retry`, json("POST")),
  search: (text: string, conversation?: number) => request<WASearchHit[]>("/wa/search" + q({ q: text, conversation })),
  lookup: (number: string) => request<WAConversation[]>("/wa/lookup" + q({ number })),
  start: (channelId: number, number: string, name: string) => request<WAConversation>("/wa/start", json("POST", { channelId, number, name })),
  history: (contactId: number) => request<WAHistoryItem[]>(`/wa/contacts/${contactId}/history`),
  updateContact: (id: number, body: Partial<Pick<WAContact, "name" | "tags" | "note" | "optedOut" | "blocked">>) => request<WAContact>(`/wa/contacts/${id}`, json("PATCH", body)),

  // devices
  channels: () => request<WAChannel[]>("/wa/channels"),
  createChannel: (body: Record<string, unknown>) => request<WAChannel>("/wa/channels", json("POST", body)),
  updateChannel: (id: number, body: Record<string, unknown>) => request<WAChannel>(`/wa/channels/${id}`, json("PATCH", body)),
  deleteChannel: (id: number) => request<void>(`/wa/channels/${id}`, json("DELETE")),
  testChannel: (id: number) => request<WAChannelCheck>(`/wa/channels/${id}/test`, json("POST")),
  subscribe: (id: number) => request<void>(`/wa/channels/${id}/subscribe`, json("POST")),
  saveSettings: (id: number, settings: WASettings, surveySecret = "") => request<WAChannel>(`/wa/channels/${id}/settings`, json("PUT", { settings, surveySecret })),
  copySettings: (id: number, from: number, sections: string[]) => request<WAChannel>(`/wa/channels/${id}/copy-settings`, json("POST", { from, sections })),
  setMembers: (id: number, userIds: number[]) => request<void>(`/wa/channels/${id}/members`, json("PUT", { userIds })),
  copyToChannel: (from: number, to: number, what: "quick_replies" | "rules") => request<{ copied: number }>("/wa/channels/copy", json("POST", { from, to, what })),

  // templates
  templates: (channel: number) => request<WATemplate[]>("/wa/templates" + q({ channel })),
  createTemplate: (body: Record<string, unknown>) => request<WATemplate>("/wa/templates", json("POST", body)),
  syncTemplates: (channelId: number) => request<{ count: number }>("/wa/templates/sync", json("POST", { channelId })),
  templateMedia: (channelId: number, file: File) => {
    const f = new FormData();
    f.set("file", file);
    f.set("channelId", String(channelId));
    return request<{ handle: string }>("/wa/templates/media", { method: "POST", body: f });
  },
  deleteTemplate: (id: number) => request<void>(`/wa/templates/${id}`, json("DELETE")),

  // people
  teams: () => request<WATeam[]>("/wa/teams"),
  saveTeam: (id: number, body: { name: string; color: string; memberIds: number[] }) => request<void>(id ? `/wa/teams/${id}` : "/wa/teams", json(id ? "PUT" : "POST", body)),
  deleteTeam: (id: number) => request<void>(`/wa/teams/${id}`, json("DELETE")),
  agents: () => request<WAAgent[]>("/wa/agents"),

  // quick replies
  quickReplies: (channel = 0) => request<WAQuickReply[]>("/wa/quick-replies" + q({ channel })),
  saveQuickReply: (id: number, body: Omit<WAQuickReply, "id">) => request<void>(id ? `/wa/quick-replies/${id}` : "/wa/quick-replies", json(id ? "PUT" : "POST", body)),
  deleteQuickReply: (id: number) => request<void>(`/wa/quick-replies/${id}`, json("DELETE")),

  // rules
  rules: () => request<WARule[]>("/wa/rules"),
  saveRule: (id: number, body: Omit<WARule, "id" | "runs" | "lastRunAt" | "updatedAt" | "position">) => request<WARule>(id ? `/wa/rules/${id}` : "/wa/rules", json(id ? "PUT" : "POST", body)),
  deleteRule: (id: number) => request<void>(`/wa/rules/${id}`, json("DELETE")),
  orderRules: (ids: number[]) => request<void>("/wa/rules/order", json("PUT", { ids })),

  // chatbots
  bots: () => request<WABot[]>("/wa/bots"),
  bot: (id: number) => request<WABot>(`/wa/bots/${id}`),
  createBot: (body: { name: string; description?: string; trigger?: string }) => request<WABot>("/wa/bots", json("POST", body)),
  updateBot: (id: number, body: { name: string; description: string; trigger: string; keywords: string[]; channelIds: number[]; active: boolean }) => request<WABot>(`/wa/bots/${id}`, json("PUT", body)),
  saveDraft: (id: number, graph: BotGraph) => request<WABot>(`/wa/bots/${id}/draft`, json("PUT", graph)),
  publishBot: (id: number) => request<{ bot?: WABot; problems?: string[] }>(`/wa/bots/${id}/publish`, json("POST")),
  botVersions: (id: number) => request<{ version: number; publishedBy: string; createdAt: string }[]>(`/wa/bots/${id}/versions`),
  restoreVersion: (id: number, version: number) => request<WABot>(`/wa/bots/${id}/restore`, json("POST", { version })),
  copyBot: (id: number, name: string, channelIds: number[]) => request<WABot>(`/wa/bots/${id}/copy`, json("POST", { name, channelIds })),
  deleteBot: (id: number) => request<void>(`/wa/bots/${id}`, json("DELETE")),
  botReport: (id: number, days: number) => request<BotStats>(`/wa/bots/${id}/report` + q({ days })),
  simulate: (body: { graph: BotGraph; nodeId: string; vars?: Record<string, string>; tries: number; text?: string; choiceId?: string; start: boolean; hoursOpen: boolean }) =>
    request<SimResult>("/wa/bots/simulate", json("POST", body)),

  // outside systems
  integrations: () => request<WAIntegration[]>("/wa/integrations"),
  saveIntegration: (id: number, body: { name: string; method: string; url: string; body: string; timeoutSec: number; headers?: Record<string, string> }) =>
    request<void>(id ? `/wa/integrations/${id}` : "/wa/integrations", json(id ? "PUT" : "POST", body)),
  deleteIntegration: (id: number) => request<void>(`/wa/integrations/${id}`, json("DELETE")),
  testIntegration: (id: number, vars: Record<string, string>) => request<unknown>(`/wa/integrations/${id}/test`, json("POST", vars)),

  // files for chatbots and templates
  uploadFile: (file: File) => {
    const f = new FormData();
    f.set("file", file);
    return request<WAFile>("/wa/files", { method: "POST", body: f });
  },
  fileUrl: (id: number) => `/api/v1/wa/files/${id}`,
  exportChat: (conversationId: number) => download(`/wa/conversations/${conversationId}/export`, `whatsapp_${conversationId}.txt`),

  // the person's own preferences
  myPrefs: () => request<WAPrefs>("/wa/me"),
  savePrefs: (body: { sound?: boolean; desktop?: boolean; mute?: WAMute }) => request<WAPrefs>("/wa/me", json("PUT", body)),
  convPref: (id: number, body: { mute?: WAMute; pin?: boolean }) => request<WAPrefs>(`/wa/me/conversations/${id}`, json("PUT", body)),

  // reply assistant
  aiStatus: () => request<{ available: boolean }>("/wa/ai/status"),
  ai: () => request<WAAISettings>("/wa/ai"),
  saveAI: (body: { enabled: boolean; model: string; instructions: string; useQuickReplies: boolean; apiKey: string }) => request<WAAISettings>("/wa/ai", json("PUT", body)),
  testAI: () => request<{ message: string }>("/wa/ai/test", json("POST")),
  suggest: (conversationId: number, draft: string) => request<{ text: string }>(`/wa/conversations/${conversationId}/suggest`, json("POST", { draft })),

  // survey after a phone call
  callSurvey: () => request<WACallSurveySettings>("/wa/call-survey"),
  saveCallSurvey: (body: WACallSurveySettings) => request<WACallSurveySettings>("/wa/call-survey", json("PUT", body)),
  callSurveyReport: (from: string, to: string) => request<WACallSurveyReport>("/wa/call-survey/report" + q({ from, to })),

  // callbacks, events, reports
  callbacks: (all = false) => request<WACallback[]>("/wa/callbacks" + (all ? "?all=1" : "")),
  doneCallback: (id: number) => request<void>(`/wa/callbacks/${id}/done`, json("POST")),
  events: () => request<WAEventRow[]>("/wa/events"),
  retryEvent: (id: number) => request<void>(`/wa/events/${id}/retry`, json("POST")),
  reports: (from: string, to: string, channel = 0) => request<WAReport>("/wa/reports" + q({ from, to, channel })),
};
