export interface User {
  id: number;
  name: string;
  email: string;
  active: boolean;
  roles: string[];
  permissions: string[];
  mfaEnabled: boolean;
  mustChangePassword: boolean;
  sipExtension?: string;
  createdAt: string;
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
  id: number;
  direction: string;
  disposition: string;
  fromNumber: string;
  toNumber: string;
  fromUserId?: number;
  toUserId?: number;
  contactId?: number;
  startedAt: string;
  answeredAt?: string;
  endedAt?: string;
  ringSeconds: number;
  talkSeconds: number;
  hangupCauseCode?: number;
  hangupCauseText?: string;
  hangupBy?: string;
}

export interface CallEvent {
  id: number;
  seq: number;
  type: string;
  channel?: string;
  at: string;
  detail: string;
}

export interface CallQuality {
  id: number;
  leg: string;
  codec?: string;
  at: string;
  jitterMs?: number;
  rttMs?: number;
  lossPct?: number;
  mos?: number;
}

export interface CallDetail extends Call {
  events: CallEvent[];
  quality: CallQuality[];
}

export interface SipCredentials {
  extension: string;
  secret: string;
  webSocketUrl: string;
}
