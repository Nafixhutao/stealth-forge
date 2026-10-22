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
 * Heights follow the Appwrite reference scale: one 36px control line at every
 * width, so the toolbar reads as a single row of equal boxes. The explicit
 * `min-h` beats the shared Button's 44px floor, which twMerge cannot drop on
 * its own.
 */
export const PRIMARY_ACTION =
  "!h-9 !min-h-0 shrink-0 !px-[15px] gap-2 !text-[12px] font-semibold bg-[#FD366E] hover:bg-[#FF4D80] rounded-lg text-white focus-visible:ring-[#FD366E]/50 focus-visible:ring-offset-[var(--projects-bg)] sm:!w-auto sm:!px-4 sm:!text-[15px]";

export const SECONDARY_ACTION =
  "h-11 min-h-11 rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[15px] text-[var(--projects-text)] hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:ring-[var(--user-accent)]/50 focus-visible:ring-offset-[var(--projects-bg)] sm:h-9 sm:min-h-9";

/**
 * The destructive control. It is not the product accent: deleting a resource
 * is a different intent from creating one, and it must not borrow the colour
 * the rest of the surface uses for "go".
 */
export const DANGER_ACTION =
  "!h-9 !min-h-0 shrink-0 !px-[15px] gap-2 !text-[12px] font-semibold rounded-lg bg-[var(--projects-danger)] text-white hover:opacity-90 focus-visible:ring-[var(--projects-danger)]/50 focus-visible:ring-offset-[var(--projects-bg)] sm:!w-auto sm:!px-4 sm:!text-[15px]";
