import type { LucideIcon } from "lucide-react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * AdminEmptyState is the shared empty-state block used by the Admin views. It
 * preserves the established Admin presentation (dashed card, small icon,
 * secondary action) that differs from the generic `EmptyState` primitive.
 *
 * `descriptionClassName` carries the one legitimate difference between callers
 * that constrain the description width for a centered block.
 */
export function AdminEmptyState({
  icon: Icon,
  title,
  description,
  descriptionClassName,
  actionLabel,
  onAction,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  descriptionClassName?: string;
  actionLabel: string;
  onAction: () => void;
}) {
  return (
    <div className="rounded-xl border border-dashed border-graphite bg-carbon/50 p-10 text-center">
      <Icon className="mx-auto size-5 text-fog" aria-hidden="true" />
      <h2 className="mt-4 text-sm text-paper">{title}</h2>
      <p className={cn("mt-2 text-sm text-fog", descriptionClassName)}>
        {description}
      </p>
      <Button className="mt-5" size="sm" onClick={onAction}>
        <Plus className="size-3.5" aria-hidden="true" /> {actionLabel}
      </Button>
    </div>
  );
}
