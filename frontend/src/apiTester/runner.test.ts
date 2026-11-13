import { describe, expect, it } from "vitest";
import { buildCurlCommand, extractPlaceholders, parseCurlCommand, renderClientRequest } from "./runner";
import type { ApiCollectionItem, ApiRequestItem } from "./types";

const col: ApiCollectionItem = {
  id: 1,
  name: "Müşteri API",
  description: "",
  projectTag: "ERP",
  baseUrl: "https://api.customer.example/",
  sortOrder: 1,
  active: true,
  requests: [],
  createdAt: "",
  updatedAt: "",
};

const req: ApiRequestItem = {
  id: 10,
  collectionId: 1,
  name: "Sipariş Detay",
  description: "",
  method: "POST",
  urlTemplate: "{{base_url}}/v1/orders/{{order_id}}",
  headers: [{ key: "Authorization", value: "Bearer {{token}}", secret: true }],
  queryParams: [{ key: "phone", value: "{{phone}}" }],
  bodyTemplate: '{"phone":"{{phone}}"}',
  contentType: "application/json",
  variables: [
    { key: "order_id", label: "Sipariş No", defaultValue: "", required: true, source: "manual", placeholder: "" },
    { key: "phone", label: "Telefon", defaultValue: "5551112233", required: false, source: "call_phone", placeholder: "" },
    { key: "token", label: "Token", defaultValue: "abc", required: false, source: "manual", placeholder: "" },
  ],
  executionMode: "client",
  allowServerFallback: true,
  runbook: "",
  sortOrder: 1,
  active: true,
  createdAt: "",
  updatedAt: "",
};

describe("apiTester runner helpers", () => {
  it("extracts placeholders cleanly", () => {
    expect(extractPlaceholders("{{base_url}}/u/{{ phone }}", '{"x":"{{order_id}}"}')).toEqual([
      "base_url",
      "phone",
      "order_id",
    ]);
  });

  it("renders request url, query params, headers and body", () => {
    const out = renderClientRequest(col, req, { order_id: "ORD-77", phone: "5559990011" });
    expect(out.url).toBe("https://api.customer.example/v1/orders/ORD-77?phone=5559990011");
    expect(out.method).toBe("POST");
    expect(out.headers).toEqual([
      { key: "Authorization", value: "Bearer abc", secret: true },
      { key: "Content-Type", value: "application/json" },
    ]);
    expect(out.body).toBe('{"phone":"5559990011"}');
  });

  it("throws when a required variable is missing", () => {
    expect(() => renderClientRequest(col, req, {})).toThrow(/Sipariş No alanı zorunludur/);
  });

  it("parses curl commands and builds equivalent curl output", () => {
    const parsed = parseCurlCommand(
      `curl -X POST "https://api.test.example/v2/check?mode=fast" -H "Authorization: Bearer tok-1" -H "Content-Type: application/json" --data-raw '{"phone":"555"}'`,
    );
    expect(parsed.method).toBe("POST");
    expect(parsed.urlTemplate).toBe("https://api.test.example/v2/check");
    expect(parsed.queryParams).toEqual([{ key: "mode", value: "fast" }]);
    expect(parsed.headers).toEqual([
      { key: "Authorization", value: "Bearer tok-1", secret: true },
      { key: "Content-Type", value: "application/json", secret: false },
    ]);
    expect(parsed.bodyTemplate).toBe('{"phone":"555"}');

    const cmd = buildCurlCommand({
      url: "https://api.test.example/v2/check",
      method: "POST",
      headers: [{ key: "Authorization", value: "Bearer tok-1" }],
      body: '{"phone":"555"}',
    });
    expect(cmd).toContain("curl -i -X POST 'https://api.test.example/v2/check'");
    expect(cmd).toContain("-H 'Authorization: Bearer tok-1'");
    expect(cmd).toContain("--data-raw '{\"phone\":\"555\"}'");
  });
});
