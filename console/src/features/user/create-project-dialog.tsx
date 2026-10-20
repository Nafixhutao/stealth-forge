"use client";

import { useId, useState } from "react";
import { useCreateProject } from "@/api/mutations/projects";
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
import { toSlug, isSlug } from "@/lib/utils";

/**
 * Creates a project in the selected organization.
 *
 * The API accepts a slug as the project name, so the field normalizes as the
 * operator types and states the rule up front instead of failing on submit. The
 * server error is surfaced verbatim, because the API is the authority on what
 * is already taken.
 */
export function CreateProjectDialog({
  organizationId,
  organizationName,
  open,
  onOpenChange,
}: {
  organizationId: string;
  organizationName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateProject(organizationId);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  // Reset on close rather than in an effect: the dialog stays mounted so Radix
  // can animate it out, and clearing here means a cancelled attempt never leaks
  // into the next one.
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
      await create.mutateAsync({ name: normalized });
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not create the project.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {/* `data-user-surface` opts this portaled dialog into the user console's
          light palette and accent (see admin.css). */}
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create project</DialogTitle>
          <DialogDescription>
            The project is created in {organizationName}. Its name is also its
            slug, so it is lower case and hyphenated.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Project name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="checkout-api"
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
              {create.isPending ? "Creating…" : "Create project"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
