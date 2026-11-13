import type {
  ApiCollectionItem,
  ApiRequestItem,
  ExecutionResult,
  ExtensionBridgeState,
  KeyValue,
  RequestInput,
} from "./types";

const VAR_RE = /\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g;

export function extractPlaceholders(...templates: string[]): string[] {
  const found = new Set<string>();
  for (const tmpl of templates) {
    if (!tmpl) continue;
    for (const m of tmpl.matchAll(VAR_RE)) {
      if (m[1]) found.add(m[1]);
    }
  }
  return [...found];
}

export function interpolate(tmpl: string, vars: Record<string, string>): string {
  if (!tmpl) return "";
  return tmpl.replace(VAR_RE, (match, key: string) => (key in vars ? vars[key] : match));
}

export function renderClientRequest(
  col: ApiCollectionItem,
  req: ApiRequestItem,
  supplied: Record<string, string>,
): { url: string; method: string; headers: KeyValue[]; body: string } {
  const vars: Record<string, string> = {};
  if (col.baseUrl.trim()) {
    vars.base_url = col.baseUrl.trim().replace(/\/+$/, "");
  }
  for (const v of req.variables ?? []) {
    if (v.key && v.defaultValue) vars[v.key] = v.defaultValue;
  }
  for (const [k, v] of Object.entries(supplied)) {
    if (k.trim()) vars[k.trim()] = v;
  }
  for (const v of req.variables ?? []) {
    if (v.required && !(vars[v.key] ?? "").trim()) {
      throw new Error(`${v.label || v.key} alanı zorunludur.`);
    }
  }

  const rawUrl = interpolate(req.urlTemplate, vars).trim();
  let parsed: URL;
  try {
    parsed = new URL(rawUrl);
  } catch {
    throw new Error("Oluşturulan hedef URL geçersiz. Değişkenleri ve taban adresi kontrol edin.");
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("Sadece http:// ve https:// adreslerine istek gönderilebilir.");
  }

  for (const q of req.queryParams ?? []) {
    const k = q.key.trim();
    if (!k) continue;
    parsed.searchParams.append(k, interpolate(q.value, vars));
  }

  const headers: KeyValue[] = [];
  let hasContentType = false;
  for (const h of req.headers ?? []) {
    const k = h.key.trim();
    if (!k) continue;
    if (k.toLowerCase() === "content-type") hasContentType = true;
    headers.push({ key: k, value: interpolate(h.value, vars), secret: h.secret });
  }

  const method = (req.method || "GET").toUpperCase();
  const body = method !== "GET" && method !== "HEAD" ? interpolate(req.bodyTemplate || "", vars) : "";
  if (body && !hasContentType && req.contentType?.trim()) {
    headers.push({ key: "Content-Type", value: req.contentType.trim() });
  }

  return { url: parsed.toString(), method, headers, body };
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

export function buildCurlCommand(rendered: {
  url: string;
  method: string;
  headers: KeyValue[];
  body: string;
}): string {
  const parts = ["curl", "-i", "-X", rendered.method, shellQuote(rendered.url)];
  for (const h of rendered.headers) {
    if (!h.key.trim()) continue;
    parts.push("-H", shellQuote(`${h.key.trim()}: ${h.value}`));
  }
  if (rendered.body) {
    parts.push("--data-raw", shellQuote(rendered.body));
  }
  return parts.join(" ");
}

function tokenizeShell(input: string): string[] {
  const normalized = input.replace(/\\\r?\n/g, " ");
  const tokens: string[] = [];
  let cur = "";
  let quote: "'" | '"' | null = null;
  let escaping = false;

  for (let i = 0; i < normalized.length; i++) {
    const ch = normalized[i];
    if (escaping) {
      cur += ch;
      escaping = false;
      continue;
    }
    if (ch === "\\" && quote !== "'") {
      escaping = true;
      continue;
    }
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
      continue;
    }
    if (ch === "'" || ch === '"') {
      quote = ch;
      continue;
    }
    if (/\s/.test(ch)) {
      if (cur.length > 0) {
        tokens.push(cur);
        cur = "";
      }
      continue;
    }
    cur += ch;
  }
  if (cur.length > 0) tokens.push(cur);
  return tokens;
}

