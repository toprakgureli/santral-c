import { describe, expect, it } from "vitest";
import type { WAMessage } from "@/whatsapp/types";
import { isBSUID, newestSeen, prettyPhone } from "@/whatsapp/util";

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
    expect(prettyPhone("TR.1502576394655843")).toBe("Kullanıcı Adı (Numara gizli)");
    expect(prettyPhone("")).toBe("Numara gizli");
    expect(prettyPhone(undefined)).toBe("Numara gizli");
  });
});

describe("isBSUID", () => {
  it("detects Meta Business-Scoped User IDs", () => {
    expect(isBSUID("TR.1502576394655843")).toBe(true);
    expect(isBSUID("US.987654321")).toBe(true);
    expect(isBSUID("905321234567")).toBe(false);
    expect(isBSUID("")).toBe(false);
    expect(isBSUID(undefined)).toBe(false);
  });
});

