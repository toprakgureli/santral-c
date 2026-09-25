export interface User {
  id: number;
  name: string;
  email: string;
  active: boolean;
  roles: string[];
  roleIds: number[];
  permissions: string[];
  mfaEnabled: boolean;
  mustChangePassword: boolean;
  sipExtension?: string;
  whatsappTemplate?: string;
  whatsappTemplateLive?: string;
  hasAvatar?: boolean;
  avatarVersion?: number;
  lastLoginAt?: string;
  createdAt: string;
}

export interface LoginAttempt {
  id: number;
  email: string;
  ip: string;
  userAgent: string;
  success: boolean;
  reason: string;
  createdAt: string;
}

export interface IPBan {
  id: number;
  ip: string;
  reason: string;
  attempts: number;
  until: string;
}

export interface AuditEntry {
  id: number;
  actorId?: number;
  actorName: string;
  actorEmail: string;
  action: string;
  targetType: string;
  targetId: string;
  ip: string;
  detail: Record<string, unknown>;
  createdAt: string;
}

export type MfaMode = "on" | "off" | "trusted";

export interface SystemSettings {
  mfaMode: MfaMode;
  mfaTrustedIps: string[];
  clientIp?: string;
}

export interface LoginChallenge {
  mfaRequired?: boolean;
  mfaSetupRequired?: boolean;
  mfaToken?: string;
  passwordChangeRequired?: boolean;
  passwordToken?: string;
}

export interface LoginResult {
  user?: User;
  challenge?: LoginChallenge;
}

export interface Role {
  id: number;
  name: string;
  displayName: string;
  description: string;
  system: boolean;
  userCount: number;
  permissionIds: number[];
}

export interface PermissionItem {
  id: number;
  key: string;
  description: string;
}

export interface PermissionGroup {
  module: string;
  label: string;
  items: PermissionItem[];
}

// "off" is reported while the agent has no open shift; it cannot be chosen.
export type AgentPresenceState = "available" | "break" | "backoffice" | "dnd" | "off";

export interface Shift {
  id: number;
  startedAt: string;
  endedAt?: string;
  endedBy?: "user" | "auto";
}

export interface ShiftStatus {
  shift: Shift | null;
  reminderAt?: string;
  autoEndAt?: string;
}

export interface PresencePause {
  state: AgentPresenceState;
  startedAt: string;
  endedAt?: string;
}

export interface AgentPresence {
  state: AgentPresenceState;
  since?: string;
  totals?: Record<string, number>;
  talk?: number;
  online?: number;
  pauses?: PresencePause[];
  breakLimit?: number;
}

export interface TodayCalls {
  items: Call[];
  short: number;
  long: number;
  unanswered: number;
  inbound: number;
  outbound: number;
  inboundMissed: number;
  outboundMissed: number;
  inboundReal: number;
  outboundReal: number;
}

// Team performance page (performance.view_role / performance.view_all).
export type TeamStatus = "talking" | "available" | "break" | "backoffice" | "dnd" | "off" | "unregistered";

export interface TeamCounts {
  total: number;
  answered: number;
  short: number;
  long: number;
  unanswered: number;
  inbound: number;
  outbound: number;
  inboundMissed: number;
  outboundMissed: number;
  inboundReal: number;
  outboundReal: number;
  talkSeconds: number;
  avgTalkSeconds: number;
  longestSeconds: number;
}

export interface TeamRow {
  userId: number;
  name: string;
  extension: string;
  roles: string[];
  status: TeamStatus;
  since?: string;
  call?: { peer: string; peerName?: string; direction: string; startedAt: string };
  shift: { startedAt?: string; firstStart?: string; lastEnd?: string; open: boolean; seconds: number };
  calls: TeamCounts;
  escalations: number;
  breakSeconds: number;
  recent: RecentCall[];
}

// RecentCall is one of an agent's latest calls on the team page.
export interface RecentCall {
  peer: string;
  peerName?: string;
  direction: string;
  disposition: string;
  startedAt: string;
  durationSeconds: number;
}

// CallLookup answers who spoke with a number.
export interface CallLookup {
  number: string;
  scope: "all" | "own";
  days: number;
  total: number;
  answered: number;
  talkSeconds: number;
  lastAt?: string;
  agents: { id: number; name: string; calls: number; answered: number; talkSeconds: number }[];
  items: { uuid: string; direction: string; disposition: string; agentId: number; agentName: string; startedAt: string; durationSeconds: number }[];
}

export interface TeamPerformance {
  scope: "all" | "role";
  from: string;
  to: string;
  items: TeamRow[];
}

export interface ProfileStats {
  totalReal: number;
  totalTalkSeconds: number;
  totalEscalations: number;
}

// ProfileRecord is the call-centre record over a day range.
export interface ProfileRecord {
  from: string;
  to: string;
  real: number;
  inboundReal: number;
  outboundReal: number;
  unanswered: number;
  short: number;
  talkSeconds: number;
  avgTalkSeconds: number;
  longestSeconds: number;
  escalations: number;
  shiftSeconds: number;
  breakSeconds: number;
  busiestHour: number;
  days: { day: string; real: number }[];
}

export interface Profile {
  id: number;
  name: string;
  email: string;
  headline: string;
  bio: string;
  hasAvatar: boolean;
  avatarVersion?: number;
  roles: string[];
  extension?: string;
  active: boolean;
  joinedAt: string;
  stats: ProfileStats;
  editable: boolean;
}

