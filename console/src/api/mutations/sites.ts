"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, uploadMultipart } from "@/api/client";
import { applyCacheChanges } from "@/api/cache-coherence";
import { queryKeys } from "@/api/query-keys";
import type { components } from "@/api/generated/schema";

/**
 * The Sites section's writes.
 *
 * A site and a deployment are separate resources: creating a site reserves its
 * name and platform address, and only an upload puts files behind that address.
 * Both writes refresh the project's usage snapshot as well, because the section
 * header counts sites and artifact bytes from it rather than from this list.
 */

/** Creates a site. `framework` and `enabled` are sent explicitly: the API
 * defaults them, but a request that states them reads the same as the response
 * it produces. */
export function useCreateSite(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["CreateSiteRequest"]) =>
      unwrap(
        await api.POST("/v1/projects/{projectID}/sites", {
          params: { path: { projectID: projectId as string } },
          body,
        }),
      ),
    onSuccess: async () => {
      await applyCacheChanges(queryClient, [
        { kind: "site", projectId: projectId as string },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}

/**
 * Uploads one immutable deployment and makes it the active release.
 *
 * The archive is validated by the API before it is stored, so a bad archive
 * fails the request instead of publishing a broken site. Activation is always
 * requested here: "deploy" on this surface means "make these files live", and a
 * stored-but-inactive release has no control yet to activate later.
 */
export function useDeploySite(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ siteId, file }: { siteId: string; file: File }) => {
      const form = new FormData();
      form.append("source", file, file.name);
      form.append("activate", "true");
      return uploadMultipart(
        (body, signal) =>
          api.POST("/v1/projects/{projectID}/sites/{siteID}/deployments", {
            params: {
              path: { projectID: projectId as string, siteID: siteId },
            },
            // The generated schema types the binary part as a string. The
            // browser sends the File inside FormData and writes the multipart
            // boundary itself, which is what the API reads.
            body: body as unknown as components["schemas"]["SiteDeploymentUploadRequest"],
            signal,
          }),
        form,
      );
    },
    onSuccess: async (_deployment, { siteId }) => {
      await applyCacheChanges(queryClient, [
        // Refreshes the site row: artifact bytes and the active deployment.
        { kind: "site", projectId: projectId as string, siteId },
        // Refreshes the deployment history for the site.
        { kind: "site-deployment", projectId: projectId as string, siteId },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}
