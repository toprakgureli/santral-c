// WhatsApp module types, shared by the inbox, the settings screens and the
// chatbot editor. They mirror what the server sends.

export interface WAPerson {
  id: number;
  name: string;
  hasAvatar: boolean;
  avatarVersion?: number;
  role?: "owner" | "helper";
}

export interface WAContact {
  id: number;
  waId: string;
  name: string;
  profileName: string;
  display: string;
  tags: string[];
  note: string;
  optedOut: boolean;
  blocked: boolean;
  source?: unknown;
}

export type WATicketStatus = "bot" | "open" | "pending" | "resolved";
export type WAPriority = "low" | "normal" | "high" | "urgent";

export interface WATicket {
  id: number;
  number: number;
  status: WATicketStatus;
  priority: WAPriority;
  category: string;
  tags: string[];
  owner?: WAPerson;
  teamId?: number;
  teamName?: string;
  awaitingSince?: string;
  waitingListedAt?: string;
  waitingCount: number;
  reopenCount: number;
  firstResponseAt?: string;
  resolvedAt?: string;
  resolvedBy?: WAPerson;
  rating?: number;
  ratingComment?: string;
  createdAt: string;
  participants: WAPerson[];
}

export interface WALast {
  id: number;
  direction: "in" | "out" | "note" | "event";
  kind: string;
  preview: string;
  at: string;
  senderName?: string;
  status: string;
}

export interface WAConversation {
  id: number;
  channelId: number;
  channelName: string;
  contact: WAContact;
  ticket?: WATicket;
  last?: WALast;
  unread: number;
  teamReadId: number;
  lastInboundAt?: string;
  windowEndsAt?: string;
  version: number;
}

export interface WAMedia {
  url: string;
  mime: string;
  name?: string;
  size?: number;
  voice?: boolean;
  animated?: boolean;
  failed?: string;
}

export interface WASender {
  kind: "customer" | "agent" | "bot" | "automation" | "system";
  userId?: number;
  name?: string;
  hasAvatar?: boolean;
  avatarVersion?: number;
  label?: string;
}

export interface WAMessage {
  id: number;
  conversationId: number;
  ticketId?: number;
  clientId?: string;
  direction: "in" | "out" | "note" | "event";
  kind: string;
  body: string;
  media?: WAMedia;
  payload?: unknown;
  replyTo?: { id: number; kind: string; body: string; sender: string };
  // the ad the customer came from
  referral?: WAReferral;
  status: "received" | "queued" | "sent" | "delivered" | "read" | "failed";
  errorText?: string;
  sender: WASender;
  reactions?: { emoji: string; ours: boolean; by?: string }[];
  createdAt: string;
  sentAt?: string;
  deliveredAt?: string;
  readAt?: string;
  // local only: a message on its way
  pending?: boolean;
}

export interface WAReferral {
  source_url?: string;
  source_type?: string;
  source_id?: string;
  headline?: string;
  body?: string;
  media_type?: string;
  image_url?: string;
  video_url?: string;
  thumbnail_url?: string;
}

export interface WAListResult {
  items: WAConversation[];
  hidden: number[];
  version: number;
  me: number;
}

export interface WAEvent {
  type: string;
  conversationId?: number;
  conversation?: WAConversation;
  message?: WAMessage;
  userId?: number;
  name?: string;
  text?: string;
  level?: string;
}

export interface WADayOpen {
  open: boolean;
  from: string;
  to: string;
}

export interface WASettings {
  readReceipts: boolean;
  greeting: { enabled: boolean; text: string; template: string; templateLang: string; forHelpers: boolean };
  distribution: { enabled: boolean; maxOpen: number };
  waitingMinutes: number;
  hours: { enabled: boolean; days: WADayOpen[]; holidays: string[] };
  survey: { mode: "off" | "tally" | "native"; url: string; text: string; template: string; templateLang: string; alertBelow: number; repeatHours: number };
  botTimeoutMinutes: number;
  humanKeywords: string[];
  optOutKeywords: string[];
  optOutReply: string;
}

export interface WAChannel {
  id: number;
  name: string;
  displayPhone: string;
  phoneNumberId: string;
  wabaId: string;
  appId: string;
  graphVersion: string;
  hasToken: boolean;
  hasAppSecret: boolean;
  verifyToken?: string;
  hookPath?: string;
  // a webhook already registered in Meta, used instead of hookPath
  existingHookUrl?: string;
  existingVerifyToken?: string;
  acceptUnsigned: boolean;
  surveyHookPath?: string;
  active: boolean;
  settings: WASettings;
  hasSurveySecret: boolean;
  verifiedName: string;
  qualityRating: string;
  messagingLimit: string;
  lastWebhookAt?: string;
  lastError?: string;
  lastErrorAt?: string;
  memberIds: number[];
  createdAt: string;
}