export function parseCurlCommand(raw: string): Partial<RequestInput> {
  const tokens = tokenizeShell(raw.trim());
  if (tokens.length === 0) {
    throw new Error("Boş cURL komutu.");
  }
  let idx = 0;
  if (tokens[0].toLowerCase() === "curl") idx = 1;

  let method = "";
  let urlStr = "";
  const headers: KeyValue[] = [];
  const dataParts: string[] = [];
  let contentType = "application/json";

  while (idx < tokens.length) {
    const t = tokens[idx];
    if (t === "-X" || t === "--request") {
      method = (tokens[++idx] ?? "").toUpperCase();
    } else if (t.startsWith("-X") && t.length > 2) {
      method = t.slice(2).toUpperCase();
    } else if (t === "-H" || t === "--header") {
      const h = tokens[++idx] ?? "";
      const colon = h.indexOf(":");
      if (colon > 0) {
        const key = h.slice(0, colon).trim();
        const value = h.slice(colon + 1).trim();
        const lower = key.toLowerCase();
        const isSecret =
          lower === "authorization" ||
          lower.includes("token") ||
          lower.includes("key") ||
          lower.includes("secret");
        if (lower === "content-type") contentType = value;
        headers.push({ key, value, secret: isSecret });
      }
    } else if (
      t === "-d" ||
      t === "--data" ||
      t === "--data-raw" ||
      t === "--data-binary" ||
      t === "--data-urlencode"
    ) {
      dataParts.push(tokens[++idx] ?? "");
    } else if (t === "--url") {
      urlStr = tokens[++idx] ?? "";
    } else if (!t.startsWith("-") && /^https?:\/\//i.test(t)) {
      urlStr = t;
    } else if ((t === "-A" || t === "--user-agent" || t === "-u" || t === "--user") && idx + 1 < tokens.length) {
      if (t === "-u" || t === "--user") {
        const creds = tokens[++idx] ?? "";
        headers.push({ key: "Authorization", value: `Basic ${btoa(creds)}`, secret: true });
      } else {
        idx++;
      }
    }
    idx++;
  }

  if (!urlStr) {
    throw new Error("cURL komutunda geçerli bir http(s) URL bulunamadı.");
  }

  const parsed = new URL(urlStr);
  const queryParams: KeyValue[] = [];
  parsed.searchParams.forEach((value, key) => {
    queryParams.push({ key, value });
  });
  parsed.search = "";

  const bodyTemplate = dataParts.join("&");
  if (!method) {
    method = bodyTemplate ? "POST" : "GET";
  }

  return {
    name: `${method} ${parsed.pathname || "/"}`,
    method,
    urlTemplate: parsed.toString(),
    headers,
    queryParams,
    bodyTemplate,
    contentType,
  };
}

let bridgeSeq = 1;

export async function detectExtensionBridge(timeoutMs = 220): Promise<ExtensionBridgeState> {
  if (typeof window === "undefined" || typeof document === "undefined") return "missing";
  const hasAttr = Boolean(document.documentElement?.getAttribute("data-santralc-ext"));
  const pingId = `ping-${Date.now()}-${bridgeSeq++}`;

  const acked = await new Promise<boolean>((resolve) => {
    let done = false;
    const timer = window.setTimeout(() => {
      if (done) return;
      done = true;
      window.removeEventListener("message", onMsg);
      resolve(false);
    }, timeoutMs);

    function onMsg(e: MessageEvent) {
      if (e.source !== window || e.origin !== window.location.origin) return;
      const d = e.data;
      if (!d || d.santralc !== "ext" || d.type !== "apiPong" || d.id !== pingId) return;
      done = true;
      window.clearTimeout(timer);
      window.removeEventListener("message", onMsg);
      resolve(true);
    }

    window.addEventListener("message", onMsg);
    window.postMessage({ santralc: "panel", type: "apiPing", id: pingId }, window.location.origin);
  });

  if (acked) return "ready";
  return hasAttr ? "wrong_origin" : "missing";
}

