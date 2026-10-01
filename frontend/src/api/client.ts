import type {
  AgentPresence,
  AgentPresenceState,
  AuditEntry,
  AgentCalls,
  CallLookup,
  CallPage,
  Contact,
  TodayCalls,
  EscalationCategory,
  EscalationReason,
  EscalationRecord,
  EscalationListPage,
  IPBan,
  LoginAttempt,
  LoginResult,
  Paged,
  PBXExtension,
  PBXQueue,
  PBXStats,
  PermissionGroup,
  Profile,
  ProfileRecord,
  DriveStatus,
  TeamsAttachment,
  TeamsGroupDetail,
  TeamsHistory,
  TeamsMediaItem,
  TeamsMessage,
  TeamsOverview,
  TeamsPerson,
  TeamsReceipt,
  Role,
  ShiftStatus,
  SipCredentials,
  SystemSettings,
  TeamPerformance,
  User,
} from "./types";

const BASE = "/api/v1";

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

// SESSION_ENDED is dispatched on window when the server rejects the session
// and it cannot be renewed (the user was deactivated, signed out elsewhere or
// the refresh token expired). AuthContext listens and returns to sign-in.
export const SESSION_ENDED = "santral:session-ended";

// BackupView is the database backup settings and the last copies.
export interface BackupView {
  enabled: boolean;
  folderId: string;
  account: string;
  hasKey: boolean;
  everyHours: number;
  running: boolean;
  pgDumpExists: boolean;
  runs: { id: number; startedAt: string; finishedAt?: string; ok: boolean; file: string; size: number; error?: string; manual: boolean }[];
}

// SystemWarning is one of the server's system warnings (system.health).
export interface SystemWarning {
  key: string;
  level: "warning" | "critical";
  title: string;
  text: string;
  action: string;
  link?: string;
  fingerprint: string;
}

// SystemHealth is the server's current list of system warnings.
export interface SystemHealth {
  warnings: SystemWarning[];
  checkedAt: string;
}

// BackupCheck is what the backup account may do in its folder.
export interface BackupCheck {
  // canLock is there once the check has tried locking a small test file.
  folder: { name: string; sharedDrive: boolean; canAdd: boolean; canDelete: boolean; canLock?: boolean; lockError?: string };
  problem: string;
}

// SipSyncJob is the background pull of every extension's SIP password.
export interface SipSyncJob {
  running: boolean;
  total: number;
  done: number;
  synced: number;
  failures: { extension: string; reason: string }[];
}

// ensureSession renews the session if it can. Live streams call it after
// their connection drops: "renewed" means reconnect now, "ended" means the
// sign-in screen is showing, "unknown" means try again a little later.
export async function ensureSession(): Promise<Renewal> {
  try {
    const r = await fetch(BASE + "/auth/me", { credentials: "include" });
    if (r.ok) return "renewed";
    if (r.status !== 401) return "unknown";
  } catch {
    return "unknown";
  }
  const renewal = await tryRefresh();
  if (renewal === "ended") endSession();
  return renewal;
}

// The outcome of renewing the session: renewed, over (the server said the
// session is gone), or not known yet (network, overload, a deploy).
type Renewal = "renewed" | "ended" | "unknown";

// A single in-flight refresh is shared by all concurrent 401s in this tab, and
// tabs take turns through a browser lock, so the session survives silently
// (the access token is short-lived; the refresh token is not).
let refreshing: Promise<Renewal> | null = null;

// When another tab renewed within this window, the shared cookie is already
// fresh and this tab only needs to retry its request.
const RENEWED_KEY = "santral.renewedAt";
const RENEWED_FRESH_MS = 10_000;

function renewedLately(): boolean {
  try {
    const at = Number(localStorage.getItem(RENEWED_KEY) ?? 0);
    return Date.now() - at < RENEWED_FRESH_MS;
  } catch {
    return false;
  }
}

function markRenewed() {
  try {
    localStorage.setItem(RENEWED_KEY, String(Date.now()));
  } catch {
    // storage may be unavailable; the server tolerates a second renewal
  }
}

const sleep = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));

