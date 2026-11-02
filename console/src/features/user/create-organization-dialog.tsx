"use client";

import { useId, useState } from "react";
import { useCreateOrganization } from "@/api/mutations/organizations";
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

/**
 * Creates an organization from the user console.
 *
 * The API requires both a display name and a unique slug. The slug follows the
 * name until the operator edits it, then their value wins so a later name change
 * cannot silently overwrite it. Validation is stated up front rather than left
 * to a submit-time 422, and the server error is surfaced verbatim because the
 * API is the authority on what is already taken.
 */
export function CreateOrganizationDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (organization: { id: string; name: string; slug: string }) => void;
}) {
  const create = useCreateOrganization();
  const [name, setName] = useState("");
  // Null means "follow the name". Once the operator edits the slug, their value
  // is kept so a later name change cannot silently overwrite it.
  const [slugOverride, setSlugOverride] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const nameId = useId();
  const slugId = useId();
  const nameHelpId = `${nameId}-help`;
  const slugHelpId = `${slugId}-help`;

  // Reset on close rather than in an effect: the dialog stays mounted so Radix
  // can animate it out, and clearing here means a cancelled attempt never leaks
  // into the next one.
  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setName("");
      setSlugOverride(null);
      setError(null);
    }
    onOpenChange(next);
  };

  const trimmedName = name.trim();
  const slug = slugOverride ?? toSlug(name);
  const nameValid = trimmedName.length >= 2 && trimmedName.length <= 120;
  const slugValid = isSlug(slug);
  const valid = nameValid && slugValid;

  const submit = async () => {
    if (!valid) return;
    setError(null);
    try {
      const created = await create.mutateAsync({
        name: trimmedName,
        slug,
      });
      if (created?.organization) onCreated?.(created.organization);
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not create the organization.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {/* `data-user-surface` opts this portaled dialog into the user console's
          light palette and accent (see admin.css). */}
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create organization</DialogTitle>
          <DialogDescription>
            An organization holds your projects, members, and billing. You
            become its owner, and you can invite teammates later.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor={nameId}>Organization name</Label>
            <Input
              id={nameId}
              value={name}
              autoFocus
              autoComplete="off"
              aria-describedby={nameHelpId}
              aria-invalid={name.length > 0 && !nameValid}
              placeholder="Acme Inc"
              // Re-themed from the dark-only shared Input so the dialog holds up
              // in the light console theme as well.
              className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
              onChange={(event) => setName(event.target.value)}
            />
            <p
              id={nameHelpId}
              className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
            >
              {name.length > 0 && !nameValid
                ? "Use 2 to 120 characters."
                : "The name your teammates will see."}
            </p>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={slugId}>URL slug</Label>
            <Input
              id={slugId}
              value={slug}
              autoComplete="off"
              spellCheck={false}
              aria-describedby={slugHelpId}
              aria-invalid={slug.length > 0 && !slugValid}
              placeholder="acme"
              className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
              onChange={(event) => setSlugOverride(toSlug(event.target.value))}
            />
            <p
              id={slugHelpId}
              className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
            >
              {slug.length > 0 && !slugValid
                ? "Use 2 to 63 characters: lower-case letters, numbers, and hyphens."
                : slug
                  ? `Your organization will be reachable at /${slug}`
                  : "Lower-case letters, numbers, and hyphens."}
            </p>
          </div>

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
              {create.isPending ? "Creating…" : "Create organization"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
