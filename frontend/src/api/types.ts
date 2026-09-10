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
}

export interface PBXQueue {
  number: string;
  name: string;
}

export interface PBXStats {
  total: number;
  missed: number;
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
