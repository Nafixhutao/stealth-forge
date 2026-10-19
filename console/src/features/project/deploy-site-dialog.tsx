"use client";

import { useId, useRef, useState } from "react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import { useDeploySite } from "@/api/mutations/sites";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import { formatBytes } from "@/lib/format";

type Site = components["schemas"]["Site"];

/** The archive shapes the API accepts for a pre-built deployment. */
const ARCHIVE_EXTENSIONS = [".zip", ".tar", ".tar.gz", ".tgz"];

/** The instance's default artifact ceiling. Only a hint: the API is the
 * authority, and an operator may have raised it. */
const LARGE_ARCHIVE_BYTES = 50 * 1024 * 1024;

function isArchive(file: File) {
  const name = file.name.toLowerCase();
  return ARCHIVE_EXTENSIONS.some((extension) => name.endsWith(extension));
}

/**
 * Publishes a pre-built archive to an existing site.
 *
 * The upload is validated and stored by the API before anything is published,
 * so a rejected archive leaves the previous release serving. The dialog states
 * the one requirement an archive cannot be checked for on this side: the
 * `index.html` at its root.
 */
export function DeploySiteDialog({
  projectId,
  site,
  open,
  onOpenChange,
}: {
  projectId: string;
  site: Site;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const deploy = useDeploySite(projectId);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setFile(null);
      setError(null);
      // The file input is uncontrolled, so its label keeps the old filename
      // unless the element itself is cleared.
      if (inputRef.current) inputRef.current.value = "";
    }
    onOpenChange(next);
  };

  const valid = Boolean(file && isArchive(file));

  const submit = async () => {
    if (!file || !valid) return;
    setError(null);
    try {
      await deploy.mutateAsync({ siteId: site.id, file });
      handleOpenChange(false);
      toast.success(`Deployed ${file.name} to ${site.name}`);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not publish the deployment.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Deploy to {site.name}</DialogTitle>
          <DialogDescription>
            Upload a built site as an archive. It becomes the live release at{" "}
            {site.platform_hostname ?? "this site's address"} as soon as the API
            accepts it.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Site archive</Label>
          <input
            ref={inputRef}
            id={fieldId}
            type="file"
            accept={ARCHIVE_EXTENSIONS.join(",")}
            aria-describedby={helpId}
            aria-invalid={Boolean(file && !valid)}
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null);
              setError(null);
            }}
            className="block w-full cursor-pointer rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[13px] text-[var(--projects-text)] file:mr-3 file:cursor-pointer file:border-0 file:bg-[var(--projects-control)] file:px-3 file:py-2 file:text-[12px] file:font-medium file:text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/15"
          />
          <p
            id={helpId}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {file && !valid
              ? "Use a .zip, .tar, .tar.gz, or .tgz archive."
              : file
                ? `${file.name} · ${formatBytes(file.size)}`
                : "A .zip, .tar, .tar.gz, or .tgz archive with index.html at its root."}
          </p>

          {file && valid && file.size > LARGE_ARCHIVE_BYTES ? (
            <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
              This archive is larger than the default 50 MiB limit. The upload
              is only rejected if the instance has not raised it.
            </p>
          ) : null}

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
              disabled={!valid || deploy.isPending}
              className={PRIMARY_ACTION}
            >
              {deploy.isPending ? "Uploading…" : "Deploy"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
