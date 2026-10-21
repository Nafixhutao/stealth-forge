"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import dynamic from "next/dynamic";
import { FileText, Image as ImageIcon, Loader2, Pencil } from "lucide-react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import { apiUrl } from "@/api/client";
import { useEditSiteDeploymentFile } from "@/api/mutations/sites";
import { useSiteDeploymentFiles } from "@/api/queries/site-detail";
import { languageForPath } from "@/features/project/code-viewer";
import {
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import { Button } from "@/components/ui/button";
import { formatBytes } from "@/lib/format";
import { cn } from "@/lib/utils";

type Deployment = components["schemas"]["SiteDeployment"];

// The editor is a heavy chunk and most visits never open it, so it is loaded on
// demand rather than shipped with the section.
const CodeViewer = dynamic(
  () => import("@/features/project/code-viewer").then((m) => m.CodeViewer),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center text-[12.5px] text-[var(--projects-muted)]">
        Loading editor…
      </div>
    ),
  },
);

/** Extensions the viewer will not try to render as text. */
const BINARY = new Set([
  "png",
  "jpg",
  "jpeg",
  "gif",
  "webp",
  "avif",
  "ico",
  "bmp",
  "woff",
  "woff2",
  "ttf",
  "otf",
  "eot",
  "mp4",
  "webm",
  "mp3",
  "wav",
  "ogg",
  "pdf",
  "zip",
  "gz",
  "br",
  "wasm",
]);

function isBinary(path: string) {
  const extension = path.split(".").pop()?.toLowerCase() ?? "";
  return BINARY.has(extension);
}

/** The public URL that serves one file of one release. */
function releaseFileUrl(siteId: string, deploymentId: string, path: string) {
  const encoded = path.split("/").map(encodeURIComponent).join("/");
  return apiUrl(`/v1/sites/${siteId}/deployments/${deploymentId}/${encoded}`);
}

/**
 * The source of one published release, and — for a caller who may write — the
 * way to publish a change.
 *
 * Saving does not modify this release. It asks the API for a new release built
 * from it with one file replaced, which is the only honest shape for an immutable
 * artifact. The panel says that before the operator types, because each save
 * consumes artifact quota until older releases are deleted.
 */
