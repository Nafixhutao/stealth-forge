import { ApplicationShell } from "@/components/application-shell";

/**
 * The first user console page: a blank black surface with the redesigned
 * sidebar. Real user content is built on top of this shell.
 */
export default function UserHomePage() {
  return (
    <ApplicationShell>
      <div className="flex min-h-dvh items-center justify-center bg-[var(--projects-bg)]">
        <p className="text-2xl font-medium text-[var(--projects-text)]">
          hi user
        </p>
      </div>
    </ApplicationShell>
  );
}
