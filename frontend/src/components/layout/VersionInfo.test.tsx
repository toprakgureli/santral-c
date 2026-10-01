import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import VersionInfo from "./VersionInfo";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | null = null;
let host: HTMLDivElement | null = null;

async function show(server: { version: string; buildTime: string; build?: string }) {
  vi.spyOn(api, "version").mockResolvedValue(server);
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root!.render(<VersionInfo collapsed={false} />);
  });
  return host.textContent ?? "";
}

describe("version line", () => {
  afterEach(() => {
    act(() => root?.unmount());
    host?.remove();
    vi.restoreAllMocks();
  });

  // The panel's own files carry only a build tag ("dev" in tests); the
  // commit comes from the server.
  it("shows the server's version and says the page is current when the tags match", async () => {
    const text = await show({ version: "abc1234", buildTime: "t", build: "dev" });
    expect(text).toContain("sürüm abc1234");
    expect(text).toContain("güncel");
  });

  it("asks for a reload when the page is from another deploy", async () => {
    const text = await show({ version: "def5678", buildTime: "t", build: "9f2c01aa77e3" });
    expect(text).toContain("sürüm def5678");
    expect(text).toContain("sayfayı yenile");
  });
});
