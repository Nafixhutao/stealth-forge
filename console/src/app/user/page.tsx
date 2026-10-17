import { ApplicationShell } from "@/components/application-shell";
import { UserHome } from "@/features/user/user-home";

/**
 * The user console home: the signed-in account's organizations, and the
 * projects inside the selected one. All content is read from the API.
 */
export default function UserHomePage() {
  return (
    // The page renders its own mobile top bar (lg:hidden), so the drawer
    // clears it while the desktop rail keeps full height.
    <ApplicationShell hasTopBar desktopHasTopBar={false}>
      <UserHome />
    </ApplicationShell>
  );
}