export interface WAChannelCheck {
  ok: boolean;
  message: string;
  displayPhone?: string;
  verifiedName?: string;
  qualityRating?: string;
  messagingLimit?: string;
  subscribed: boolean;
  webhookSeen: boolean;
}

export interface WATemplateComponent {
  type: string;
  format?: string;
  text?: string;
  buttons?: { type: string; text: string; url?: string; phone_number?: string }[];
}

export interface WATemplate {
  id: number;
  name: string;
  language: string;
  category: "MARKETING" | "UTILITY" | "AUTHENTICATION" | string;
  status: string;
  components: WATemplateComponent[];
  rejectedReason?: string;
  quality?: string;
  // what fills each body blank when sent: "" typed by hand, "customer",
  // "agent" (sender's first name) or "agent_full"
  fill: WATemplateFill[];
  updatedAt: string;
}

export type WATemplateFill = "" | "customer" | "agent" | "agent_full";

export interface WATeam {
  id: number;
  name: string;
  color: string;
  memberIds: number[];
}

export interface WAAgent extends WAPerson {
  channelIds: number[];
  online: boolean;
  available: boolean;
  canReply: boolean;
}

export interface WAQuickReply {
  id: number;
  shortcut: string;
  title: string;
  body: string;
  channelIds: number[];
}

export interface WARuleCondition {
  kind: string;
  value: string;
}

export interface WARuleAction {
  kind: string;
  text?: string;
  templateId?: number;
  params?: string[];
  teamId?: number;
  userId?: number;
  value?: string;
  url?: string;
}

export interface WARule {
  id: number;
  name: string;
  active: boolean;
  channelIds: number[];
  trigger: string;
  conditions: WARuleCondition[];
  actions: WARuleAction[];
  cooldownMin: number;
  position: number;
  runs: number;
  lastRunAt?: string;
  lastError?: string;
  updatedAt: string;
}

export interface BotOption {
  id: string;
  label: string;
  description?: string;
}

export interface BotRule {
  var: string;
  op: string;
  value: string;
  // time_between: Turkey time, on these weekdays (0 is Monday, none is every day)
  from?: string;
  to?: string;
  days?: number[];
}

export interface TimeSpan {
  days: number[];
  from: string;
  to: string;
}

// When a chatbot answers.
export interface BotSchedule {
  mode: "always" | "hours" | "off_hours" | "custom";
  spans: TimeSpan[];
}

export interface BotData {
  text?: string;
  mediaUrl?: string;
  fileId?: number;
  fileName?: string;
  mediaKind?: string;
  style?: "buttons" | "list";
  buttonLabel?: string;
  options?: BotOption[];
  var?: string;
  validate?: string;
  retry?: string;
  match?: "all" | "any";
  rules?: BotRule[];
  integration?: number;
  map?: { var: string; path: string }[];
  tags?: string[];
  priority?: string;
  category?: string;
  teamId?: number;
  note?: string;
  resolve?: boolean;
}

export type BotNodeType = "start" | "message" | "menu" | "ask" | "condition" | "api" | "tag" | "handoff" | "callback" | "survey" | "end";

export interface BotNode {
  id: string;
  type: BotNodeType;
  x: number;
  y: number;
  data: BotData;
}

export interface BotEdge {
  id: string;
  from: string;
  port: string;
  to: string;
}

export interface BotGraph {
  nodes: BotNode[];
  edges: BotEdge[];
}

export interface WABot {
  id: number;
  name: string;
  description: string;
  active: boolean;
  channelIds: number[];
  trigger: "entry" | "after_hours" | "keyword";
  keywords: string[];
  schedule: BotSchedule;
  draft: BotGraph;
  publishedVersion: number;
  publishedAt?: string;
  draftChanged: boolean;
  updatedAt: string;
}

export interface SimOutput {
  kind: string;
  text?: string;
  options?: BotOption[];
  style?: string;
  detail?: string;
}

export interface SimResult {
  outputs: SimOutput[];
  nodeId: string;
  vars: Record<string, string>;
  tries: number;
  done: boolean;
  // what the test assumed: in working hours or not, by which device
  hoursOpen: boolean;
  channel?: string;
}

