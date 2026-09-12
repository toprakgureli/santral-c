import { cn } from "@/lib/utils";

// SuccessCheck is the animated check mark shown inside a button once an auth
// step succeeds (stroke draws in, then the icon pops).
export default function SuccessCheck({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" className={cn("check-pop size-4", className)}>
      <path
        d="M5 12.5 L10 17.5 L19 7"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="check-draw"
      />
    </svg>
  );
}
