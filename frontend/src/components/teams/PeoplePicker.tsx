// PeoplePicker: search and tick users. Used to create a group, to add or
// invite members, and to start a direct message.

import { useEffect, useMemo, useState } from "react";
import { Check, Search } from "lucide-react";
import { api } from "@/api/client";
import type { TeamsPerson } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";

export default function PeoplePicker({
  selected,
  onChange,
  exclude = [],
  single = false,
  height = 240,
}: {
  selected: number[];
  onChange: (ids: number[]) => void;
  exclude?: number[];
  single?: boolean;
  height?: number;
}) {
  const [people, setPeople] = useState<TeamsPerson[]>([]);
  const [q, setQ] = useState("");

  useEffect(() => {
    api.teamsPeople().then(setPeople).catch(() => setPeople([]));
  }, []);

  const visible = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase("tr");
    return people.filter((p) => !exclude.includes(p.id) && (!needle || p.name.toLocaleLowerCase("tr").includes(needle)));
  }, [people, q, exclude]);

  function toggle(id: number) {
    if (single) {
      onChange([id]);
      return;
    }
    onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Kişi ara..."
          className="h-10 w-full rounded-xl border border-border/70 bg-muted/40 pl-10 pr-3 text-sm outline-none transition placeholder:text-muted-foreground/60 hover:border-border focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
        />
      </div>
      <ul className="overflow-y-auto rounded-xl border border-border/70 bg-muted/20 p-1" style={{ maxHeight: height }}>
        {visible.map((p) => {
          const on = selected.includes(p.id);
          return (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => toggle(p.id)}
                className={cn("flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm transition-colors hover:bg-accent", on && "bg-primary/10")}
              >
                <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
                <span className="min-w-0 flex-1 truncate font-medium">{p.name}</span>
                {on && <Check className="size-4 shrink-0 text-success" />}
              </button>
            </li>
          );
        })}
        {visible.length === 0 && <li className="px-3 py-6 text-center text-xs text-muted-foreground">Kişi bulunamadı.</li>}
      </ul>
    </div>
  );
}