async function runViaExtension(rendered: {
  url: string;
  method: string;
  headers: KeyValue[];
  body: string;
}): Promise<ExecutionResult | null> {
  const reqId = `api-${Date.now()}-${bridgeSeq++}`;
  return new Promise<ExecutionResult | null>((resolve) => {
    let settled = false;
    const timer = window.setTimeout(() => {
      if (settled) return;
      settled = true;
      window.removeEventListener("message", onMsg);
      resolve(null);
    }, 26000);

    function onMsg(e: MessageEvent) {
      if (e.source !== window || e.origin !== window.location.origin) return;
      const d = e.data;
      if (!d || d.santralc !== "ext" || d.type !== "apiResponse" || d.id !== reqId) return;
      settled = true;
      window.clearTimeout(timer);
      window.removeEventListener("message", onMsg);
      if (d.code === "unauthorized_origin") {
        resolve(null);
        return;
      }
      resolve({
        statusCode: typeof d.statusCode === "number" ? d.statusCode : 0,
        statusText: typeof d.statusText === "string" ? d.statusText : "",
        headers: Array.isArray(d.headers) ? d.headers : [],
        body: typeof d.body === "string" ? d.body : "",
        truncated: Boolean(d.truncated),
        durationMs: typeof d.durationMs === "number" ? d.durationMs : 0,
        targetUrl: typeof d.targetUrl === "string" ? d.targetUrl : rendered.url,
        executedVia: "extension",
        error: d.ok ? undefined : String(d.error || "Eklenti üzerinden istek gönderilemedi."),
      });
    }

    window.addEventListener("message", onMsg);
    window.postMessage(
      {
        santralc: "panel",
        type: "apiRequest",
        id: reqId,
        req: {
          method: rendered.method,
          url: rendered.url,
          headers: rendered.headers.map((h) => ({ key: h.key, value: h.value })),
          body: rendered.body,
        },
      },
      window.location.origin,
    );
  });
}

export async function runFromClientIP(
  rendered: { url: string; method: string; headers: KeyValue[]; body: string },
  bridgeState: ExtensionBridgeState,
): Promise<ExecutionResult> {
  if (bridgeState === "ready") {
    const extRes = await runViaExtension(rendered);
    if (extRes) return extRes;
  }

  const start = performance.now();
  const headerInit: Record<string, string> = {};
  for (const h of rendered.headers) {
    if (h.key.trim()) headerInit[h.key.trim()] = h.value;
  }

  try {
    const res = await fetch(rendered.url, {
      method: rendered.method,
      headers: headerInit,
      body: rendered.method !== "GET" && rendered.method !== "HEAD" && rendered.body ? rendered.body : undefined,
      mode: "cors",
    });
    const durationMs = Math.round(performance.now() - start);
    let body = await res.text();
    let truncated = false;
    if (body.length > 1024 * 1024) {
      body = body.slice(0, 1024 * 1024);
      truncated = true;
    }
    const headers: KeyValue[] = [];
    res.headers.forEach((value, key) => headers.push({ key, value }));
    return {
      statusCode: res.status,
      statusText: res.statusText,
      headers,
      body,
      truncated,
      durationMs,
      targetUrl: res.url || rendered.url,
      executedVia: "browser",
    };
  } catch (err) {
    const durationMs = Math.round(performance.now() - start);
    const msg = err instanceof Error ? err.message : String(err);
    const corsLikely = err instanceof TypeError || /failed to fetch|networkerror|cors/i.test(msg);
    return {
      statusCode: 0,
      statusText: "",
      headers: [],
      body: "",
      durationMs,
      targetUrl: rendered.url,
      executedVia: "browser",
      corsBlocked: corsLikely,
      extensionHint: bridgeState === "wrong_origin" ? "wrong_origin" : bridgeState === "missing" ? "missing" : undefined,
      error: corsLikely
        ? "Tarayıcı güvenlik politikası (CORS) bu isteği engelledi."
        : `İstek başarısız oldu: ${msg}`,
    };
  }
}
