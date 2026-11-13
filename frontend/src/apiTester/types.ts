export interface KeyValue {
  key: string;
  value: string;
  secret?: boolean;
}

export interface VariableDef {
  key: string;
  label: string;
  defaultValue: string;
  required: boolean;
  source: "manual" | "call_phone" | "call_name" | string;
  placeholder: string;
}

export interface ApiRequestItem {
  id: number;
  collectionId: number;
  name: string;
  description: string;
  method: string;
  urlTemplate: string;
  headers: KeyValue[];
  queryParams: KeyValue[];
  bodyTemplate: string;
  contentType: string;
  variables: VariableDef[];
  executionMode: "client" | "server";
  allowServerFallback: boolean;
  runbook: string;
  sortOrder: number;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ApiCollectionItem {
  id: number;
  name: string;
  description: string;
  projectTag: string;
  baseUrl: string;
  sortOrder: number;
  active: boolean;
  requests: ApiRequestItem[];
  createdAt: string;
  updatedAt: string;
}

export interface CollectionListResponse {
  enabled: boolean;
  collections: ApiCollectionItem[];
}

export interface CollectionInput {
  name: string;
  description: string;
  projectTag: string;
  baseUrl: string;
  active?: boolean;
}

export interface RequestInput {
  name: string;
  description: string;
  method: string;
  urlTemplate: string;
  headers: KeyValue[];
  queryParams: KeyValue[];
  bodyTemplate: string;
  contentType: string;
  variables: VariableDef[];
  executionMode: "client" | "server";
  allowServerFallback: boolean;
  runbook: string;
  active?: boolean;
}

export interface ExecutionResult {
  statusCode: number;
  statusText: string;
  headers: KeyValue[];
  body: string;
  truncated?: boolean;
  durationMs: number;
  targetUrl: string;
  executedVia: "extension" | "browser" | "server";
  error?: string;
  corsBlocked?: boolean;
  extensionHint?: string;
}

export interface RunLogItem {
  id: number;
  requestId?: number;
  requestName: string;
  collectionName: string;
  userId?: number;
  userName: string;
  method: string;
  targetUrl: string;
  executionMode: string;
  statusCode: number;
  durationMs: number;
  errorMessage?: string;
  createdAt: string;
}

export type ExtensionBridgeState = "checking" | "ready" | "wrong_origin" | "missing";
