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

/**
 * Queues a build from a public Git repository.
 *
 * The API downloads the archive and builds it in the isolated worker, so this
 * returns as soon as the release is queued, not when it is live. The caller
 * watches the release's build status and logs; `activate` is requested so a
 * successful build becomes the live release.
 */
export function useCreateGitDeployment(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      siteId,
      body,
    }: {
      siteId: string;
      body: components["schemas"]["GitSiteDeploymentRequest"];
    }) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/sites/{siteID}/deployments/git",
          {
            params: {
              path: { projectID: projectId as string, siteID: siteId },
            },
            body,
          },
        ),
      ),
    onSuccess: async (_deployment, { siteId }) => {
      await applyCacheChanges(queryClient, [
        { kind: "site-deployment", projectId: projectId as string, siteId },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}

/** Makes a ready release the live one. */
export function useActivateSiteDeployment(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      siteId,
      deploymentId,
    }: {
      siteId: string;
      deploymentId: string;
    }) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/sites/{siteID}/deployments/{deploymentID}/activate",
          {
            params: {
              path: {
                projectID: projectId as string,
                siteID: siteId,
                deploymentID: deploymentId,
              },
            },
          },
        ),
      ),
    onSuccess: async (_result, { siteId, deploymentId }) => {
      await applyCacheChanges(queryClient, [
        // The site row carries the active deployment, so it moves too.
        { kind: "site", projectId: projectId as string, siteId },
        { kind: "site-deployment", projectId: projectId as string, siteId },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.siteDeployment(
          projectId as string,
          siteId,
          deploymentId,
        ),
      });
    },
  });
}

/**
 * Deletes one release. The API refuses to delete the live one, so the caller
 * only offers this for releases that are not active.
 */
export function useDeleteSiteDeployment(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      siteId,
      deploymentId,
    }: {
      siteId: string;
      deploymentId: string;
    }) =>
      unwrap(
        await api.DELETE(
          "/v1/projects/{projectID}/sites/{siteID}/deployments/{deploymentID}",
          {
            params: {
              path: {
                projectID: projectId as string,
                siteID: siteId,
                deploymentID: deploymentId,
              },
            },
          },
        ),
      ),
    onSuccess: async (_result, { siteId }) => {
      await applyCacheChanges(queryClient, [
        { kind: "site-deployment", projectId: projectId as string, siteId },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}

/**
 * Publishes an edited copy of a release.
 *
 * A release is immutable, so this does not modify one: the API copies the source
 * release, rewrites the file, and returns a new deployment. The previous release
 * stays deployable, and each save therefore consumes artifact quota until older
 * releases are deleted — which is why the caller warns before saving.
 */
export function useEditSiteDeploymentFile(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      siteId,
      deploymentId,
      path,
      content,
    }: {
      siteId: string;
      deploymentId: string;
      path: string;
      content: string;
    }) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/sites/{siteID}/deployments/{deploymentID}/files",
          {
            params: {
              path: {
                projectID: projectId as string,
                siteID: siteId,
                deploymentID: deploymentId,
              },
            },
            body: { path, content, activate: true },
          },
        ),
      ),
    onSuccess: async (_result, { siteId, deploymentId }) => {
      await applyCacheChanges(queryClient, [
        // The new release becomes live, so the site row moves with it.
        { kind: "site", projectId: projectId as string, siteId },
        { kind: "site-deployment", projectId: projectId as string, siteId },
      ]);
      // The edited file belongs to a release that no longer exists as such.
      await queryClient.invalidateQueries({
        queryKey: queryKeys.siteDeploymentFiles(
          projectId as string,
          siteId,
          deploymentId,
        ),
      });
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}
