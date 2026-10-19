"use client";

import { useId, useState } from "react";
import type { components } from "@/api/generated/schema";
import { CreateSiteRequestFramework } from "@/api/generated/schema";
import { useCreateSite } from "@/api/mutations/sites";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import { isSlug, toSlug } from "@/lib/utils";

type Site = components["schemas"]["Site"];

/**
 * Creates a site in the project.
 *
 * The API takes the site name as a DNS label, so the field normalizes while the
 * operator types and states the rule before submit rather than after a failed
 * request. The address is derived from the name on the server: a name already
 * claimed by another resource gets a suffix instead of an error, so the dialog
 * promises an address rather than promising this exact one.
 */
export function CreateSiteDialog({
  projectId,
  open,
  onOpenChange,
  onCreated,
}: {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Hands the new site back so the caller can offer a deployment next. */
  onCreated?: (site: Site) => void;
}) {
  const create = useCreateSite(projectId);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  // Reset on close, not in an effect: the dialog stays mounted so Radix can
  // animate it out, and clearing here keeps a cancelled attempt out of the next.
  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setName("");
      setError(null);
    }
    onOpenChange(next);
  };

  const normalized = toSlug(name);
  const valid = isSlug(normalized);

  const submit = async () => {
    if (!valid) return;
    setError(null);
    try {
      const created = await create.mutateAsync({
        name: normalized,
        framework: CreateSiteRequestFramework.static,
        enabled: true,
      });
      handleOpenChange(false);
      const site = created?.site;
      if (site) onCreated?.(site);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Could not create the site.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {/* `data-user-surface` opts this portaled dialog into the user console's
          light palette and accent (see admin.css). */}
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create site</DialogTitle>
          <DialogDescription>
            A site serves built static files from one address. The address is
            derived from the name, so the name is a DNS label: lower case,
            numbers, and hyphens.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Site name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="docs"
            // Re-themed from the dark-only shared Input so the dialog holds up
            // in the light console theme as well.
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setName(event.target.value)}
          />
          <p
            id={helpId}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {name.length > 0 && !valid
              ? "Use 2 to 63 characters: lower-case letters, numbers, and hyphens."
              : normalized
                ? `Will be created as ${normalized}`
                : "Lower-case letters, numbers, and hyphens."}
          </p>

          {error ? (
            <p
              role="alert"
              className="m-0 text-[12px] leading-4 text-[var(--projects-danger)]"
            >
              {error}
            </p>
          ) : null}

          <DialogFooter className="mt-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => handleOpenChange(false)}
              className={SECONDARY_ACTION}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!valid || create.isPending}
              className={PRIMARY_ACTION}
            >
              {create.isPending ? "Creating…" : "Create site"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