export function SiteSource({
  projectId,
  siteId,
  deploymentId,
  canEdit = false,
  onPublished,
  className,
}: {
  projectId: string;
  siteId: string;
  deploymentId: string;
  canEdit?: boolean;
  /** Receives the release that a save published, so the caller can follow it. */
  onPublished?: (deployment: Deployment) => void;
  className?: string;
}) {
  const listing = useSiteDeploymentFiles(projectId, siteId, deploymentId, true);
  const files = useMemo(() => listing.data?.files ?? [], [listing.data]);
  const publish = useEditSiteDeploymentFile(projectId);

  // The selection is derived, not synchronised: no effect writes it, and the
  // caller keys this component by release so a different release starts clean.
  const [chosen, setChosen] = useState<string | null>(null);
  const selected = useMemo(() => {
    if (chosen && files.some((file) => file.path === chosen)) return chosen;
    return (
      files.find((file) => file.path === "index.html")?.path ??
      files[0]?.path ??
      null
    );
  }, [chosen, files]);

  const binary = selected ? isBinary(selected) : false;
  const content = useQuery({
    queryKey: ["site-release-file", siteId, deploymentId, selected ?? ""],
    enabled: Boolean(selected) && !binary,
    // A published file never changes for a given release id, so the cache is
    // authoritative until the page is left.
    staleTime: Infinity,
    queryFn: async ({ signal }) => {
      const response = await fetch(
        releaseFileUrl(siteId, deploymentId, selected as string),
        { signal, credentials: "omit" },
      );
      if (!response.ok) {
        throw new Error(`This file answered ${response.status}.`);
      }
      return response.text();
    },
  });

  // Editing is a mode, and the draft only exists while it is on. Leaving the
  // mode drops the draft, so a cancelled edit cannot be saved by accident.
  const [draft, setDraft] = useState<string | null>(null);
  const editing = draft !== null;
  const dirty = editing && content.data !== undefined && draft !== content.data;

  const startEditing = () => {
    if (content.data === undefined) return;
    setDraft(content.data);
  };

  const save = () => {
    if (!selected || draft === null) return;
    publish.mutate(
      { siteId, deploymentId, path: selected, content: draft },
      {
        onSuccess: (result) => {
          setDraft(null);
          toast.success("Published a new release from your edit");
          if (result?.deployment) onPublished?.(result.deployment);
        },
        onError: (cause) =>
          toast.error(
            cause instanceof Error
              ? cause.message
              : "Could not publish the edit",
          ),
      },
    );
  };

  return (
    <div className="flex flex-col gap-2">
      {canEdit && selected && !binary ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-3 py-2">
          {editing ? (
            <>
              <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
                Saving publishes a{" "}
                <strong className="text-[var(--projects-text)]">
                  new release
                </strong>
                ; this one stays as it is. Each save counts against the
                site&apos;s artifact quota.
              </p>
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => setDraft(null)}
                  disabled={publish.isPending}
                  className={cn(SECONDARY_ACTION, "!h-8 !text-[12px]")}
                >
                  Cancel
                </Button>
                <Button
                  type="button"
                  onClick={save}
                  disabled={publish.isPending || !dirty}
                  className={cn(PRIMARY_ACTION, "!h-8 !text-[12px]")}
                >
                  {publish.isPending ? "Publishing…" : "Publish changes"}
                </Button>
              </div>
            </>
          ) : (
            <>
              <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
                Read-only by default. Editing publishes a new release.
              </p>
              <Button
                type="button"
                variant="secondary"
                onClick={startEditing}
                disabled={content.data === undefined}
                className={cn(SECONDARY_ACTION, "!h-8 !text-[12px]")}
              >
                <Pencil
                  className="size-3.5"
                  strokeWidth={2}
                  aria-hidden="true"
                />
                Edit
              </Button>
            </>
          )}
        </div>
      ) : null}

      <div
        className={cn(
          "grid min-h-0 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:grid-cols-[220px_1fr]",
          className,
        )}
      >
        <div className="max-h-[200px] overflow-auto border-b border-[var(--projects-divider)] sm:max-h-[420px] sm:border-b-0 sm:border-r">
          {listing.isPending ? (
            <p className="m-0 p-3 text-[12.5px] text-[var(--projects-muted)]">
              Listing files…
            </p>
          ) : listing.isError ? (
            <p className="m-0 p-3 text-[12.5px] text-[var(--projects-danger)]">
              This release has no published files yet.
            </p>
          ) : (
            <ul className="m-0 flex list-none flex-col p-1.5">
              {files.map((file) => {
                const active = file.path === selected;
                return (
                  <li key={file.path}>
                    <button
                      type="button"
                      onClick={() => {
                        setDraft(null);
                        setChosen(file.path);
                      }}
                      title={file.path}
                      aria-current={active ? "true" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[12.5px] transition-colors",
                        active
                          ? "bg-[var(--projects-control)] text-[var(--projects-text)]"
                          : "text-[var(--projects-muted)] hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
                      )}
                    >
                      {isBinary(file.path) ? (
                        <ImageIcon
                          className="size-3.5 shrink-0"
                          strokeWidth={1.8}
                          aria-hidden="true"
                        />
                      ) : (
                        <FileText
                          className="size-3.5 shrink-0"
                          strokeWidth={1.8}
                          aria-hidden="true"
                        />
                      )}
                      <span className="truncate">{file.path}</span>
                      <span className="ml-auto shrink-0 text-[11px] text-[var(--projects-muted)]">
                        {formatBytes(file.size)}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        <div className="relative h-[300px] overflow-auto sm:h-[420px]">
          {content.isFetching ? (
            <div className="flex h-full items-center justify-center gap-2 text-[12.5px] text-[var(--projects-muted)]">
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Reading…
            </div>
          ) : content.isError ? (
            <p className="m-0 p-4 text-[12.5px] text-[var(--projects-danger)]">
              {content.error instanceof Error
                ? content.error.message
                : "Could not read the file."}
            </p>
          ) : selected && binary ? (
            <p className="m-0 p-4 text-[12.5px] text-[var(--projects-muted)]">
              This file is not text, so it is not shown here.{" "}
              <a
                href={releaseFileUrl(siteId, deploymentId, selected)}
                target="_blank"
                rel="noreferrer"
                className="text-[var(--projects-text)] underline"
              >
                Open it
              </a>
              .
            </p>
          ) : typeof content.data === "string" && selected ? (
            <CodeViewer
              // A new file or a new mode remounts the editor; a keystroke does
              // not, so the caret and undo history survive.
              key={`${selected}-${editing ? "edit" : "read"}`}
              initialValue={editing ? (draft ?? content.data) : content.data}
              language={languageForPath(selected)}
              readOnly={!editing}
              onChange={editing ? setDraft : undefined}
              className="h-full"
            />
          ) : (
            <p className="m-0 p-4 text-[12.5px] text-[var(--projects-muted)]">
              Select a file to read it.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
