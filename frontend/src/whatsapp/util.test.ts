import { describe, expect, it } from "vitest";
import type { WAMessage } from "@/whatsapp/types";
import { newestSeen, prettyPhone } from "@/whatsapp/util";

const m = (id: number, pending = false) => ({ id, pending, conversationId: 1, direction: "in", kind: "text", body: "", status: "received", createdAt: "", sender: { kind: "customer" } }) as WAMessage;

describe("newestSeen", () => {
  it("is the newest stored message on screen, a message still being sent left out", () => {
    expect(newestSeen([m(4), m(9), m(7), m(-1700000000000, true)])).toBe(9);
    expect(newestSeen([])).toBe(0);
    expect(newestSeen([m(-5, true)])).toBe(0);
  });
});

describe("prettyPhone", () => {
  it("writes a Turkish mobile number in groups and leaves a hidden one as it came", () => {
    expect(prettyPhone("905321234567")).toBe("+90 532 123 45 67");
    expect(prettyPhone("••••••••••67")).toBe("••••••••••67");
  });
});
