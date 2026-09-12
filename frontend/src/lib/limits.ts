// Field length limits. These mirror the backend validate:"max=..." values; if
// the two drift the user fills a form and then gets a server error.
//
// CharCount counts code points, not .length, so Turkish letters and emoji are
// not double counted.

export const LIMITS = {
  roleName: 60,
  roleDisplayName: 120,
  roleDescription: 255,
  userName: 120,
  email: 255,
  sipExtension: 32,
};

export function charCount(value: string) {
  return [...value].length;
}
