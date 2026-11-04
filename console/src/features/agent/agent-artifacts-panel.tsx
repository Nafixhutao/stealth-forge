"use client";

import { useMemo, useState } from "react";
import { Download, X } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { apiUrl } from "@/api/client";
import { cn } from "@/lib/utils";

type AgentArtifact = components["schemas"]["AgentArtifact"];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * The document panel: version selector, PDF preview, and DOCX/PDF downloads.
 * On desktop it is the right column; on phones the page renders it full-screen.
 */
export function AgentArtifactsPanel({
  agentId,
  artifacts,
  onClose,
}: {
  agentId: string;
  artifacts: AgentArtifact[];
  onClose?: () => void;
}) {
  const versions = useMemo(() => {
    const grouped = new Map<number, AgentArtifact[]>();
    for (const artifact of artifacts) {
      const list = grouped.get(artifact.version) ?? [];
      list.push(artifact);
      grouped.set(artifact.version, list);
    }
    return [...grouped.entries()].sort(([left], [right]) => right - left);
  }, [artifacts]);

  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [tab, setTab] = useState<"preview" | "source">("preview");

  // Derived rather than synchronised in an effect: an unknown version falls
  // back to the newest one, and a user selection survives new artifacts.
  const activeVersion = versions.some(([value]) => value === selectedVersion)
    ? selectedVersion
    : (versions[0]?.[0] ?? null);

  const current =
    versions.find(([value]) => value === activeVersion)?.[1] ??
    versions[0]?.[1] ??
    [];
  const pdf = current.find((artifact) => artifact.mime_type === "application/pdf");
  const docx = current.find((artifact) =>
    artifact.mime_type.includes("wordprocessingml"),
  );
  const previewUrl = pdf
    ? apiUrl(`/v1/agents/${agentId}/artifacts/${pdf.id}/download?inline=1`)
    : null;
  const downloadUrl = (artifact: AgentArtifact) =>
    apiUrl(`/v1/agents/${agentId}/artifacts/${artifact.id}/download`);

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-[#0D0E0E]">
      <div className="flex h-14 shrink-0 items-center gap-2 border-b border-[#232326] px-3">
        <span className="text-[13px] font-semibold tracking-[-0.01em] text-[#EDEDF0]">
          Artifact
        </span>
        {versions.length > 1 ? (
          <select
            value={activeVersion ?? ""}
            onChange={(event) => setSelectedVersion(Number(event.target.value))}
            aria-label="Artifact version"
            className={cn(
              "rounded-md border border-[#2A2A2E] bg-[#161617] px-2 py-1 text-[12px] text-[#C9C9CF]",
              FOCUS,
            )}
          >
            {versions.map(([value]) => (
              <option key={value} value={value}>
                v{value}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-[12px] text-[#8B8B90]">
            v{current[0]?.version ?? 1}
          </span>
        )}

        <div className="ml-auto flex items-center gap-1">
          {docx ? (
            <a
              href={downloadUrl(docx)}
              download
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium text-[#C9C9CF] transition-colors hover:bg-white/[0.06] hover:text-white",
                FOCUS,
              )}
            >
              <Download size={13} strokeWidth={1.9} aria-hidden="true" />
              DOCX
            </a>
          ) : null}
          {pdf ? (
            <a
              href={downloadUrl(pdf)}
              download
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium text-[#C9C9CF] transition-colors hover:bg-white/[0.06] hover:text-white",
                FOCUS,
              )}
            >
              <Download size={13} strokeWidth={1.9} aria-hidden="true" />
              PDF
            </a>
          ) : null}
          {onClose ? (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close artifact"
              className={cn(
                "inline-flex size-8 items-center justify-center rounded-md text-[#8B8B90] transition-colors hover:bg-white/[0.06] hover:text-white",
                FOCUS,
              )}
            >
              <X size={15} strokeWidth={1.9} aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-1 border-b border-[#232326] px-2 py-1.5">
        {(["preview", "source"] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            className={cn(
              "rounded-md px-2.5 py-1 text-[12px] font-medium capitalize transition-colors",
              tab === value
                ? "bg-white/[0.06] text-[#EDEDF0]"
                : "text-[#8B8B90] hover:text-[#C9C9CF]",
              FOCUS,
            )}
          >
            {value}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {tab === "preview" ? (
          previewUrl ? (
            <iframe
              src={previewUrl}
              title="Artifact preview"
              className="h-full w-full border-0 bg-white"
            />
          ) : (
            <div className="flex h-full items-center justify-center p-6 text-center text-[13px] leading-5 text-[#8B8B90]">
              This version has no PDF preview. Download the DOCX instead.
            </div>
          )
        ) : (
          <div className="flex h-full flex-col gap-2 overflow-y-auto p-3">
            {current.map((artifact) => (
              <a
                key={artifact.id}
                href={downloadUrl(artifact)}
                download
                className={cn(
                  "rounded-lg border border-[#232326] bg-[#161617] p-3 transition-colors hover:border-[#34343A]",
                  FOCUS,
                )}
              >
                <p className="m-0 text-[13px] font-medium text-[#EDEDF0]">
                  {artifact.title}
                </p>
                <p className="m-0 mt-1 text-[11.5px] text-[#8B8B90]">
                  {formatBytes(artifact.size_bytes)} · v{artifact.version} · run{" "}
                  {artifact.run_id.slice(0, 8)}
                </p>
              </a>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