export interface BotStats {
  started: number;
  handoffs: number;
  ended: number;
  timeouts: number;
  nodes: Record<string, number>;
  fails: Record<string, number>;
  drops: Record<string, number>;
}

export interface WAIntegration {
  id: number;
  name: string;
  method: string;
  url: string;
  body: string;
  timeoutSec: number;
  headerNames: string[];
}

export interface WACallback {
  id: number;
  channelName: string;
  conversationId: number;
  customer: string;
  phone: string;
  note: string;
  status: "open" | "done";
  doneBy?: string;
  doneAt?: string;
  createdAt: string;
}

export interface WAEventRow {
  id: number;
  channelId?: number;
  status: string;
  attempts: number;
  lastError: string;
  receivedAt: string;
  processedAt?: string;
  summary: string;
}

export interface WAHistoryItem {
  conversationId: number;
  ticketId: number;
  number: number;
  channelName: string;
  status: string;
  owner: string;
  createdAt: string;
  resolvedAt?: string;
  rating?: number;
  messages: number;
}

export interface WASearchHit {
  conversationId: number;
  messageId: number;
  contact: string;
  snippet: string;
  at: string;
}

export interface WAReadInfo {
  user: WAPerson;
  messageId: number;
  readAt: string;
}

export interface WAAgentReport {
  user: WAPerson;
  owned: number;
  helped: number;
  resolved: number;
  messages: number;
  avgFirstReplySec: number;
  avgResolveSec: number;
  waitingEntries: number;
  ratings: number;
  avgRating: number;
}

export interface WAChannelReport {
  id: number;
  name: string;
  tickets: number;
  resolved: number;
  botResolved: number;
  inbound: number;
  outbound: number;
  failed: number;
  avgFirstReplySec: number;
  waitingEntries: number;
  avgRating: number;
}

export interface WAReport {
  from: string;
  to: string;
  agents: WAAgentReport[];
  channels: WAChannelReport[];
  hours: number[];
}

export interface WAFile {
  id: number;
  name: string;
  mime: string;
  size: number;
  kind: "image" | "video" | "audio" | "document";
  url: string;
}

export interface WAAISettings {
  enabled: boolean;
  model: string;
  instructions: string;
  useQuickReplies: boolean;
  hasKey: boolean;
  models: { id: string; label: string }[];
}

export interface WACallSurveySettings {
  enabled: boolean;
  channelId: number;
  template: string;
  templateLang: string;
  params: string[];
  mode: "buttons" | "link";
  buttonScores: number[];
  linkUrl: string;
  directions: "inbound" | "outbound" | "both";
  minSeconds: number;
  delayMinutes: number;
  quietDays: number;
  alertBelow: number;
  thankYou: string;
}

export interface WACallSurveyReport {
  queued: number;
  sent: number;
  answered: number;
  failed: number;
  skipped: number;
  average: number;
  agents: { user: WAPerson; sent: number; answered: number; average: number; low: number }[];
  recent: { id: number; agent: string; phone: string; score: number; comment: string; conversationId: number; answeredAt: string }[];
}

// A person's own WhatsApp preferences.
export interface WAPrefs {
  sound: boolean;
  desktop: boolean;
  mutedUntil?: string;
  conversations: { id: number; mutedUntil?: string; pinnedAt?: string }[];
}

export type WAMute = "1h" | "8h" | "1d" | "1w" | "always" | "off";

// One score a customer gave: at the end of a WhatsApp conversation, or in
// the survey after a phone call.
export interface WARating {
  source: "chat" | "call";
  at: string;
  score: number;
  comment?: string;
  customer: string;
  phone: string;
  conversationId?: number;
  ticketNumber?: number;
  channel?: string;
  agent?: WAPerson;
  talkSeconds?: number;
  // each question of the form; a one-question survey is "Tek soruluk anket"
  answers: { question: string; score: number }[];
  // written answers, each under its question
  texts: { question: string; text: string }[];
}

export interface WARatingQuestion {
  question: string;
  count: number;
  average: number;
  dist: number[];
}

export interface WARatings {
  count: number;
  average: number;
  dist: number[]; // how many 1s, 2s ... 5s
  withComment: number;
  agents: { agent: WAPerson; count: number; average: number; low: number; questions: { question: string; count: number; average: number }[] }[];
  questions: WARatingQuestion[];
  items: WARating[];
  total: number;
  pageSize: number;
}

export interface WARatingFilter {
  from: string;
  to: string;
  channel?: number;
  agent?: number;
  source?: "" | "chat" | "call";
  score?: string;
  comment?: boolean;
  q?: string;
  page?: number;
}
