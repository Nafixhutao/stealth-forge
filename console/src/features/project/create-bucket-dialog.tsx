"use client";

import { useId, useState } from "react";
import { useCreateBucket } from "@/api/mutations/storage";
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
import { cn, isSlug, toSlug } from "@/lib/utils";

/**
 * Creates a storage bucket in the project.
 *
 * The name becomes part of the bucket's identity, so it is normalized to a DNS
 * label as the operator types. `file_security` is exposed because it changes how
 * every file in the bucket is authorised, and it is on by default in the API:
 * per-file permissions unless the operator deliberately turns them off.
 */
export function CreateBucketDialog({
  projectId,
  open,
  onOpenChange,
}: {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateBucket(projectId);
  const [name, setName] = useState("");
  const [fileSecurity, setFileSecurity] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setName("");
      setFileSecurity(true);
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
      await create.mutateAsync({
        name: normalized,
        file_security: fileSecurity,
      });
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Could not create the bucket.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create bucket</DialogTitle>
          <DialogDescription>
            A bucket holds the project&apos;s files. Its name is also its
            identity, so it is lower case and hyphenated.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Bucket name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="avatars"
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

          <label
            className={cn(
              "mt-1 flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 transition-colors",
              fileSecurity
                ? "border-[var(--projects-border-hover)] bg-[var(--projects-control)]"
                : "border-[var(--projects-border)]",
            )}
          >
            <input
              type="checkbox"
              checked={fileSecurity}
              onChange={(event) => setFileSecurity(event.target.checked)}
              className="mt-0.5 size-4 shrink-0 accent-[var(--user-accent)]"
            />
            <span className="flex min-w-0 flex-col">
              <span className="text-[13px] font-medium text-[var(--projects-text)]">
                Per-file permissions
              </span>
              <span className="text-[12px] leading-4 text-[var(--projects-muted)]">
                {fileSecurity
                  ? "Each file keeps its own permissions. Recommended."
                  : "Every file in the bucket shares the bucket's permissions."}
              </span>
            </span>
          </label>

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
              {create.isPending ? "Creating…" : "Create bucket"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
