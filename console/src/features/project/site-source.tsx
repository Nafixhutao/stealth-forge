"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import dynamic from "next/dynamic";
import { FileText, Image as ImageIcon, Loader2 } from "lucide-react";
import { apiUrl } from "@/api/client";
import { useSiteDeploymentFiles } from "@/api/queries/site-detail";
import { languageForPath } from "@/features/project/code-viewer";
import { formatBytes } from "@/lib/format";
import { cn } from "@/lib/utils";

// The editor is a heavy chunk and most visits never open it, so it is loaded on
// demand rather than shipped with the section.
const CodeViewer = dynamic(
  () => import("@/features/project/code-viewer").then((m) => m.CodeViewer),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center text-[12.5px] text-[var(--projects-muted)]">
        Loading viewer…
      </div>
    ),
  },
);

/** Extensions the viewer will not try to render as text. */
const BINARY = new Set([
  "png", "jpg", "jpeg", "gif", "webp", "avif", "ico", "bmp",
  "woff", "woff2", "ttf", "otf", "eot",
  "mp4", "webm", "mp3", "wav", "ogg",
  "pdf", "zip", "gz", "br", "wasm",
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
 * The source of one published release.
 *
 * Read-only by design: a release is immutable and the API has no endpoint that
 * writes a file into one, so the honest thing to offer is inspection. The file
 * list comes from the deployment's own listing endpoint; the contents come from
 * the release's public preview route, which already serves any file in it — so
 * only the file the operator opens is fetched, not the whole site.
 */
export function SiteSource({
  projectId,
  siteId,
  deploymentId,
  className,
}: {
  projectId: string;
  siteId: string;
  deploymentId: string;
  className?: string;
}) {
  const listing = useSiteDeploymentFiles(projectId, siteId, deploymentId, true);
  const files = useMemo(() => listing.data?.files ?? [], [listing.data]);

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

  return (
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
                    onClick={() => setChosen(file.path)}
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
            value={content.data}
            language={languageForPath(selected)}
            className="h-full"
          />
        ) : (
          <p className="m-0 p-4 text-[12.5px] text-[var(--projects-muted)]">
            Select a file to read it.
          </p>
        )}
      </div>
    </div>
  );
}