// Teams (in-house chat)
export interface TeamsPerson {
  id: number;
  name: string;
  hasAvatar: boolean;
  avatarVersion?: number;
  online?: boolean;
  lastSeen?: string;
  // Looking at the room this card is shown in.
  inRoom?: boolean;
}

export interface TeamsReceipt extends TeamsPerson {
  deliveredAt?: string;
  readAt?: string;
}

export interface TeamsReaction {
  emoji: string;
  count: number;
  mine: boolean;
  names: string[];
  people: TeamsPerson[];
}

export interface TeamsAttachment {
  id: number;
  kind: "image" | "video" | "file";
  name: string;
  mime: string;
  size: number;
  width?: number;
  height?: number;
  durationMs?: number;
  hasThumb: boolean;
  ready: boolean;
}

export interface TeamsMediaItem extends TeamsAttachment {
  messageId: number;
  sender: string;
  createdAt: string;
}

export interface DriveStatus {
  configured: boolean;
  connected: boolean;
  account: string;
  folder: string;
  limit: number;
  usage: number;
  error?: string;
}

export interface TeamsMessage {
  id: number;
  groupId: number;
  kind: "text" | "system" | "game";
  body: string;
  // A game card: the match the line stands for.
  gameId?: number;
  sender?: TeamsPerson;
  replyTo?: { id: number; body: string; sender: string; deleted: boolean };
  deleted: boolean;
  mine: boolean;
  canDelete: boolean;
  reactions: TeamsReaction[];
  mentions: number[];
  mentionsAll: boolean;
  attachments: TeamsAttachment[];
  editedAt?: string;
  createdAt: string;
  // Only on the reader's own lines.
  status?: "sent" | "delivered" | "read";
  readBy?: string[];
}

export interface TeamsGroup {
  id: number;
  kind: "group" | "dm";
  name: string;
  description: string;
  hasAvatar: boolean;
  avatarVersion?: number;
  postPolicy: "everyone" | "admins";
  myRole: "owner" | "admin" | "member" | "";
  canPost: boolean;
  canManage: boolean;
  canAdd: boolean;
  canInvite: boolean;
  canDelete: boolean;
  muted: boolean;
  mute: "none" | "mentions" | "all";
  unread: number;
  memberCount: number;
  peer?: TeamsPerson;
  lastMessage?: TeamsMessage;
  updatedAt: string;
}

export interface TeamsMember extends TeamsPerson {
  role: "owner" | "admin" | "member";
  canPost: boolean;
  joinedAt: string;
  deliveredId: number;
  readId: number;
}

export interface TeamsGroupDetail extends TeamsGroup {
  members: TeamsMember[];
  invited: TeamsPerson[];
  editable: boolean;
}

export interface TeamsInvite {
  id: number;
  groupId: number;
  groupName: string;
  hasAvatar: boolean;
  avatarVersion?: number;
  invitedBy?: TeamsPerson;
  createdAt: string;
}

export interface TeamsOverview {
  groups: TeamsGroup[];
  invites: TeamsInvite[];
  unread: number;
}

export interface TeamsEvent {
  type: "hello" | "message" | "message.edited" | "message.deleted" | "reaction" | "group" | "invite" | "presence" | "receipt" | "typing" | "game" | "game.stroke" | "game.frame" | "game.invite";
  groupId?: number;
  message?: TeamsMessage;
  id?: number;
  // presence
  userId?: number;
  online?: boolean;
  lastSeen?: string;
  // receipt
  deliveredId?: number;
  readId?: number;
  // typing (name), presence (room the person now looks at, 0 for none)
  name?: string;
  room?: number;
  // reaction
  emoji?: string;
  added?: boolean;
  senderId?: number;
  // game (id carries the version), game.stroke and game.frame (payload)
  gameId?: number;
  payload?: unknown;
}

export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  perPage: number;
}

export interface ContactPhone {
  id: number;
  label: string;
  number: string;
  isPrimary: boolean;
}

export interface Contact {
  id: number;
  name: string;
  company?: string;
  email?: string;
  notes?: string;
  phones: ContactPhone[];
  createdAt: string;
  updatedAt: string;
}

export interface Call {
  uuid: string;
  direction: string;
  disposition: string;
  fromNumber: string;
  toNumber: string;
  startedAt: string;
  durationSeconds: number;
  recording: boolean;
}

export interface CallPage {
  items: Call[];
  page: number;
  total: number;
  totalPages: number;
}

export interface Webphone {
  extension: string;
  url: string;
}

export interface PBXExtension {
  extension: string;
  status: string;
  names?: string[];
  peer?: string;
  peerName?: string;
}

export interface PBXQueue {
  number: string;
  name: string;
}

export interface PBXStats {
  total: number;
  missed: number;
}

export interface EscalationReason {
  id: number;
  name: string;
}

export interface EscalationCategory {
  id: number;
  name: string;
  reasons: EscalationReason[];
}

export interface EscalationRecord {
  id: number;
  number: string;
  categoryName: string;
  reasonName: string;
  note: string;
  agentId?: number;
  agentName: string;
  createdAt: string;
}

export interface EscalationListPage extends Paged<EscalationRecord> {
  scope: "all" | "own";
}

export interface SipCredentials {
  extension: string;
  password: string;
  domain: string;
  webSocketUrl: string;
  stunUrl?: string;
  turnUrl?: string;
  turnUser?: string;
  turnPass?: string;
}