// renewOnce asks the server for a new access token. Only 401 and 403 mean the
// session is over; anything else is tried again a few times and then left
// undecided, so a network blip never signs anyone out or drops a call.
async function renewOnce(): Promise<Renewal> {
  const waits = [0, 1000, 3000, 7000];
  for (const wait of waits) {
    if (wait) await sleep(wait);
    try {
      const r = await fetch(BASE + "/auth/refresh", { method: "POST", credentials: "include" });
      if (r.ok) {
        markRenewed();
        return "renewed";
      }
      if (r.status === 401 || r.status === 403) return "ended";
    } catch {
      // network error: try again
    }
  }
  return "unknown";
}

function tryRefresh(): Promise<Renewal> {
  if (!refreshing) {
    const run = async (): Promise<Renewal> => (renewedLately() ? "renewed" : renewOnce());
    const locks = (navigator as Navigator & { locks?: LockManager }).locks;
    const turn: Promise<Renewal> = locks ? (locks.request("santral-refresh", run) as unknown as Promise<Renewal>) : run();
    const shared = turn.finally(() => {
      refreshing = null;
    });
    refreshing = shared;
    return shared;
  }
  return refreshing;
}

// renewable reports whether a 401 on path may be fixed by a refresh. The
// sign-in steps answer 401 for a wrong password or code, which a refresh
// cannot change; /auth/me is an ordinary session call.
function renewable(path: string): boolean {
  return !path.startsWith("/auth/") || path === "/auth/me";
}

function endSession() {
  window.dispatchEvent(new Event(SESSION_ENDED));
}

function parseBody(text: string): { code?: string; message?: string } | undefined {
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    // A proxy error page is not JSON; the status still tells what happened.
    return undefined;
  }
}

export async function request<T>(path: string, options: RequestInit = {}, allowRetry = true): Promise<T> {
  // FormData bodies must keep the browser-set multipart Content-Type (with its
  // boundary); only default to JSON for the rest.
  const isForm = options.body instanceof FormData;
  const baseHeaders: Record<string, string> = isForm ? {} : { "Content-Type": "application/json" };
  const res = await fetch(BASE + path, {
    ...options,
    credentials: "include",
    headers: { ...baseHeaders, ...((options.headers as Record<string, string>) ?? {}) },
  });
  // Access token expired: refresh once (using the long-lived refresh cookie) and
  // retry, so the user is not logged out mid-session. Only a session the
  // server says is over ends here; an undecided renewal fails this request
  // and keeps the user signed in.
  if (res.status === 401 && renewable(path)) {
    const renewal = allowRetry ? await tryRefresh() : "ended";
    if (renewal === "renewed") {
      return request<T>(path, options, false);
    }
    if (renewal === "ended") {
      endSession();
    } else {
      throw new ApiError(503, "UNAVAILABLE", "Sunucuya şu an ulaşılamıyor. Birazdan tekrar dene.");
    }
  }
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (!res.ok) {
    const body = parseBody(text);
    throw new ApiError(res.status, body?.code ?? "ERROR", body?.message ?? "İstek başarısız oldu.");
  }
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    // A proxy page instead of our answer.
    throw new ApiError(502, "BAD_RESPONSE", "Sunucudan beklenmeyen bir yanıt geldi. Birazdan tekrar dene.");
  }
}

// download fetches a file endpoint (same auth/refresh handling as request) and
// hands it to the browser as a save dialog.
export async function download(path: string, fallbackName: string, allowRetry = true): Promise<void> {
  const res = await fetch(BASE + path, { credentials: "include" });
  if (res.status === 401) {
    const renewal = allowRetry ? await tryRefresh() : "ended";
    if (renewal === "renewed") {
      return download(path, fallbackName, false);
    }
    if (renewal === "ended") {
      endSession();
    } else {
      throw new ApiError(503, "UNAVAILABLE", "Sunucuya şu an ulaşılamıyor. Birazdan tekrar dene.");
    }
  }
  if (!res.ok) {
    const body = parseBody(await res.text());
    throw new ApiError(res.status, body?.code ?? "ERROR", body?.message ?? "İndirme başarısız oldu.");
  }
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  const match = /filename="([^"]+)"/.exec(disposition);
  let name = match?.[1] ?? fallbackName;
  if (encoded) {
    try {
      name = decodeURIComponent(encoded[1]);
    } catch {
      // keep the plain name
    }
  }
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Some browsers read the file only after click() returns.
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

