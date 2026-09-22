import type {
  AgentPresence,
  AgentPresenceState,
  AuditEntry,
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

// A single in-flight refresh is shared by all concurrent 401s so the session
// survives silently (the access token is short-lived; the refresh token is not).
let refreshing: Promise<boolean> | null = null;

function tryRefresh(): Promise<boolean> {
  if (!refreshing) {
    refreshing = fetch(BASE + "/auth/refresh", { method: "POST", credentials: "include" })
      .then((r) => r.ok)
      .catch(() => false)
      .finally(() => { refreshing = null; });
  }
  return refreshing;
}

async function request<T>(path: string, options: RequestInit = {}, allowRetry = true): Promise<T> {
  // FormData bodies must keep the browser-set multipart Content-Type (with its
  // boundary); only default to JSON for the rest.
  const isForm = options.body instanceof FormData;
  const baseHeaders: Record<string, string> = isForm ? {} : { "Content-Type": "application/json" };
  const res = await fetch(BASE + path, {
    credentials: "include",
    headers: { ...baseHeaders, ...(options.headers ?? {}) },
    ...options,
  });
  // Access token expired: refresh once (using the long-lived refresh cookie) and
  // retry, so the user is not logged out mid-session.
  if (res.status === 401 && allowRetry && !path.startsWith("/auth/")) {
    if (await tryRefresh()) {
      return request<T>(path, options, false);
    }
  }
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  const body = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const code = body?.code ?? "ERROR";
    const message = body?.message ?? "İstek başarısız oldu.";
    throw new ApiError(res.status, code, message);
  }
  return body as T;
}

// download fetches a file endpoint (same auth/refresh handling as request) and
// hands it to the browser as a save dialog.
async function download(path: string, fallbackName: string, allowRetry = true): Promise<void> {
  const res = await fetch(BASE + path, { credentials: "include" });
  if (res.status === 401 && allowRetry) {
    if (await tryRefresh()) {
      return download(path, fallbackName, false);
    }
  }
  if (!res.ok) {
    const text = await res.text();
    let body: { code?: string; message?: string } | undefined;
    try { body = text ? JSON.parse(text) : undefined; } catch { body = undefined; }
    throw new ApiError(res.status, body?.code ?? "ERROR", body?.message ?? "İndirme başarısız oldu.");
  }
  const blob = await res.blob();
  const match = /filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") ?? "");
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = match?.[1] ?? fallbackName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
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

  // Build stamp of the running backend (public), to compare against the frontend.
  version: () => request<{ version: string; buildTime: string }>("/version"),

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
  myProfile: () => request<Profile>("/profile/me"),
  profileOf: (id: number) => request<Profile>(`/profile/${id}`),
  updateMyProfile: (body: { headline: string; bio: string }) => request<Profile>("/profile/me", { method: "PUT", body: JSON.stringify(body) }),
  setMyAvatar: (avatar: string) => request<User>("/users/me/avatar", { method: "PUT", body: JSON.stringify({ avatar }) }),
  setMyWhatsAppTemplates: (body: { template: string; live: string }) =>
    request<User>("/users/me/whatsapp-template", { method: "PUT", body: JSON.stringify(body) }),
  syncUserSip: (id: number, extension: string) =>
    request<void>(`/users/${id}/sip/sync`, { method: "POST", body: JSON.stringify({ extension }) }),
  syncAllSip: () =>
    request<{ synced: number; failed: number; failedExtensions?: string[]; failures?: { extension: string; reason: string }[] }>(
      "/pbx/sip/sync-all",
      { method: "POST" },
    ),

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
  updateSystemSettings: (body: SystemSettings) => request<SystemSettings>("/settings/", { method: "PUT", body: JSON.stringify(body) }),
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

  // Call log (our own store, used for the panel history — today, per agent)
  recentCalls: () => request<TodayCalls>("/calls/log/"),
  logCall: (body: { callId: string; phase: "start" | "answer" | "end"; direction?: string; peer?: string; disposition?: string; durationSeconds?: number }) =>
    request<void>("/calls/log/", { method: "POST", body: JSON.stringify(body) }),

  // Softphone (SIP over WSS to Bulutsantralim)
  sipCredentials: () => request<SipCredentials>("/sip/credentials"),
  pbxExtensions: () => request<{ items: PBXExtension[] }>("/pbx/extensions").then((r) => r.items),
  pbxQueues: () => request<{ items: PBXQueue[] }>("/pbx/queues").then((r) => r.items),
  pbxStats: () => request<PBXStats>("/pbx/stats"),
  // Team performance (today's figures per agent, scoped by permission)
  performanceToday: (params: { from?: string; to?: string } = {}) => request<TeamPerformance>("/performance/today" + query(params)),

  // Shift (mesai): the dialer opens only while a shift is open
  shiftStatus: () => request<ShiftStatus>("/shift/"),
  startShift: () => request<ShiftStatus>("/shift/start", { method: "POST" }),
  endShift: () => request<ShiftStatus>("/shift/end", { method: "POST" }),

  getAgentStatus: () => request<AgentPresence>("/pbx/status"),
  setAgentStatus: (state: AgentPresenceState) => request<void>("/pbx/status", { method: "POST", body: JSON.stringify({ state }) }),

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
