"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DANGER_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";

/**
 * Confirms a password rotation.
 *
 * Rotation is not destructive to data, but it immediately breaks every App
 * still using the old credential. The dialog says that plainly and keeps the
 * confirm control behind an explicit click rather than a hover action.
 */
export function RotateDatabasePasswordDialog({
  open,
  onOpenChange,
  databaseName,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  databaseName: string;
  onConfirm: () => Promise<void>;
}) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setPending(false);
      setError(null);
    }
    onOpenChange(next);
  };

  const submit = async () => {
    if (pending) return;
    setError(null);
    setPending(true);
    try {
      await onConfirm();
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not rotate the password.",
      );
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Rotate database password</DialogTitle>
          <DialogDescription>
            A new password is generated for {databaseName}. Every App still
            using the old password loses access until its environment is
            updated.
          </DialogDescription>
        </DialogHeader>

        {error ? (
          <p
            role="alert"
            className="m-0 mt-3 text-[12px] leading-4 text-[var(--projects-danger)]"
          >
            {error}
          </p>
        ) : null}

        <DialogFooter className="mt-4">
          <Button
            type="button"
            className={SECONDARY_ACTION}
            onClick={() => handleOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            className={DANGER_ACTION}
            disabled={pending}
            onClick={() => void submit()}
          >
            {pending ? "Rotating…" : "Rotate password"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
