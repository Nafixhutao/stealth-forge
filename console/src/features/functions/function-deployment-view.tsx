"use client";

import Link from "next/link";
import { useMemo } from "react";
import { useFunctionDeployment } from "@/api/queries";
import { EmptyState } from "@/components/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { createLogSource, LogViewer } from "@/components/log-viewer";
import { PageHeader } from "@/components/page-header";
import { ResourceId } from "@/components/resource-id";
import { StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { formatBytes, formatDate, formatDuration } from "@/lib/format";
import {
  deploymentDurationMs,
  getDeploymentLifecycleStatus,
  isDeploymentInProgress,
} from "@/lib/deployment-state";
import { BackLink } from "@/components/back-link";

export function FunctionDeploymentView({
  organizationId,
  projectId,
  functionId,
  deploymentId,
}: {
  organizationId: string;
  projectId: string;
  functionId: string;
  deploymentId: string;
}) {
  const query = useFunctionDeployment(projectId, functionId, deploymentId);
  const logSource = useMemo(
    () =>
      createLogSource({
        kind: "function-build",
        projectId,
        functionId,
        deploymentId,
      }),
    [deploymentId, functionId, projectId],
  );
  const deployment = query.data?.deployment;
  if (query.error)
    return <ErrorState error={query.error} retry={() => query.refetch()} />;
  if (query.isPending) return <LoadingState rows={5} />;
  if (!deployment)
    return (
      <EmptyState
        title="Deployment not found"
        description="The deployment may have been removed or is outside this function."
      />
    );
  return (
    <>
      <BackLink
        href={`/organizations/${organizationId}/projects/${projectId}/functions/${functionId}`}
        label="Back to function"
      />
      <PageHeader
        eyebrow="Deployment"
        title={`Version ${deployment.version}`}
        description="Immutable deployment metadata and incremental build logs."
        actions={
          <StatusBadge status={getDeploymentLifecycleStatus(deployment)} />
        }
      />
      <div className="mb-5 flex flex-wrap items-center gap-2">
        <ResourceId id={deployment.id} label="Deployment ID" />
        <span className="text-xs text-slate-600">
          Updated {formatDate(deployment.updated_at)}
        </span>
        <span className="text-xs text-slate-600">
          Queued {formatDate(deployment.queued_at)}
        </span>
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-slate-500">Build</p>
            <div className="mt-2">
              <StatusBadge status={deployment.build_status} />
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-slate-500">Source</p>
            <p className="mt-2 text-sm text-white">{deployment.source}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-slate-500">Size</p>
            <p className="mt-2 text-sm text-white">
              {formatBytes(deployment.size_bytes)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-slate-500">Duration</p>
            <p className="mt-2 text-sm text-white">
              {formatDuration(deploymentDurationMs(deployment))}
            </p>
            {deployment.build_started_at ? (
              <p className="mt-1 text-xs text-slate-600">
                Started {formatDate(deployment.build_started_at)}
              </p>
            ) : null}
          </CardContent>
        </Card>
      </div>
      {isDeploymentInProgress(deployment) ? (
        <Card className="mt-5 border-violet-300/20 bg-violet-300/[0.04]">
          <CardContent className="p-5">
            <p className="text-sm font-medium text-violet-100">
              Build in progress
            </p>
            <p className="mt-1 text-xs leading-5 text-violet-200/70">
              This deployment is queued or building. Its status and build logs
              refresh automatically.
            </p>
          </CardContent>
        </Card>
      ) : null}
      {deployment.error_message ||
      getDeploymentLifecycleStatus(deployment) === "failed" ? (
        <Card className="mt-5 border-rose-300/20 bg-rose-400/[0.04]">
          <CardContent className="space-y-3 p-5">
            <div>
              <p className="text-xs uppercase tracking-[0.14em] text-rose-300">
                Deployment failed
              </p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-rose-100">
                {deployment.error_message ??
                  "The build did not complete successfully."}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button asChild variant="outline">
                <a href="#build-logs">View build logs</a>
              </Button>
              <Button asChild variant="ghost">
                <Link
                  href={`/organizations/${organizationId}/projects/${projectId}/functions/${functionId}`}
                >
                  Deploy another archive
                </Link>
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}
      <div id="build-logs" className="mt-5 scroll-mt-6">
        <LogViewer
          key={deploymentId}
          title="Build logs"
          description="Backend sequence cursor; only new lines are requested while following."
          source={logSource}
          polling={isDeploymentInProgress(deployment)}
          emptyMessage="No logs yet. Build output will appear when this deployment starts."
        />
      </div>
    </>
  );
}
