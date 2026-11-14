import { useEffect, useState } from "react";
import { FileCode2, Lock, Plus, Sparkles, Trash2 } from "lucide-react";
import { ApiError } from "@/api/client";
import { apiTesterApi } from "@/apiTester/api";
import { extractPlaceholders } from "@/apiTester/runner";
import type { ApiRequestItem, KeyValue, RequestInput, VariableDef } from "@/apiTester/types";
import { Button, Field, Input, Modal, Select } from "@/components/ui";
import { Switch } from "@/components/whatsapp/settings/parts";
import { CurlImportModal } from "./CurlImportModal";

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

interface RequestEditorModalProps {
  open: boolean;
  collectionId: number | null;
  editing: ApiRequestItem | null;
  onClose: () => void;
  onSaved: (req: ApiRequestItem) => void;
}

export function RequestEditorModal({
  open,
  collectionId,
  editing,
  onClose,
  onSaved,
}: RequestEditorModalProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [method, setMethod] = useState("GET");
  const [urlTemplate, setUrlTemplate] = useState("");
  const [headers, setHeaders] = useState<KeyValue[]>([]);
  const [queryParams, setQueryParams] = useState<KeyValue[]>([]);
  const [contentType, setContentType] = useState("application/json");
  const [bodyTemplate, setBodyTemplate] = useState("");
  const [variables, setVariables] = useState<VariableDef[]>([]);
  const [executionMode, setExecutionMode] = useState<"client" | "server">("client");
  const [allowServerFallback, setAllowServerFallback] = useState(true);
  const [runbook, setRunbook] = useState("");
  const [active, setActive] = useState(true);
  const [curlOpen, setCurlOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    if (editing) {
      setName(editing.name);
      setDescription(editing.description);
      setMethod(editing.method);
      setUrlTemplate(editing.urlTemplate);
      setHeaders(editing.headers?.length ? editing.headers : []);
      setQueryParams(editing.queryParams?.length ? editing.queryParams : []);
      setContentType(editing.contentType || "application/json");
      setBodyTemplate(editing.bodyTemplate || "");
      setVariables(editing.variables?.length ? editing.variables : []);
      setExecutionMode(editing.executionMode || "client");
      setAllowServerFallback(editing.allowServerFallback ?? true);
      setRunbook(editing.runbook || "");
      setActive(editing.active);
    } else {
      setName("");
      setDescription("");
      setMethod("GET");
      setUrlTemplate("{{base_url}}/");
      setHeaders([]);
      setQueryParams([]);
      setContentType("application/json");
      setBodyTemplate("");
      setVariables([]);
      setExecutionMode("client");
      setAllowServerFallback(true);
      setRunbook("");
      setActive(true);
    }
  }, [open, editing]);

  const detectVariables = () => {
    const keys = extractPlaceholders(
      urlTemplate,
      bodyTemplate,
      ...headers.map((h) => h.value),
      ...queryParams.map((q) => q.value),
    ).filter((k) => k !== "base_url");

    const existing = new Set(variables.map((v) => v.key));
    const added: VariableDef[] = [];
    for (const k of keys) {
      if (existing.has(k)) continue;
      const lower = k.toLowerCase();
      const source =
        lower.includes("phone") || lower.includes("tel") || lower.includes("gsm")
          ? "call_phone"
          : lower.includes("name") || lower.includes("musteri")
            ? "call_name"
            : "manual";
      added.push({
        key: k,
        label: k,
        defaultValue: "",
        required: true,
        source,
        placeholder: "",
      });
    }
    if (added.length > 0) {
      setVariables([...variables, ...added]);
    }
  };

  const applyCurl = (parsed: Partial<RequestInput>) => {
    if (!name.trim() && parsed.name) setName(parsed.name);
    if (parsed.method) setMethod(parsed.method);
    if (parsed.urlTemplate) setUrlTemplate(parsed.urlTemplate);
    if (parsed.headers) setHeaders(parsed.headers);
    if (parsed.queryParams) setQueryParams(parsed.queryParams);
    if (parsed.contentType) setContentType(parsed.contentType);
    if (parsed.bodyTemplate !== undefined) setBodyTemplate(parsed.bodyTemplate);
    if (parsed.variables) setVariables(parsed.variables);
  };

  const submit = async () => {
    const targetColId = editing?.collectionId ?? collectionId;
    if (!targetColId) return;
    setBusy(true);
    setError(null);
    const payload: RequestInput = {
      name: name.trim(),
      description: description.trim(),
      method,
      urlTemplate: urlTemplate.trim(),
      headers: headers.filter((h) => h.key.trim()),
      queryParams: queryParams.filter((q) => q.key.trim()),
      bodyTemplate,
      contentType: contentType.trim() || "application/json",
      variables: variables.filter((v) => v.key.trim()),
      executionMode,
      allowServerFallback,
      runbook: runbook.trim(),
      active,
    };
    try {
      const saved = editing
        ? await apiTesterApi.updateRequest(editing.id, payload)
        : await apiTesterApi.createRequest(targetColId, payload);
      onSaved(saved);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İstek şablonu kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Modal
        open={open}
        onClose={onClose}
        title={editing ? "İstek Şablonunu Düzenle" : "Yeni API İsteği Oluştur"}
        description="Değişkenler için {{degisken_adi}} sözdizimini kullanabilirsiniz. Koleksiyon taban URL'si için {{base_url}} tanımlıdır."
        size="lg"
        footer={
          <>
            <Button variant="secondary" onClick={onClose} disabled={busy}>Vazgeç</Button>
            <Button onClick={submit} disabled={busy || !name.trim() || !urlTemplate.trim()}>
              {busy ? "Kaydediliyor..." : editing ? "Güncelle" : "Oluştur"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">Hızlı şablon oluşturmak için cURL yapıştırabilirsiniz.</span>
            <Button type="button" variant="secondary" className="h-8 px-3 text-xs" onClick={() => setCurlOpen(true)}>
              <FileCode2 className="size-3.5" /> cURL'den İçe Aktar
            </Button>
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="İstek Adı">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Örn. Müşteri Bakiye Sorgu" />
            </Field>
            <Field label="HTTP Metodu">
              <Select value={method} onChange={(e) => setMethod(e.target.value)}>
                {METHODS.map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </Select>
            </Field>
            <Field label="Varsayılan Çalışma Modu">
              <Select value={executionMode} onChange={(e) => setExecutionMode(e.target.value as "client" | "server")}>
                <option value="client">Kullanıcı IP'si (Önerilen)</option>
                <option value="server">Santral-C Sunucu IP'si</option>
              </Select>
            </Field>
          </div>

          <Field label="Hedef URL Şablonu">
            <Input
              value={urlTemplate}
              onChange={(e) => setUrlTemplate(e.target.value)}
              placeholder="{{base_url}}/v1/customers/{{phone}}"
              className="font-mono text-xs"
            />
          </Field>

          <Field label="Kısa Açıklama (Opsiyonel)">
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Teknik destek uzmanı bu isteği hangi durumda kullanmalı?"
            />
          </Field>

          <div className="space-y-2 rounded-xl border border-border/60 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold">Değişken Parametreler (Form Alanları)</span>
              <div className="flex items-center gap-2">
                <Button type="button" variant="secondary" className="h-7 px-2.5 text-xs" onClick={detectVariables}>
                  <Sparkles className="size-3.5" /> Şablondan Algıla
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  className="h-7 px-2.5 text-xs"
                  onClick={() =>
                    setVariables([
                      ...variables,
                      { key: "", label: "", defaultValue: "", required: true, source: "manual", placeholder: "" },
                    ])
                  }
                >
                  <Plus className="size-3.5" /> Ekle
                </Button>
              </div>
            </div>
            {variables.length === 0 ? (
              <p className="text-xs text-muted-foreground">Henüz form alanı eklenmedi. URL veya gövdede {"{{phone}}"} gibi ifadeler yazıp "Şablondan Algıla"ya basın.</p>
            ) : (
              <div className="space-y-2">
                {variables.map((v, idx) => (
                  <div key={idx} className="grid items-center gap-2 sm:grid-cols-12">
                    <Input
                      className="h-8 font-mono text-xs sm:col-span-3"
                      placeholder="anahtar"
                      value={v.key}
                      onChange={(e) => {
                        const next = [...variables];
                        next[idx] = { ...v, key: e.target.value };
                        setVariables(next);
                      }}
                    />
                    <Input
                      className="h-8 text-xs sm:col-span-3"
                      placeholder="Etiket (Örn. Telefon)"
                      value={v.label}
                      onChange={(e) => {
                        const next = [...variables];
                        next[idx] = { ...v, label: e.target.value };
                        setVariables(next);
                      }}
                    />
                    <Input
                      className="h-8 text-xs sm:col-span-2"
                      placeholder="Varsayılan"
                      value={v.defaultValue}
                      onChange={(e) => {
                        const next = [...variables];
                        next[idx] = { ...v, defaultValue: e.target.value };
                        setVariables(next);
                      }}
                    />
                    <Select
                      className="h-8 text-xs sm:col-span-2"
                      value={v.source}
                      onChange={(e) => {
                        const next = [...variables];
                        next[idx] = { ...v, source: e.target.value };
                        setVariables(next);
                      }}
                    >
                      <option value="manual">Manuel</option>
                      <option value="call_phone">Çağrı No</option>
                      <option value="call_name">Çağrı Adı</option>
                    </Select>
                    <label className="flex items-center gap-1 text-xs sm:col-span-1">
                      <input
                        type="checkbox"
                        checked={v.required}
                        onChange={(e) => {
                          const next = [...variables];
                          next[idx] = { ...v, required: e.target.checked };
                          setVariables(next);
                        }}
                      />
                      Zor.
                    </label>
                    <button
                      type="button"
                      onClick={() => setVariables(variables.filter((_, i) => i !== idx))}
                      className="flex h-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive sm:col-span-1"
                      aria-label="Değişkeni sil"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2 rounded-xl border border-border/60 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold">Header Bilgileri (Şifreli Saklanır)</span>
              <Button
                type="button"
                variant="secondary"
                className="h-7 px-2.5 text-xs"
                onClick={() => setHeaders([...headers, { key: "", value: "", secret: false }])}
              >
                <Plus className="size-3.5" /> Header Ekle
              </Button>
            </div>
            {headers.map((h, idx) => (
              <div key={idx} className="grid items-center gap-2 sm:grid-cols-12">
                <Input
                  className="h-8 font-mono text-xs sm:col-span-4"
                  placeholder="Authorization"
                  value={h.key}
                  onChange={(e) => {
                    const next = [...headers];
                    next[idx] = { ...h, key: e.target.value };
                    setHeaders(next);
                  }}
                />
                <Input
                  className="h-8 font-mono text-xs sm:col-span-6"
                  type={h.secret ? "password" : "text"}
                  placeholder="Bearer ..."
                  value={h.value}
                  onChange={(e) => {
                    const next = [...headers];
                    next[idx] = { ...h, value: e.target.value };
                    setHeaders(next);
                  }}
                />
                <button
                  type="button"
                  onClick={() => {
                    const next = [...headers];
                    next[idx] = { ...h, secret: !h.secret };
                    setHeaders(next);
                  }}
                  title="Gizli header olarak işaretle"
                  className={`flex h-8 items-center justify-center gap-1 rounded-lg border px-2 text-xs sm:col-span-1 ${
                    h.secret ? "border-amber-500/40 bg-amber-500/10 text-amber-600" : "border-border/60 text-muted-foreground"
                  }`}
                >
                  <Lock className="size-3" />
                </button>
                <button
                  type="button"
                  onClick={() => setHeaders(headers.filter((_, i) => i !== idx))}
                  className="flex h-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive sm:col-span-1"
                >
                  <Trash2 className="size-3.5" />
                </button>
              </div>
            ))}
          </div>

          <div className="space-y-2 rounded-xl border border-border/60 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold">Query Parametreleri</span>
              <Button
                type="button"
                variant="secondary"
                className="h-7 px-2.5 text-xs"
                onClick={() => setQueryParams([...queryParams, { key: "", value: "" }])}
              >
                <Plus className="size-3.5" /> Parametre Ekle
              </Button>
            </div>
            {queryParams.map((q, idx) => (
              <div key={idx} className="grid items-center gap-2 sm:grid-cols-12">
                <Input
                  className="h-8 font-mono text-xs sm:col-span-5"
                  placeholder="parametre"
                  value={q.key}
                  onChange={(e) => {
                    const next = [...queryParams];
                    next[idx] = { ...q, key: e.target.value };
                    setQueryParams(next);
                  }}
                />
                <Input
                  className="h-8 font-mono text-xs sm:col-span-6"
                  placeholder="değer veya {{degisken}}"
                  value={q.value}
                  onChange={(e) => {
                    const next = [...queryParams];
                    next[idx] = { ...q, value: e.target.value };
                    setQueryParams(next);
                  }}
                />
                <button
                  type="button"
                  onClick={() => setQueryParams(queryParams.filter((_, i) => i !== idx))}
                  className="flex h-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive sm:col-span-1"
                >
                  <Trash2 className="size-3.5" />
                </button>
              </div>
            ))}
          </div>

          {method !== "GET" && method !== "HEAD" && (
            <div className="space-y-2">
              <Field label="Content-Type">
                <Input value={contentType} onChange={(e) => setContentType(e.target.value)} className="font-mono text-xs" />
              </Field>
              <Field label="İstek Gövdesi (Body Şablonu)">
                <textarea
                  rows={5}
                  value={bodyTemplate}
                  onChange={(e) => setBodyTemplate(e.target.value)}
                  placeholder='{"phone":"{{phone}}"}'
                  className="w-full rounded-xl border border-border/70 bg-muted/40 p-3 font-mono text-xs text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
                />
              </Field>
            </div>
          )}

          <Field label="Sorun Giderme Notu / Runbook (Opsiyonel)">
            <textarea
              rows={2}
              value={runbook}
              onChange={(e) => setRunbook(e.target.value)}
              placeholder="Örn. 401/403 hatası alınırsa müşteriden teknisyenin IP adresini whitelist'e eklemesini isteyin."
              className="w-full rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2 text-xs text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
            />
          </Field>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="flex items-center justify-between rounded-xl border border-border/60 px-3.5 py-2.5">
              <div>
                <p className="text-xs font-semibold">Sunucu Proxy Yedeğine İzin Ver</p>
                <p className="text-[11px] text-muted-foreground">CORS engeli çıkarsa sunucu üzerinden göndermeyi önerir.</p>
              </div>
              <Switch on={allowServerFallback} onChange={setAllowServerFallback} label="Sunucu proxy yedeği" />
            </div>
            {editing && (
              <div className="flex items-center justify-between rounded-xl border border-border/60 px-3.5 py-2.5">
                <div>
                  <p className="text-xs font-semibold">İstek Aktif</p>
                  <p className="text-[11px] text-muted-foreground">Pasif şablonlar listede gizlenir.</p>
                </div>
                <Switch on={active} onChange={setActive} label="İstek aktif" />
              </div>
            )}
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
      </Modal>
      <CurlImportModal open={curlOpen} onClose={() => setCurlOpen(false)} onImported={applyCurl} />
    </>
  );
}