export const api = {
  // Auth
  login: (email: string, password: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) })),
  passwordChange: (token: string, password: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/password/change", { method: "POST", body: JSON.stringify({ token, password }) })),
  mfaVerify: (token: string, code: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/mfa/verify", { method: "POST", body: JSON.stringify({ token, code }) })),
  mfaEnroll: (token: string) =>
    request<{ secret: string; url: string; qr: string }>("/auth/mfa/enroll", { method: "POST", body: JSON.stringify({ token }) }),
  mfaEnrollVerify: (token: string, code: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/mfa/enroll/verify", { method: "POST", body: JSON.stringify({ token, code }) })),
  me: () => request<User>("/auth/me"),
  logout: () => request<void>("/auth/logout", { method: "POST" }),
  // The user's own password; other devices are signed out, this one gets a
  // fresh session.
  changeOwnPassword: (current: string, password: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/password", { method: "POST", body: JSON.stringify({ current, password }) })),
  logoutEverywhere: () => request<void>("/auth/logout/everywhere", { method: "POST" }),

  // Build stamp of the running backend (public), to compare against the frontend.
  version: () => request<{ version: string; buildTime: string; build?: string }>("/version"),

  // Users
  listUsers: (params: { query?: string; roleId?: number | string; active?: string; page?: number; perPage?: number } = {}) =>
    request<Paged<User>>("/users/" + query(params)),
  createUser: (body: { name: string; email: string; password: string; roleIds: number[]; sipExtension?: string }) =>
    request<User>("/users/", { method: "POST", body: JSON.stringify(body) }),
  updateUser: (id: number, body: { name: string; email: string; roleIds: number[] }) =>
    request<User>(`/users/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  setUserActive: (id: number, active: boolean) =>
    request<void>(`/users/${id}/active`, { method: "PATCH", body: JSON.stringify({ active }) }),
  resetUserPassword: (id: number, password: string) =>
    request<void>(`/users/${id}/password`, { method: "POST", body: JSON.stringify({ password }) }),
  setUserSip: (id: number, extension: string, password: string) =>
    request<void>(`/users/${id}/sip`, { method: "POST", body: JSON.stringify({ extension, password }) }),
  // Teams
  teamsPeople: () => request<{ items: TeamsPerson[] }>("/teams/people").then((r) => r.items),
  teamsOverview: () => request<TeamsOverview>("/teams/overview"),
  teamsCreateGroup: (body: { name: string; description: string; postPolicy: string; memberIds: number[] }) =>
    request<TeamsGroupDetail>("/teams/groups", { method: "POST", body: JSON.stringify(body) }),
  teamsOpenDM: (userId: number) => request<TeamsGroupDetail>(`/teams/dm/${userId}`, { method: "POST" }),
  teamsGroup: (id: number) => request<TeamsGroupDetail>(`/teams/groups/${id}`),
  teamsUpdateGroup: (id: number, body: { name: string; description: string; postPolicy: string }) =>
    request<TeamsGroupDetail>(`/teams/groups/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  teamsSetGroupAvatar: (id: number, avatar: string) =>
    request<TeamsGroupDetail>(`/teams/groups/${id}/avatar`, { method: "PUT", body: JSON.stringify({ avatar }) }),
  teamsDeleteGroup: (id: number) => request<void>(`/teams/groups/${id}`, { method: "DELETE" }),
  teamsAddMembers: (id: number, userIds: number[], history: TeamsHistory) =>
    request<TeamsGroupDetail>(`/teams/groups/${id}/members`, { method: "POST", body: JSON.stringify({ userIds, history }) }),
  teamsInvite: (id: number, userIds: number[], history: TeamsHistory) =>
    request<TeamsGroupDetail>(`/teams/groups/${id}/invites`, { method: "POST", body: JSON.stringify({ userIds, history }) }),
  teamsDecideInvite: (inviteId: number, decision: "accept" | "decline") =>
    request<void>(`/teams/invites/${inviteId}/${decision}`, { method: "POST" }),
  teamsUpdateMember: (id: number, userId: number, body: { role?: string; canPost?: boolean }) =>
    request<TeamsGroupDetail>(`/teams/groups/${id}/members/${userId}`, { method: "PUT", body: JSON.stringify(body) }),
  teamsRemoveMember: (id: number, userId: number) => request<void>(`/teams/groups/${id}/members/${userId}`, { method: "DELETE" }),
  teamsMute: (id: number, mute: "none" | "mentions" | "all") => request<void>(`/teams/groups/${id}/mute`, { method: "POST", body: JSON.stringify({ mute }) }),
  teamsMarkUnread: (id: number) => request<void>(`/teams/groups/${id}/unread`, { method: "POST" }),
  // messageId 0 marks everything read.
  teamsMarkRead: (id: number, messageId: number) => request<void>(`/teams/groups/${id}/read`, { method: "POST", body: JSON.stringify({ messageId }) }),
  teamsMessages: (id: number, opts: { before?: number; around?: number; after?: number } = {}) =>
    request<{ items: TeamsMessage[]; more: boolean; moreNewer: boolean }>(`/teams/groups/${id}/messages` + query(opts)),
  teamsSearch: (id: number, q: string) => request<{ items: TeamsMessage[] }>(`/teams/groups/${id}/search` + query({ q })).then((r) => r.items),
  teamsMedia: (id: number, kind: "image" | "video" | "file", before?: number) =>
    request<{ items: TeamsMediaItem[]; more: boolean }>(`/teams/groups/${id}/media` + query({ kind, before })),
  teamsSend: (id: number, body: { body: string; replyToId?: number; mentionIds?: number[]; mentionsAll?: boolean; attachmentIds?: number[] }) =>
    request<TeamsMessage>(`/teams/groups/${id}/messages`, { method: "POST", body: JSON.stringify(body) }),
  teamsEditMessage: (id: number, messageId: number, body: { body: string; mentionIds?: number[]; mentionsAll?: boolean }) =>
    request<TeamsMessage>(`/teams/groups/${id}/messages/${messageId}`, { method: "PUT", body: JSON.stringify(body) }),
  teamsBeginUpload: (id: number, body: { name: string; mime: string; size: number }) =>
    request<{ attachmentId: number; uploadUrl: string; chunkBytes: number }>(`/teams/groups/${id}/uploads`, { method: "POST", body: JSON.stringify(body) }),
  teamsFinishUpload: (attachmentId: number, body: { driveId: string; width?: number; height?: number; durationMs?: number; thumb?: string }) =>
    request<TeamsAttachment>(`/teams/uploads/${attachmentId}/finish`, { method: "POST", body: JSON.stringify(body) }),
  teamsCancelUpload: (attachmentId: number) => request<void>(`/teams/uploads/${attachmentId}`, { method: "DELETE" }),
  driveStatus: () => request<DriveStatus>("/teams/drive/status"),
  driveDisconnect: () => request<void>("/teams/drive/disconnect", { method: "POST" }),
  teamsTyping: (id: number) => request<void>(`/teams/groups/${id}/typing`, { method: "POST" }),
  teamsPresence: (room: number) => request<void>("/teams/presence", { method: "POST", body: JSON.stringify({ room }) }),
  teamsReceipts: (id: number, messageId: number) => request<{ items: TeamsReceipt[] }>(`/teams/groups/${id}/messages/${messageId}/receipts`).then((r) => r.items),
  teamsDeleteMessage: (id: number, messageId: number) => request<void>(`/teams/groups/${id}/messages/${messageId}`, { method: "DELETE" }),
  teamsReact: (id: number, messageId: number, emoji: string) =>
    request<void>(`/teams/groups/${id}/messages/${messageId}/reactions`, { method: "POST", body: JSON.stringify({ emoji }) }),

  myProfile: () => request<Profile>("/profile/me"),
  profileOf: (id: number) => request<Profile>(`/profile/${id}`),
  profileRecord: (id: number | "me", params: { from: string; to: string }) => request<ProfileRecord>(`/profile/${id}/record` + query(params)),
  updateMyProfile: (body: { headline: string; bio: string }) => request<Profile>("/profile/me", { method: "PUT", body: JSON.stringify(body) }),
  setMyAvatar: (avatar: string) => request<User>("/users/me/avatar", { method: "PUT", body: JSON.stringify({ avatar }) }),
  setMyWhatsAppTemplates: (body: { template: string; live: string }) =>
    request<User>("/users/me/whatsapp-template", { method: "PUT", body: JSON.stringify(body) }),
  syncUserSip: (id: number, extension: string) =>
    request<void>(`/users/${id}/sip/sync`, { method: "POST", body: JSON.stringify({ extension }) }),
  syncAllSip: () => request<SipSyncJob>("/pbx/sip/sync-all", { method: "POST" }),

  // Database backups
  backupSettings: () => request<BackupView>("/backup/"),
  saveBackup: (body: { enabled: boolean; folderId: string; credentials: string }) =>
    request<BackupView>("/backup/", { method: "PUT", body: JSON.stringify(body) }),
  checkBackup: () => request<BackupCheck>("/backup/check", { method: "POST" }),
  runBackup: () => request<BackupView>("/backup/run", { method: "POST" }),

  // System warnings (system.health)
  systemHealth: () => request<SystemHealth>("/system/health"),
  syncAllSipStatus: () => request<SipSyncJob>("/pbx/sip/sync-all"),

  // Roles & permissions
  listRoles: () => request<{ items: Role[] }>("/roles").then((r) => r.items),
  rolePermissions: () => request<{ items: PermissionGroup[] }>("/roles/permissions").then((r) => r.items),
  createRole: (body: { name: string; displayName: string; description: string; permissionIds: number[] }) =>
    request<Role>("/roles/", { method: "POST", body: JSON.stringify(body) }),
  updateRole: (id: number, body: { displayName: string; description: string; permissionIds: number[] }) =>
    request<Role>(`/roles/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteRole: (id: number) => request<void>(`/roles/${id}`, { method: "DELETE" }),
  setUserRoles: (id: number, roleIds: number[]) =>
    request<User>(`/users/${id}/roles`, { method: "PATCH", body: JSON.stringify({ roleIds }) }),

  // Security page (system.logs) and system settings (system.settings)
  securityAttempts: (params: { page?: number; perPage?: number; success?: string; email?: string } = {}) =>
    request<Paged<LoginAttempt>>("/security/attempts" + query(params)),
  securityBans: () => request<{ items: IPBan[] }>("/security/bans").then((r) => r.items),
  removeBan: (id: number) => request<void>(`/security/bans/${id}`, { method: "DELETE" }),
  systemSettings: () => request<SystemSettings>("/settings/"),
  updateSystemSettings: (body: { mfaMode: SystemSettings["mfaMode"]; mfaTrustedIps: string[] }) => request<SystemSettings>("/settings/", { method: "PUT", body: JSON.stringify(body) }),
  breakLimit: () => request<{ minutes: number }>("/settings/break-limit"),
  updateBreakLimit: (minutes: number) => request<{ minutes: number }>("/settings/break-limit", { method: "PUT", body: JSON.stringify({ minutes }) }),

  // Audit trail (system.audit_view)
  auditLogs: (params: { page?: number; perPage?: number; action?: string; query?: string } = {}) =>
    request<Paged<AuditEntry>>("/audit" + query(params)),

  // Contacts
  listContacts: (params: { query?: string; page?: number; perPage?: number } = {}) =>
    request<Paged<Contact>>("/contacts/" + query(params)),
  getContact: (id: number) => request<Contact>(`/contacts/${id}`),
  createContact: (body: unknown) => request<Contact>("/contacts/", { method: "POST", body: JSON.stringify(body) }),
  updateContact: (id: number, body: unknown) => request<Contact>(`/contacts/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteContact: (id: number) => request<void>(`/contacts/${id}`, { method: "DELETE" }),
  addContactPhone: (id: number, body: { label?: string; number: string; isPrimary?: boolean }) =>
    request<Contact>(`/contacts/${id}/phones`, { method: "POST", body: JSON.stringify(body) }),
  removeContactPhone: (id: number, phoneId: number) =>
    request<void>(`/contacts/${id}/phones/${phoneId}`, { method: "DELETE" }),
  lookupContact: (number: string) => request<Contact>("/contacts/lookup" + query({ number })),

  // Calls (Bulutsantralim CDR — full santral view)
  listCalls: (params: { direction?: string; number?: string; scope?: string; from?: string; to?: string; archive?: string; page?: number; perPage?: number } = {}) =>
    request<CallPage>("/calls" + query(params)),
  exportCalls: (params: { direction?: string; number?: string; scope?: string; from?: string; to?: string } = {}) =>
    download("/calls/export" + query(params), "cagrilar.csv"),
  originate: (to: string) => request<{ callUuid: string }>("/calls/originate", { method: "POST", body: JSON.stringify({ to }) }),
  // The softphone asks before handing a call over; the server checks the
  // permission and logs the hand-over.
  authorizeTransfer: (callId: string, target: string) =>
    request<void>("/calls/transfer", { method: "POST", body: JSON.stringify({ callId, target }) }),

  // Call log (our own store, used for the panel history — today, per agent)
  recentCalls: () => request<TodayCalls>("/calls/log/"),
  callLookup: (number: string) => request<CallLookup>("/calls/log/lookup" + query({ number })),
  logCall: (body: { callId: string; phase: "start" | "answer" | "end"; direction?: string; peer?: string; disposition?: string; durationSeconds?: number }) =>
    request<void>("/calls/log/", { method: "POST", body: JSON.stringify(body) }),

  // Softphone (SIP over WSS to Bulutsantralim)
  sipCredentials: () => request<SipCredentials>("/sip/credentials"),
  pbxExtensions: () => request<{ items: PBXExtension[] }>("/pbx/extensions").then((r) => r.items),
  pbxQueues: () => request<{ items: PBXQueue[] }>("/pbx/queues").then((r) => r.items),
  pbxStats: () => request<PBXStats>("/pbx/stats"),
  // Team performance (today's figures per agent, scoped by permission)
  performanceToday: (params: { from?: string; to?: string } = {}) => request<TeamPerformance>("/performance/today" + query(params)),
  agentCalls: (params: { userId: number; from: string; to: string }) => request<AgentCalls>("/performance/calls" + query(params)),

  // Shift (mesai): the dialer opens only while a shift is open
  shiftStatus: () => request<ShiftStatus>("/shift/"),
  startShift: () => request<ShiftStatus>("/shift/start", { method: "POST" }),
  endShift: () => request<ShiftStatus>("/shift/end", { method: "POST" }),

  getAgentStatus: () => request<AgentPresence>("/pbx/status"),
  setAgentStatus: (state: AgentPresenceState) =>
    request<{ state: AgentPresenceState; pbxPending?: boolean } | undefined>("/pbx/status", { method: "POST", body: JSON.stringify({ state }) }),

  // Escalations
  escalationCategories: () => request<{ items: EscalationCategory[] }>("/escalations/categories").then((r) => r.items),
  createEscalationCategory: (name: string) =>
    request<EscalationCategory>("/escalations/categories", { method: "POST", body: JSON.stringify({ name }) }),
  deleteEscalationCategory: (id: number) => request<void>(`/escalations/categories/${id}`, { method: "DELETE" }),
  reorderEscalationCategories: (ids: number[]) =>
    request<void>("/escalations/categories/order", { method: "PUT", body: JSON.stringify({ ids }) }),
  reorderEscalationReasons: (categoryId: number, ids: number[]) =>
    request<void>(`/escalations/categories/${categoryId}/reasons/order`, { method: "PUT", body: JSON.stringify({ ids }) }),
  createEscalationReason: (categoryId: number, name: string) =>
    request<EscalationReason>(`/escalations/categories/${categoryId}/reasons`, { method: "POST", body: JSON.stringify({ name }) }),
  deleteEscalationReason: (id: number) => request<void>(`/escalations/reasons/${id}`, { method: "DELETE" }),
  importEscalationCatalog: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<{ addedCategories: number; addedReasons: number }>("/escalations/categories/import", { method: "POST", body: form });
  },
  escalationHistory: (number: string) =>
    request<{ items: EscalationRecord[] }>("/escalations/" + query({ number })).then((r) => r.items),
  logEscalation: (body: { number: string; reasonId: number; note?: string; callUuid?: string }) =>
    request<EscalationRecord>("/escalations/", { method: "POST", body: JSON.stringify(body) }),
  listEscalations: (params: { number?: string; from?: string; to?: string; agentId?: number; categoryId?: number; page?: number; perPage?: number } = {}) =>
    request<EscalationListPage>("/escalations/list" + query(params)),
  escalationAgents: () => request<{ items: { id: number; name: string }[] }>("/escalations/agents").then((r) => r.items),
  logNoEscalation: (body: { number: string; callUuid?: string }) =>
    request<EscalationRecord>("/escalations/none", { method: "POST", body: JSON.stringify(body) }),
};

async function parseLogin(p: Promise<Record<string, unknown>>): Promise<LoginResult> {
  const body = await p;
  if (body.user) {
    return { user: body.user as User };
  }
  return {
    challenge: {
      mfaRequired: body.mfaRequired as boolean | undefined,
      mfaSetupRequired: body.mfaSetupRequired as boolean | undefined,
      mfaToken: body.mfaToken as string | undefined,
      passwordChangeRequired: body.passwordChangeRequired as boolean | undefined,
      passwordToken: body.passwordToken as string | undefined,
    },
  };
}
