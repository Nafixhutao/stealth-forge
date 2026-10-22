import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UserTopBar } from "@/features/user/user-top-bar";

// The bar's contract is which control does what: the brand mark goes home,
// navigation has its own button beside the account, the magnifier asks the
// sidebar for the command palette, and the avatar opens the account actions.
const openSidebar = vi.fn();
const useCurrentAccount = vi.fn();
const replace = vi.fn();

vi.mock("@/components/application-shell", () => ({
  useApplicationShell: () => ({ openSidebar }),
}));

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => useCurrentAccount(),
}));

vi.mock("@/api/mutations/auth", () => ({
  useLogout: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn() } }));

beforeEach(() => {
  vi.clearAllMocks();
  useCurrentAccount.mockReturnValue({
    data: {
      account: {
        id: "22222222-2222-2222-2222-222222222222",
        email: "nafi@example.com",
        display_name: "Nafi",
        avatar_url: "",
        instance_role: null,
      },
    },
  });
});

describe("UserTopBar", () => {
  it("makes the brand mark the way home, not a menu", () => {
    render(<UserTopBar canOpenAdmin={false} />);

    const home = screen.getByRole("link", { name: "Stealth home" });
    expect(home).toHaveAttribute("href", "/user");
    // The mark itself is decorative: the link carries the name.
    expect(home.querySelector("img")).not.toBeNull();
  });

  it("opens the navigation from the three-line button beside the account", () => {
    render(<UserTopBar canOpenAdmin={false} />);

    const toggle = screen.getByRole("button", { name: "Open navigation" });
    expect(toggle).toHaveAttribute("aria-haspopup", "dialog");

    fireEvent.click(toggle);

    expect(openSidebar).toHaveBeenCalledOnce();
  });

  it("opens on click alone, not on pointer-down", () => {
    render(<UserTopBar canOpenAdmin={false} />);

    const toggle = screen.getByRole("button", { name: "Open navigation" });

    // Opening on pointer-down puts the sheet's backdrop under the finger before
    // the tap ends, and the tap's own click then closes the sheet — the button
    // appears to do nothing on a phone.
    fireEvent.pointerDown(toggle);
    expect(openSidebar).not.toHaveBeenCalled();

    fireEvent.click(toggle);
    expect(openSidebar).toHaveBeenCalledOnce();
  });

  it("keeps the brand mark out of the navigation action", () => {
    render(<UserTopBar canOpenAdmin={false} />);

    fireEvent.click(screen.getByRole("link", { name: "Stealth home" }));

    expect(openSidebar).not.toHaveBeenCalled();
  });

  it("asks the sidebar for the command palette from the magnifier", () => {
    const listener = vi.fn();
    window.addEventListener("open-command-palette", listener);
    render(<UserTopBar canOpenAdmin={false} />);

    fireEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener("open-command-palette", listener);
  });

  it("offers the account actions, including the admin console for an admin", () => {
    render(<UserTopBar canOpenAdmin />);

    fireEvent.pointerDown(
      screen.getByRole("button", { name: "Account menu" }),
      {
        button: 0,
        ctrlKey: false,
        pointerType: "mouse",
      },
    );

    expect(screen.getByText("nafi@example.com")).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "Open admin console" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "Log out" }),
    ).toBeInTheDocument();
  });

  it("hides the admin console from an account that cannot open it", () => {
    render(<UserTopBar canOpenAdmin={false} />);

    fireEvent.pointerDown(
      screen.getByRole("button", { name: "Account menu" }),
      {
        button: 0,
        ctrlKey: false,
        pointerType: "mouse",
      },
    );

    expect(
      screen.queryByRole("menuitem", { name: "Open admin console" }),
    ).toBeNull();
    expect(
      screen.getByRole("menuitem", { name: "Log out" }),
    ).toBeInTheDocument();
  });
});
