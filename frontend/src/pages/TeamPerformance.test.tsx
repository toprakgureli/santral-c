import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamPerformance as TeamPerformancePayload } from "../api/types";
import { TeamPerformance } from "./TeamPerformance";

const auth = vi.hoisted(() => ({ permissions: [] as string[] }));
vi.mock("../auth/AuthContext", () => ({
  useAuth: () => ({ can: (p: string) => auth.permissions.includes(p) }),
}));

const samplePayload: TeamPerformancePayload = {
  scope: "all",
  from: "2026-11-06",
  to: "2026-11-06",
  items: [
    {
      userId: 10,
      name: "Ayşe Kaya",
      extension: "1002",
      roles: ["Satış Ekibi"],
      status: "available",
      shift: { open: true, seconds: 3600 },
      calls: {
        total: 7,
        answered: 6,
        long: 5,
        short: 1,
        unanswered: 1,
        inbound: 2,
        outbound: 5,
        inboundMissed: 0,
        outboundMissed: 1,
        inboundReal: 2,
        outboundReal: 3,
        talkSeconds: 600,
        avgTalkSeconds: 120,
        longestSeconds: 180,
        avgAnswerSeconds: 4,
        peers: 5,
        over5: 0,
        over10: 0,
        over20: 0,
      },
      escalations: 0,
      breakSeconds: 0,
      recent: [
        {
          peer: "05551112233",
          direction: "outbound",
          disposition: "answered",
          startedAt: "2026-11-06T08:00:00Z",
          durationSeconds: 95,
        },
      ],
    },
  ],
};

vi.mock("../api/client", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    performanceToday: vi.fn(() => Promise.resolve(samplePayload)),
    agentCalls: vi.fn(() => Promise.resolve({ userId: 10, from: "2026-11-06", to: "2026-11-06", items: [] })),
    realCall: vi.fn(() => Promise.resolve({ seconds: 30 })),
  },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | null = null;
let box: HTMLDivElement;

async function renderWithPerms(...perms: string[]) {
  auth.permissions = perms;
  box = document.createElement("div");
  document.body.appendChild(box);
  root = createRoot(box);
  await act(async () => {
    root!.render(
      <MemoryRouter>
        <TeamPerformance />
      </MemoryRouter>,
    );
  });
  return box;
}

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  box?.remove();
});

describe("TeamPerformance permissions", () => {
  it("hides the Paylaş button when performance.share is missing and shows it when granted", async () => {
    const withoutShare = await renderWithPerms("performance.view_all");
    expect(withoutShare.textContent).not.toContain("Paylaş");

    act(() => root?.unmount());
    box.remove();

    const withShare = await renderWithPerms("performance.view_all", "performance.share");
    expect(withShare.textContent).toContain("Paylaş");
  });

  it("hides Son görüşmeler in agent detail when performance.recent_calls is missing and shows it when granted", async () => {
    const withoutRecent = await renderWithPerms("performance.view_all");
    const rowBtn = [...withoutRecent.querySelectorAll("button")].find((b) => b.textContent?.includes("Ayşe Kaya"));
    expect(rowBtn).toBeDefined();
    await act(async () => {
      rowBtn!.click();
    });
    expect(withoutRecent.textContent).toContain("Profili aç");
    expect(withoutRecent.textContent).not.toContain("Son görüşmeler");
    expect(withoutRecent.textContent).not.toContain("Seçilen tarihlerin bütün görüşmeleri");

    act(() => root?.unmount());
    box.remove();

    const withRecent = await renderWithPerms("performance.view_all", "performance.recent_calls");
    const rowBtn2 = [...withRecent.querySelectorAll("button")].find((b) => b.textContent?.includes("Ayşe Kaya"));
    expect(rowBtn2).toBeDefined();
    await act(async () => {
      rowBtn2!.click();
    });
    expect(withRecent.textContent).toContain("Son görüşmeler");
    expect(withRecent.textContent).toContain("Seçilen tarihlerin bütün görüşmeleri");
  });
});
