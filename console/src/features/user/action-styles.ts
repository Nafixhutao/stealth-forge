/**
 * Shared skins for the user console's controls.
 *
 * The shared `Button` is the Inlark, dark-only control: its default variant
 * paints the lavender `acid-lime`, and its secondary variant the graphite and
 * obsidian surfaces. Both read as a different product on this surface, which
 * ships the pink accent and a light theme. These class strings re-skin the
 * same component instead of forking it, so focus, press, and disabled
 * behavior stay in one place. Landing after the variant classes lets twMerge
 * drop the conflicting utilities.
 *
 * Heights follow the Appwrite reference scale: 40px controls on phones (the
 * toolbar row is one uniform 40px line) and 36px from `sm` up, where the
 * denser desktop chrome takes over. The explicit `min-h` beats the shared
 * Button's 44px floor, which twMerge cannot drop on its own.
 */
export const PRIMARY_ACTION =
  "!h-10 !min-h-0 shrink-0 !px-4 gap-2 !text-[13px] font-semibold bg-[#FD366E] hover:bg-[#FF4D80] rounded-[10px] text-white focus-visible:ring-[#FD366E]/50 focus-visible:ring-offset-[var(--projects-bg)] sm:!h-9 sm:!min-h-0 sm:!w-auto sm:px-4 sm:!text-[15px]";

export const SECONDARY_ACTION =
  "h-11 min-h-11 rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[15px] text-[var(--projects-text)] hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:ring-[var(--user-accent)]/50 focus-visible:ring-offset-[var(--projects-bg)] sm:h-9 sm:min-h-9";
