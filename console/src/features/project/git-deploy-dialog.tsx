"use client";

import { useId, useState } from "react";
import { toast } from "sonner";
import { GitSiteDeploymentRequestBuild_runtime as BuildRuntime } from "@/api/generated/schema";
import { useCreateGitDeployment } from "@/api/mutations/sites";
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

/** The build runtimes the API accepts. */
const RUNTIMES: { value: BuildRuntime; label: string }[] = [
  { value: BuildRuntime.node_22, label: "Node.js 22" },
  { value: BuildRuntime.python_3_13, label: "Python 3.13" },
  { value: BuildRuntime.go_1_24, label: "Go 1.24" },
];

/**
 * The hosts the API will build from. It accepts only these, over HTTPS, because
 * the worker downloads the archive itself and must not be pointed at an
 * arbitrary URL.
 */
const ALLOWED_HOSTS = ["github.com", "gitlab.com"];

function repositoryError(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    return "Enter a full HTTPS URL, such as https://github.com/owner/repo.";
  }
  if (url.protocol !== "https:") return "The repository URL must use HTTPS.";
  if (!ALLOWED_HOSTS.includes(url.hostname)) {
    return "Only github.com and gitlab.com repositories are supported.";
  }
  const path = url.pathname.replace(/^\/+|\/+$/g, "");
  if (path.split("/").length < 2) {
    return "The URL must name a repository, such as https://github.com/owner/repo.";
  }
  return null;
}

/**
 * Builds a release from a public Git repository.
 *
 * This is the console's half of "connect a repo": point a site at a repository,
 * a branch, and a build command, and the worker clones, builds, and publishes it
 * in an isolated workspace. The API supports public HTTPS repositories only —
 * no OAuth, no private repos, and no push-triggered rebuild — so the dialog says
 * that instead of implying a GitHub integration that does not exist yet.
 */
export function GitDeployDialog({
  projectId,
  siteId,
  siteName,
  open,
  onOpenChange,
}: {
  projectId: string;
  siteId: string;
  siteName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const deploy = useCreateGitDeployment(projectId);
  const [repository, setRepository] = useState("");
  const [ref, setRef] = useState("main");
  const [runtime, setRuntime] = useState<BuildRuntime>(BuildRuntime.node_22);
  const [buildCommand, setBuildCommand] = useState("npm ci && npm run build");
  const [outputDirectory, setOutputDirectory] = useState("dist");
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setError(null);
    }
    onOpenChange(next);
  };

  const repoProblem = repositoryError(repository);
  const valid =
    repository.trim().length > 0 &&
    repoProblem === null &&
    ref.trim().length > 0 &&
    buildCommand.trim().length > 0;

  const submit = async () => {
    if (!valid) return;
    setError(null);
    try {
      await deploy.mutateAsync({
        siteId,
        body: {
          repository: repository.trim(),
          ref: ref.trim(),
          build_runtime: runtime,
          build_command: buildCommand.trim(),
          output_directory: outputDirectory.trim() || ".",
          activate: true,
        },
      });
      handleOpenChange(false);
      toast.success(`Build queued for ${siteName}`);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Could not queue the build.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Deploy from Git</DialogTitle>
          <DialogDescription>
            The worker downloads the repository, runs the build command, and
            publishes the output. Public HTTPS repositories on GitHub or GitLab
            only; private repositories and push-triggered rebuilds are not
            supported yet.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={`${fieldId}-repo`}>Repository</Label>
          <Input
            id={`${fieldId}-repo`}
            value={repository}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            placeholder="https://github.com/owner/repo"
            aria-invalid={Boolean(repoProblem)}
            aria-describedby={`${fieldId}-repo-help`}
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[13px] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setRepository(event.target.value)}
          />
          <p
            id={`${fieldId}-repo-help`}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {repoProblem ??
              "The repository's default archive is downloaded, so the branch must be public."}
          </p>

          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${fieldId}-ref`}>Branch or tag</Label>
              <Input
                id={`${fieldId}-ref`}
                value={ref}
                autoComplete="off"
                spellCheck={false}
                className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[13px] text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
                onChange={(event) => setRef(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${fieldId}-runtime`}>Runtime</Label>
              <select
                id={`${fieldId}-runtime`}
                value={runtime}
                onChange={(event) =>
                  setRuntime(event.target.value as BuildRuntime)
                }
                className="h-9 w-full rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-2.5 text-[13px] text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/15"
              >
                {RUNTIMES.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <Label htmlFor={`${fieldId}-command`} className="mt-1">
            Build command
          </Label>
          <Input
            id={`${fieldId}-command`}
            value={buildCommand}
            autoComplete="off"
            spellCheck={false}
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[13px] text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setBuildCommand(event.target.value)}
          />
          <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
            Runs in a network-isolated workspace. It must leave an index.html in
            the output directory.
          </p>

          <Label htmlFor={`${fieldId}-output`} className="mt-1">
            Output directory
          </Label>
          <Input
            id={`${fieldId}-output`}
            value={outputDirectory}
            autoComplete="off"
            spellCheck={false}
            placeholder="."
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[13px] text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setOutputDirectory(event.target.value)}
          />

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
              {deploy.isPending ? "Queueing…" : "Build and deploy"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
