import { ApiError } from "@/api/client";

export const FALLBACK_ERROR = "Bir hata oluştu.";

// errorMessage turns a thrown API error into the localized text the backend
// already provides, falling back to a generic message for network failures.
export function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : FALLBACK_ERROR;
}

// SUCCESS_DELAY_MS lets the green "success" button state play before the
// session switches over to the app, so the sign-in ends on a visible tick.
export const SUCCESS_DELAY_MS = 900;
