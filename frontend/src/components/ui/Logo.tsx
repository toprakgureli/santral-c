import { cn } from "@/lib/utils";

// The SantralC mark: a dark rounded square, a white C that reads as a call
// wave, a green dot and two signal arcs. Drawn inline so it scales crisply
// and needs no image request; the square is always dark, so the mark works
// on both themes. `wordmark` adds the product name in the current text
// colour (the source files only ship a white wordmark).
export default function Logo({ className, wordmark, wordmarkClassName }: { className?: string; wordmark?: boolean; wordmarkClassName?: string }) {
  const mark = (
    <svg viewBox="0 0 100 100" className={cn("shrink-0", className)} aria-hidden="true" focusable="false">
      <rect width="100" height="100" rx="23" fill="#1E2128" />
      <path d="M 37 26 C 22 30 16 43 17 55 C 18 70 30 81 45 81 C 51 81 58 79 63 74" fill="none" stroke="#FFFFFF" strokeWidth="9" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="44" cy="51" r="6.7" fill="#3CB371" />
      <path d="M 55 29 C 67 33 72 40 73 51" fill="none" stroke="#3CB371" strokeWidth="7" strokeLinecap="round" />
      <path d="M 56 15 C 74 19 85 33 85 51" fill="none" stroke="#3CB371" strokeWidth="7" strokeLinecap="round" />
    </svg>
  );
  if (!wordmark) return mark;
  return (
    <span className="inline-flex items-center gap-3">
      {mark}
      <span className={cn("font-semibold tracking-[-0.04em] text-foreground", wordmarkClassName)}>SantralC</span>
    </span>
  );
}
