import { cn } from "@/lib/utils";

// A simple lettermark so no image asset is needed.
export default function Logo({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold tracking-tight",
        className,
      )}
    >
      sc
    </span>
  );
}
