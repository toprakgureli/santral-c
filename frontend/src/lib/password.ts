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
const LOWER = "abcdefghjkmnpqrstuvwxyz";
const DIGIT = "23456789";
const SPECIAL = "!@#$%&*?+-=/";

// rng hands out CSPRNG values one at a time.
function rng() {
  const pool = new Uint32Array(64);
  crypto.getRandomValues(pool);
  let i = 0;
  return (n: number) => pool[i++ % pool.length] % n;
}

function draw(chars: string, next: (n: number) => number, count: number) {
  let out = "";
  for (let i = 0; i < count; i++) out += chars[next(chars.length)];
  return out;
}

// generatePassword builds a readable password in the house shape, e.g.
// "W7D/54262" or "ASDF%FS345": mostly upper-case letters with one or two
// lower-case ones, one or two special characters splitting the letters, and a
// block of four to six digits. Look-alike characters (0/O, 1/l/I) are left out
// so it can be read out or typed from a note. Every policy rule is satisfied.
export function generatePassword(): string {
  const next = rng();
  const digits = 4 + next(3); // 4-6
  const specials = 1 + next(2); // 1-2
  const letters = 4 + next(3); // 4-6, always leaves room under PASSWORD_MAX

  // Letter pool: upper case with one or two lower case mixed in.
  const lowerCount = 1 + next(2);
  const pool = (draw(UPPER, next, letters - lowerCount) + draw(LOWER, next, lowerCount)).split("");
  for (let i = pool.length - 1; i > 0; i--) {
    const j = next(i + 1);
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }

  // Split the letters into (specials + 1) chunks and put a special between
  // each pair, so the special never lands at the very edge.
  const chunks: string[] = [];
  let rest = pool.join("");
  for (let s = 0; s < specials; s++) {
    const remainingChunks = specials - s + 1;
    const maxCut = rest.length - (remainingChunks - 1);
    const cut = 1 + next(Math.max(1, maxCut - 1));
    chunks.push(rest.slice(0, cut));
    rest = rest.slice(cut);
  }
  chunks.push(rest);
  const parts: string[] = [];
  chunks.forEach((chunk, i) => {
    parts.push(chunk);
    if (i < specials) parts.push(SPECIAL[next(SPECIAL.length)]);
  });

  // The digit block goes at the end or right after a special character.
  const block = draw(DIGIT, next, digits);
  const slots = parts.map((_, i) => i).filter((i) => i === parts.length || parts[i - 1]?.length === 1 && SPECIAL.includes(parts[i - 1]));
  slots.push(parts.length);
  parts.splice(slots[next(slots.length)], 0, block);
  return parts.join("");
}
