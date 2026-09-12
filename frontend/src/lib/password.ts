// Account password policy, mirrored from backend/pkg/password: 8 to 16
// characters with an upper-case letter, a lower-case letter, a digit and a
// special character.

export const PASSWORD_MIN = 8;
export const PASSWORD_MAX = 16;

export type PasswordRule = { key: string; label: string; ok: boolean };

// checkPassword evaluates every rule so the form can show which ones the
// current value already satisfies.
export function checkPassword(value: string): PasswordRule[] {
  const len = [...value].length;
  return [
    { key: "length", label: `${PASSWORD_MIN}-${PASSWORD_MAX} karakter`, ok: len >= PASSWORD_MIN && len <= PASSWORD_MAX },
    { key: "upper", label: "Büyük harf", ok: /\p{Lu}/u.test(value) },
    { key: "lower", label: "Küçük harf", ok: /\p{Ll}/u.test(value) },
    { key: "digit", label: "Rakam", ok: /\d/.test(value) },
    { key: "special", label: "Özel karakter", ok: /[^\p{L}\p{N}\s]/u.test(value) },
  ];
}

export function passwordValid(value: string): boolean {
  return checkPassword(value).every((r) => r.ok);
}

const UPPER = "ABCDEFGHJKLMNPQRSTUVWXYZ";
const LOWER = "abcdefghijkmnopqrstuvwxyz";
const DIGIT = "23456789";
const SPECIAL = "!@#$%&*?+-=";

function pick(chars: string, random: Uint32Array, i: number) {
  return chars[random[i] % chars.length];
}

// generatePassword builds a 14-character password that satisfies every rule,
// using the browser's CSPRNG. Look-alike characters (0/O, 1/l/I) are left out so
// the password can be read out or typed from a note.
export function generatePassword(length = 14): string {
  const all = UPPER + LOWER + DIGIT + SPECIAL;
  const random = new Uint32Array(length * 2);
  crypto.getRandomValues(random);
  const chars = [pick(UPPER, random, 0), pick(LOWER, random, 1), pick(DIGIT, random, 2), pick(SPECIAL, random, 3)];
  for (let i = chars.length; i < length; i++) chars.push(pick(all, random, i));
  // Fisher-Yates so the guaranteed classes are not always at the front.
  for (let i = chars.length - 1; i > 0; i--) {
    const j = random[length + i] % (i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join("");
}
