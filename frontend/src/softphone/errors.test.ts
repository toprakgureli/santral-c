// What the agent reads when the phone fails: always plain Turkish.
import { describe, expect, it } from "vitest";
import { PhoneError, phoneMessage } from "./errors";

const fallback = "Çağrı başlatılamadı.";
const named = (name: string, message = "x") => Object.assign(new Error(message), { name });

describe("phone messages", () => {
  it("keeps what the panel wrote itself", () => {
    expect(phoneMessage(new PhoneError("Önce süren görüşmeyi bitir."), fallback)).toBe("Önce süren görüşmeyi bitir.");
    const fromServer = Object.assign(new Error("Hesabın için SIP bilgisi tanımlı değil."), { status: 404, code: "NOT_FOUND" });
    expect(phoneMessage(fromServer, fallback)).toBe("Hesabın için SIP bilgisi tanımlı değil.");
  });

  it("gives microphone advice", () => {
    expect(phoneMessage(named("NotAllowedError"), fallback)).toMatch(/^Mikrofon izni verilmedi/);
    expect(phoneMessage(named("NotFoundError"), fallback)).toMatch(/^Mikrofon bulunamadı/);
    expect(phoneMessage(named("NotReadableError"), fallback)).toMatch(/başka bir uygulama/);
  });

  it("turns library and loader texts into plain sentences", () => {
    expect(phoneMessage(new TypeError("Failed to fetch dynamically imported module: /assets/x.js"), fallback)).toMatch(/yeni sürümü/);
    expect(phoneMessage(new Error("Importing a module script failed."), fallback)).toMatch(/yeni sürümü/);
    expect(phoneMessage(new Error("Transport error."), fallback)).toMatch(/^Santrale bağlanılamadı/);
    expect(phoneMessage(new Error("WebSocket closed"), fallback)).toMatch(/^Santrale bağlanılamadı/);
    expect(phoneMessage(new Error("Invalid state transition from Terminated"), fallback)).toBe(fallback);
    expect(phoneMessage(undefined, fallback)).toBe(fallback);
  });
});
