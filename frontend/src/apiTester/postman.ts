import { extractPlaceholders } from "./runner";
import type { CollectionInput, KeyValue, RequestInput, VariableDef } from "./types";

export interface TemplatizeOptions {
  asTemplate: boolean;
  templatizeBaseUrl: boolean;
  keepDefaults: boolean;
  collectionVariables?: Record<string, string>;
}

export interface ParsedPostmanCollection {
  collection: CollectionInput;
  requests: RequestInput[];
  warnings: string[];
}

function toVarKey(raw: string): string {
  const cleaned = raw
    .trim()
    .replace(/[^a-zA-Z0-9_.-]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return cleaned || "param";
}

function detectVarSource(key: string): VariableDef["source"] {
  const lower = key.toLowerCase();
  if (/(phone|tel|gsm|msisdn|caller|numara)/.test(lower)) return "call_phone";
  if (/(caller_name|customer_name|musteri_adi|ad_soyad)/.test(lower)) return "call_name";
  return "manual";
}

function isSecretHeader(key: string): boolean {
  const lower = key.toLowerCase();
  return (
    lower === "authorization" ||
    lower.includes("token") ||
    lower.includes("key") ||
    lower.includes("secret")
  );
}

function extractOriginPrefix(urlTemplate: string): { origin: string; rest: string } | null {
  const m = /^(https?:\/\/[^/?#]+)(\/.*)?$/i.exec(urlTemplate.trim());
  if (!m) return null;
  return { origin: m[1].replace(/\/+$/, ""), rest: m[2] || "/" };
}

function replacePostmanPathParams(urlTemplate: string): { url: string; pathKeys: string[] } {
  const pathKeys: string[] = [];
  // Replace /:param or {{base_url}}/:param without touching scheme ports like :8080.
  const url = urlTemplate.replace(/\/:([a-zA-Z_][a-zA-Z0-9_-]*)(?=\/|\?|#|$)/g, (_, key: string) => {
    const varKey = toVarKey(key);
    pathKeys.push(varKey);
    return `/{{${varKey}}}`;
  });
  return { url, pathKeys };
}

function templatizeJsonBody(
  rawBody: string,
  keepDefaults: boolean,
  captured: Map<string, { value: string; label: string }>,
): string {
  const trimmed = rawBody.trim();
  if (!trimmed || (!trimmed.startsWith("{") && !trimmed.startsWith("["))) {
    return rawBody;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return rawBody;
  }

  const numericMarkers = new Map<string, string>();
  let markerSeq = 0;

  const allocKey = (rawKey: string, valStr: string): string => {
    const base = toVarKey(rawKey);
    let candidate = base;
    let idx = 2;
    while (captured.has(candidate) && captured.get(candidate)?.value !== valStr) {
      candidate = `${base}_${idx++}`;
    }
    captured.set(candidate, { value: valStr, label: rawKey });
    return candidate;
  };

  const transform = (node: unknown): unknown => {
    if (Array.isArray(node)) {
      return node.map((item) => transform(item));
    }
    if (node && typeof node === "object") {
      const out: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
        if (typeof v === "string") {
          if (/\{\{\s*[a-zA-Z0-9_.-]+\s*\}\}/.test(v)) {
            out[k] = v;
          } else {
            const varKey = allocKey(k, v);
            out[k] = `{{${varKey}}}`;
          }
        } else if (typeof v === "number" && Number.isFinite(v)) {
          const valStr = String(v);
          const varKey = allocKey(k, valStr);
          if (keepDefaults) {
            const marker = `__NUM_TPL_${markerSeq++}__`;
            numericMarkers.set(JSON.stringify(marker), `{{${varKey}}}`);
            out[k] = marker;
          } else {
            // Quote when no numeric default is stored so partial empty inputs never break JSON syntax.
            out[k] = `{{${varKey}}}`;
          }
        } else if (v && typeof v === "object") {
          out[k] = transform(v);
        } else {
          out[k] = v;
        }
      }
      return out;
    }
    return node;
  };

  let serialized = JSON.stringify(transform(parsed), null, 2);
  for (const [quotedMarker, placeholder] of numericMarkers.entries()) {
    serialized = serialized.replace(quotedMarker, placeholder);
  }
  return serialized;
}

export function templatizeRequestInput(
  input: RequestInput,
  opts: TemplatizeOptions,
  knownBaseUrl?: string,
): { request: RequestInput; extractedBaseUrl?: string } {
  const captured = new Map<string, { value: string; label: string }>();
  let urlTemplate = input.urlTemplate.trim();
  let queryParams = (input.queryParams ?? []).map((q) => ({ ...q }));
  let bodyTemplate = input.bodyTemplate ?? "";
  let extractedBaseUrl: string | undefined;

  if (opts.asTemplate) {
    const pathRes = replacePostmanPathParams(urlTemplate);
    urlTemplate = pathRes.url;
    for (const pk of pathRes.pathKeys) {
      if (!captured.has(pk)) {
        captured.set(pk, { value: opts.collectionVariables?.[pk] ?? "", label: pk });
      }
    }

    if (opts.templatizeBaseUrl) {
      const originInfo = extractOriginPrefix(urlTemplate);
      if (originInfo) {
        if (!knownBaseUrl || originInfo.origin === knownBaseUrl.replace(/\/+$/, "")) {
          extractedBaseUrl = originInfo.origin;
          urlTemplate = `{{base_url}}${originInfo.rest}`;
        }
      }
    }

    queryParams = queryParams.map((q) => {
      const val = q.value ?? "";
      if (/\{\{\s*[a-zA-Z0-9_.-]+\s*\}\}/.test(val)) {
        return q;
      }
      let varKey = toVarKey(q.key);
      let idx = 2;
      while (captured.has(varKey) && captured.get(varKey)?.value !== val) {
        varKey = `${toVarKey(q.key)}_${idx++}`;
      }
      captured.set(varKey, { value: val, label: q.key });
      return { ...q, value: `{{${varKey}}}` };
    });

    if (
      bodyTemplate.trim() &&
      input.method !== "GET" &&
      input.method !== "HEAD" &&
      (input.contentType?.toLowerCase().includes("json") || bodyTemplate.trim().startsWith("{"))
    ) {
      bodyTemplate = templatizeJsonBody(bodyTemplate, opts.keepDefaults, captured);
    }
  }

  const placeholders = extractPlaceholders(
    urlTemplate,
    bodyTemplate,
    ...(input.headers ?? []).map((h) => h.value),
    ...queryParams.map((q) => q.value),
  ).filter((k) => k !== "base_url");

  const existingVars = new Map((input.variables ?? []).map((v) => [v.key, v]));
  const mergedVars: VariableDef[] = [...(input.variables ?? [])];

  for (const key of placeholders) {
    if (existingVars.has(key)) continue;
    const cap = captured.get(key);
    const colDefault = opts.collectionVariables?.[key] ?? "";
    const rawSample = cap?.value ?? colDefault;
    mergedVars.push({
      key,
      label: cap?.label || key,
      defaultValue: opts.keepDefaults ? rawSample : colDefault,
      required: true,
      source: detectVarSource(key),
      placeholder: rawSample,
    });
  }

  return {
    request: {
      ...input,
      urlTemplate,
      queryParams,
      bodyTemplate,
      variables: mergedVars,
    },
    extractedBaseUrl,
  };
}

interface PostmanKV {
  key?: string;
  value?: string;
  disabled?: boolean;
  type?: string;
  description?: string | { content?: string };
}

interface PostmanAuth {
  type?: string;
  bearer?: PostmanKV[];
  basic?: PostmanKV[];
  apikey?: PostmanKV[];
}

interface PostmanUrl {
  raw?: string;
  protocol?: string;
  host?: string | string[];
  port?: string;
  path?: string | string[];
  query?: PostmanKV[];
  variable?: PostmanKV[];
}

interface PostmanItem {
  name?: string;
  description?: string | { content?: string };
  item?: PostmanItem[];
  request?: {
    method?: string;
    header?: PostmanKV[];
    url?: string | PostmanUrl;
    body?: {
      mode?: string;
      raw?: string;
      urlencoded?: PostmanKV[];
      formdata?: PostmanKV[];
      options?: { raw?: { language?: string } };
    };
    auth?: PostmanAuth;
    description?: string | { content?: string };
  };
}

interface PostmanDocument {
  info?: {
    name?: string;
    description?: string | { content?: string };
    schema?: string;
  };
  item?: PostmanItem[];
  variable?: PostmanKV[];
  auth?: PostmanAuth;
}

function readDesc(d: string | { content?: string } | undefined): string {
  if (!d) return "";
  if (typeof d === "string") return d.trim();
  return typeof d.content === "string" ? d.content.trim() : "";
}

function kvVal(list: PostmanKV[] | undefined, name: string): string {
  if (!Array.isArray(list)) return "";
  const hit = list.find((x) => x && x.key === name);
  return typeof hit?.value === "string" ? hit.value : "";
}

function applyPostmanAuth(
  auth: PostmanAuth | undefined,
  headers: KeyValue[],
  queryParams: KeyValue[],
): void {
  if (!auth || !auth.type || auth.type === "noauth" || auth.type === "inherit") return;
  const hasAuthHeader = headers.some((h) => h.key.toLowerCase() === "authorization");

  if (auth.type === "bearer") {
    const token = kvVal(auth.bearer, "token");
    if (token && !hasAuthHeader) {
      headers.push({ key: "Authorization", value: `Bearer ${token}`, secret: true });
    }
  } else if (auth.type === "basic") {
    const user = kvVal(auth.basic, "username");
    const pass = kvVal(auth.basic, "password");
    if ((user || pass) && !hasAuthHeader) {
      headers.push({ key: "Authorization", value: `Basic ${btoa(`${user}:${pass}`)}`, secret: true });
    }
  } else if (auth.type === "apikey") {
    const keyName = kvVal(auth.apikey, "key") || "X-API-Key";
    const keyVal = kvVal(auth.apikey, "value");
    const loc = kvVal(auth.apikey, "in") || "header";
    if (loc === "query") {
      if (!queryParams.some((q) => q.key === keyName)) {
        queryParams.push({ key: keyName, value: keyVal });
      }
    } else if (!headers.some((h) => h.key.toLowerCase() === keyName.toLowerCase())) {
      headers.push({ key: keyName, value: keyVal, secret: true });
    }
  }
}

function extractPostmanUrl(
  rawUrl: string | PostmanUrl | undefined,
  pathVarDefaults: Map<string, string>,
): { urlTemplate: string; queryParams: KeyValue[] } {
  if (!rawUrl) return { urlTemplate: "", queryParams: [] };

  if (typeof rawUrl === "string") {
    const trimmed = rawUrl.trim();
    const qIdx = trimmed.indexOf("?");
    if (qIdx < 0) return { urlTemplate: trimmed, queryParams: [] };
    const base = trimmed.slice(0, qIdx);
    const qs = trimmed.slice(qIdx + 1);
    const params: KeyValue[] = [];
    for (const part of qs.split("&")) {
      if (!part) continue;
      const eq = part.indexOf("=");
      if (eq < 0) {
        params.push({ key: decodeSafe(part), value: "" });
      } else {
        params.push({ key: decodeSafe(part.slice(0, eq)), value: decodeSafe(part.slice(eq + 1)) });
      }
    }
    return { urlTemplate: base, queryParams: params };
  }

  for (const pv of rawUrl.variable ?? []) {
    if (pv.key && typeof pv.value === "string" && pv.value) {
      pathVarDefaults.set(toVarKey(pv.key), pv.value);
    }
  }

  const queryParams: KeyValue[] = [];
  if (Array.isArray(rawUrl.query)) {
    for (const q of rawUrl.query) {
      if (!q || q.disabled || !q.key) continue;
      queryParams.push({ key: q.key, value: q.value ?? "" });
    }
  }

  // Prefer structured host + path so disabled query params stay omitted,
  // falling back to rawUrl.raw stripped of query string.
  let base = "";
  if (rawUrl.raw) {
    const qIdx = rawUrl.raw.indexOf("?");
    base = (qIdx >= 0 ? rawUrl.raw.slice(0, qIdx) : rawUrl.raw).trim();
  } else {
    const host = Array.isArray(rawUrl.host) ? rawUrl.host.join(".") : (rawUrl.host ?? "");
    const path = Array.isArray(rawUrl.path) ? rawUrl.path.join("/") : (rawUrl.path ?? "");
    const proto = rawUrl.protocol ? `${rawUrl.protocol}://` : host.startsWith("{{") ? "" : "https://";
    const port = rawUrl.port ? `:${rawUrl.port}` : "";
    base = `${proto}${host}${port}${path ? `/${path.replace(/^\/+/, "")}` : ""}`;
  }

  return { urlTemplate: base, queryParams };
}

function decodeSafe(s: string): string {
  try {
    return decodeURIComponent(s.replace(/\+/g, " "));
  } catch {
    return s;
  }
}

export function parsePostmanCollection(
  rawJson: string,
  opts: TemplatizeOptions,
): ParsedPostmanCollection {
  let doc: PostmanDocument;
  try {
    doc = JSON.parse(rawJson) as PostmanDocument;
  } catch {
    throw new Error("Geçersiz JSON dosyası: Postman koleksiyonu okunamadı.");
  }

  if (!doc || typeof doc !== "object" || !Array.isArray(doc.item)) {
    throw new Error("Bu dosya geçerli bir Postman v2.0 / v2.1 koleksiyonu değil ('item' listesi bulunamadı).");
  }

  const warnings: string[] = [];
  const colVars: Record<string, string> = {};
  let detectedBaseUrl = "";

  for (const v of doc.variable ?? []) {
    if (!v || v.disabled || !v.key) continue;
    const val = typeof v.value === "string" ? v.value : "";
    colVars[v.key] = val;
    if (!detectedBaseUrl && /^(base_?url|url|host|api_?url)$/i.test(v.key) && /^https?:\/\//i.test(val)) {
      detectedBaseUrl = val.replace(/\/+$/, "");
    }
  }

  const rawItems: { req: RequestInput; pathVarDefaults: Map<string, string> }[] = [];

  const walk = (items: PostmanItem[], prefix: string[]) => {
    for (const item of items) {
      if (!item || typeof item !== "object") continue;
      if (Array.isArray(item.item)) {
        const nextPrefix = item.name ? [...prefix, item.name.trim()] : prefix;
        walk(item.item, nextPrefix);
        continue;
      }
      if (!item.request) continue;

      const r = item.request;
      const fullName = [...prefix, (item.name || "İsimsiz İstek").trim()]
        .filter(Boolean)
        .join(" / ")
        .slice(0, 120);
      const method = (r.method || "GET").toUpperCase();
      const pathVarDefaults = new Map<string, string>();
      const { urlTemplate, queryParams } = extractPostmanUrl(r.url, pathVarDefaults);

      if (!urlTemplate) {
        warnings.push(`"${fullName}" isteğinin URL adresi boş olduğu için atlandı.`);
        continue;
      }

      const headers: KeyValue[] = [];
      for (const h of r.header ?? []) {
        if (!h || h.disabled || !h.key) continue;
        headers.push({
          key: h.key.trim(),
          value: h.value ?? "",
          secret: isSecretHeader(h.key),
        });
      }

      const authToApply = r.auth && r.auth.type && r.auth.type !== "inherit" ? r.auth : doc.auth;
      applyPostmanAuth(authToApply, headers, queryParams);

      let bodyTemplate = "";
      let contentType =
        headers.find((h) => h.key.toLowerCase() === "content-type")?.value || "application/json";

      if (r.body) {
        if (r.body.mode === "raw") {
          bodyTemplate = r.body.raw ?? "";
          const lang = r.body.options?.raw?.language;
          if (!headers.some((h) => h.key.toLowerCase() === "content-type")) {
            if (lang === "xml") contentType = "application/xml";
            else if (lang === "text") contentType = "text/plain";
            else contentType = "application/json";
          }
        } else if (r.body.mode === "urlencoded" && Array.isArray(r.body.urlencoded)) {
          contentType = "application/x-www-form-urlencoded";
          const pairs = r.body.urlencoded
            .filter((p) => p && !p.disabled && p.key)
            .map((p) => `${encodeURIComponent(p.key! )}=${encodeURIComponent(p.value ?? "")}`);
          bodyTemplate = pairs.join("&");
        } else if (r.body.mode === "formdata" && Array.isArray(r.body.formdata)) {
          const obj: Record<string, string> = {};
          for (const f of r.body.formdata) {
            if (!f || f.disabled || !f.key || f.type === "file") continue;
            obj[f.key] = f.value ?? "";
          }
          if (Object.keys(obj).length > 0) {
            contentType = "application/json";
            bodyTemplate = JSON.stringify(obj, null, 2);
            warnings.push(`"${fullName}" isteğindeki form-data metin alanları JSON gövdesine çevrildi.`);
          }
        }
      }

      const desc = readDesc(r.description) || readDesc(item.description);

      rawItems.push({
        pathVarDefaults,
        req: {
          name: fullName,
          description: desc.slice(0, 500),
          method,
          urlTemplate,
          headers,
          queryParams,
          bodyTemplate,
          contentType,
          variables: [],
          executionMode: "client",
          allowServerFallback: true,
          runbook: desc.length > 500 ? desc : "",
          active: true,
        },
      });
    }
  };

  walk(doc.item, []);

  if (rawItems.length === 0) {
    throw new Error("Koleksiyon içinde içe aktarılabilir geçerli HTTP isteği bulunamadı.");
  }

  // If Postman collection didn't declare a base_url variable and base URL
  // templatization is enabled, pick the most frequent origin prefix cleanly.
  if (opts.asTemplate && opts.templatizeBaseUrl && !detectedBaseUrl) {
    const counts = new Map<string, number>();
    for (const item of rawItems) {
      const originInfo = extractOriginPrefix(item.req.urlTemplate);
      if (originInfo) {
        counts.set(originInfo.origin, (counts.get(originInfo.origin) ?? 0) + 1);
      }
    }
    let bestOrigin = "";
    let bestCount = 0;
    for (const [orig, count] of counts.entries()) {
      if (count > bestCount) {
        bestOrigin = orig;
        bestCount = count;
      }
    }
    detectedBaseUrl = bestOrigin;
  }

  const requests: RequestInput[] = rawItems.map(({ req, pathVarDefaults }) => {
    const mergedColVars: Record<string, string> = { ...colVars };
    for (const [k, v] of pathVarDefaults.entries()) {
      if (!(k in mergedColVars)) mergedColVars[k] = v;
    }
    // Normalize Postman's {{baseUrl}} / {{url}} aliases if baseUrl was extracted.
    const normalizedUrl =
      detectedBaseUrl && /^\{\{\s*(baseUrl|url|host|api_url)\s*\}\}/i.test(req.urlTemplate)
        ? req.urlTemplate.replace(/^\{\{\s*(baseUrl|url|host|api_url)\s*\}\}/i, "{{base_url}}")
        : req.urlTemplate;

    const { request } = templatizeRequestInput(
      { ...req, urlTemplate: normalizedUrl },
      { ...opts, collectionVariables: mergedColVars },
      detectedBaseUrl || undefined,
    );
    return request;
  });

  return {
    collection: {
      name: (doc.info?.name || "Postman Koleksiyonu").trim().slice(0, 120),
      description: readDesc(doc.info?.description),
      projectTag: "Postman",
      baseUrl: detectedBaseUrl,
      active: true,
    },
    requests,
    warnings,
  };
}
