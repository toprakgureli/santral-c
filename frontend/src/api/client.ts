import type {
  CallPage,
  Contact,
  EscalationCategory,
  EscalationReason,
  EscalationRecord,
  LoginResult,
  Paged,
  PBXExtension,
  PBXQueue,
  PBXStats,
  Role,
  SipCredentials,
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

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  // FormData bodies must keep the browser-set multipart Content-Type (with its
  // boundary); only default to JSON for the rest.
  const isForm = options.body instanceof FormData;
  const baseHeaders: Record<string, string> = isForm ? {} : { "Content-Type": "application/json" };
  const res = await fetch(BASE + path, {
    credentials: "include",
    headers: { ...baseHeaders, ...(options.headers ?? {}) },
    ...options,
  });
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
    request<{ secret: string; otpauthUrl?: string; uri?: string }>("/auth/mfa/enroll", { method: "POST", body: JSON.stringify({ token }) }),
  mfaEnrollVerify: (token: string, code: string) =>
    parseLogin(request<Record<string, unknown>>("/auth/mfa/enroll/verify", { method: "POST", body: JSON.stringify({ token, code }) })),
  me: () => request<User>("/auth/me"),
  logout: () => request<void>("/auth/logout", { method: "POST" }),

  // Users
  listUsers: (params: { query?: string; page?: number; perPage?: number } = {}) =>
    request<Paged<User>>("/users/" + query(params)),
  createUser: (body: { name: string; email: string; password: string; roleIds: number[]; sipExtension?: string }) =>
    request<User>("/users/", { method: "POST", body: JSON.stringify(body) }),
  setUserActive: (id: number, active: boolean) =>
    request<void>(`/users/${id}/active`, { method: "PATCH", body: JSON.stringify({ active }) }),
  resetUserPassword: (id: number, password: string) =>
    request<void>(`/users/${id}/password`, { method: "POST", body: JSON.stringify({ password }) }),
  setUserSip: (id: number, extension: string, password: string) =>
    request<void>(`/users/${id}/sip`, { method: "POST", body: JSON.stringify({ extension, password }) }),

  // Roles
  listRoles: () => request<{ items: Role[] }>("/roles").then((r) => r.items),

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

  // Calls (Bulutsantralim CDR)
  listCalls: (params: { direction?: string; number?: string; page?: number; perPage?: number } = {}) =>
    request<CallPage>("/calls" + query(params)),
  originate: (to: string) => request<{ callUuid: string }>("/calls/originate", { method: "POST", body: JSON.stringify({ to }) }),

  // Softphone (SIP over WSS to Bulutsantralim)
  sipCredentials: () => request<SipCredentials>("/sip/credentials"),
  pbxExtensions: () => request<{ items: PBXExtension[] }>("/pbx/extensions").then((r) => r.items),
  pbxQueues: () => request<{ items: PBXQueue[] }>("/pbx/queues").then((r) => r.items),
  pbxStats: () => request<PBXStats>("/pbx/stats"),
  setAgentStatus: (dnd: boolean) => request<void>("/pbx/status", { method: "POST", body: JSON.stringify({ dnd }) }),

  // Escalations
  escalationCategories: () => request<{ items: EscalationCategory[] }>("/escalations/categories").then((r) => r.items),
  createEscalationCategory: (name: string) =>
    request<EscalationCategory>("/escalations/categories", { method: "POST", body: JSON.stringify({ name }) }),
  deleteEscalationCategory: (id: number) => request<void>(`/escalations/categories/${id}`, { method: "DELETE" }),
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
