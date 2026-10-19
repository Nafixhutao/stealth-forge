/**
 * Where the project surface lives.
 *
 * Kept in its own module because both the frame (`project-shell`) and the
 * organization home's project tiles need it, and the shell already imports the
 * project mark from the tiles: sharing the path from either side would make
 * those two modules import each other.
 */

export function projectPath(projectId: string, segment = ""): string {
  const base = `/projects/${projectId}`;
  return segment ? `${base}/${segment}` : base;
}
