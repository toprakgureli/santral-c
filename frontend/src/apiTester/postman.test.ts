import { describe, expect, it } from "vitest";
import { parsePostmanCollection, templatizeRequestInput } from "./postman";
import type { RequestInput } from "./types";

const SAMPLE_POSTMAN = JSON.stringify({
  info: {
    name: "Müşteri Kargo API",
    description: "Kargo takip ve iptal servisleri",
    schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
  },
  auth: {
    type: "bearer",
    bearer: [{ key: "token", value: "pm-secret-token" }],
  },
  item: [
    {
      name: "Siparişler",
      item: [
        {
          name: "Gönderi Sorgula",
          request: {
            method: "GET",
            url: {
              raw: "https://api.kargo.example/v1/shipments/:tracking_no?phone=05551234567&disabled=skip",
              query: [
                { key: "phone", value: "05551234567" },
                { key: "disabled", value: "skip", disabled: true },
              ],
              variable: [{ key: "tracking_no", value: "TRK-99" }],
            },
          },
        },
        {
          name: "Talep Oluştur",
          request: {
            method: "POST",
            url: "https://api.kargo.example/v1/tickets",
            body: {
              mode: "raw",
              options: { raw: { language: "json" } },
              raw: '{"phone":"05551234567","amount":120,"note":"Acil"}',
            },
          },
        },
      ],
    },
  ],
});

describe("postman collection parser & {{}} templatizer", () => {
  it("converts Postman collection into {{}} template variables when requested", () => {
    const res = parsePostmanCollection(SAMPLE_POSTMAN, {
      asTemplate: true,
      templatizeBaseUrl: true,
      keepDefaults: true,
    });

    expect(res.collection.name).toBe("Müşteri Kargo API");
    expect(res.collection.baseUrl).toBe("https://api.kargo.example");
    expect(res.requests).toHaveLength(2);

    const getReq = res.requests[0];
    expect(getReq.name).toBe("Siparişler / Gönderi Sorgula");
    expect(getReq.urlTemplate).toBe("{{base_url}}/v1/shipments/{{tracking_no}}");
    expect(getReq.queryParams).toEqual([{ key: "phone", value: "{{phone}}" }]);
    expect(getReq.headers).toEqual([
      { key: "Authorization", value: "Bearer pm-secret-token", secret: true },
    ]);
    expect(getReq.variables.find((v) => v.key === "phone")).toMatchObject({
      defaultValue: "05551234567",
      source: "call_phone",
    });
    expect(getReq.variables.find((v) => v.key === "tracking_no")?.defaultValue).toBe("TRK-99");

    const postReq = res.requests[1];
    expect(postReq.urlTemplate).toBe("{{base_url}}/v1/tickets");
    expect(postReq.bodyTemplate).toContain('"phone": "{{phone}}"');
    expect(postReq.bodyTemplate).toContain('"amount": {{amount}}');
    expect(postReq.variables.map((v) => v.key)).toEqual(["phone", "amount", "note"]);
  });

  it("keeps raw concrete values when asTemplate is false", () => {
    const res = parsePostmanCollection(SAMPLE_POSTMAN, {
      asTemplate: false,
      templatizeBaseUrl: false,
      keepDefaults: false,
    });

    expect(res.collection.baseUrl).toBe("");
    expect(res.requests[0].urlTemplate).toBe("https://api.kargo.example/v1/shipments/:tracking_no");
    expect(res.requests[0].queryParams).toEqual([{ key: "phone", value: "05551234567" }]);
    expect(res.requests[1].bodyTemplate).toBe('{"phone":"05551234567","amount":120,"note":"Acil"}');
  });

  it("supports templatizing cURL-imported requests on demand", () => {
    const base: RequestInput = {
      name: "POST /check",
      description: "",
      method: "POST",
      urlTemplate: "https://erp.example.com/api/check",
      headers: [{ key: "Authorization", value: "Bearer abc", secret: true }],
      queryParams: [{ key: "gsm", value: "5550001122" }],
      bodyTemplate: '{"customerId":"C-10"}',
      contentType: "application/json",
      variables: [],
      executionMode: "client",
      allowServerFallback: true,
      runbook: "",
    };

    const { request, extractedBaseUrl } = templatizeRequestInput(base, {
      asTemplate: true,
      templatizeBaseUrl: true,
      keepDefaults: false,
    });
    expect(extractedBaseUrl).toBe("https://erp.example.com");
    expect(request.urlTemplate).toBe("{{base_url}}/api/check");
    expect(request.queryParams).toEqual([{ key: "gsm", value: "{{gsm}}" }]);
    expect(request.bodyTemplate).toContain('"customerId": "{{customerId}}"');
    expect(request.variables.find((v) => v.key === "gsm")).toMatchObject({
      defaultValue: "",
      placeholder: "5550001122",
      source: "call_phone",
    });
  });
});
