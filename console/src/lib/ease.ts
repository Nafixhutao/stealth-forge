// Shared motion tokens. Single source for every animation in the app —
// values originated from the beui animated-sidebar (lib/ease) plus the
// gsap curves the pre-motion version used.

export const EASE_OUT: [number, number, number, number] = [0.16, 1, 0.3, 1];
export const EASE_DRAWER: [number, number, number, number] = [0.32, 0.72, 0, 1];
// gsap power2.out — entrance/popover feel
export const POWER2_OUT: [number, number, number, number] = [0.33, 1, 0.68, 1];
// gsap power2.inOut — symmetric ease for the height expand/collapse
export const POWER2_INOUT: [number, number, number, number] = [0.45, 0, 0.55, 1];

export const SPRING_PRESS = { type: "spring", stiffness: 500, damping: 30, mass: 0.6 } as const;
export const SPRING_LAYOUT = { type: "spring", stiffness: 360, damping: 32, mass: 0.6 } as const;

export const PANEL_TRANSITION = { duration: 0.36, ease: EASE_DRAWER } as const;
export const PANEL_CLOSE_TRANSITION = { duration: 0.28, ease: EASE_OUT } as const;
export const REDUCED_TRANSITION = { duration: 0.16, ease: EASE_OUT } as const;
export const SUBMENU_TRANSITION = { duration: 0.22, ease: EASE_OUT } as const;

// The desktop rail has a hard width boundary. Direction-aware tweens avoid
// the spring's last-frame wobble while keeping expansion a little more open.
export const SIDEBAR_COLLAPSE_TRANSITION = { duration: 0.24, ease: EASE_OUT } as const;
export const SIDEBAR_EXPAND_TRANSITION = { duration: 0.3, ease: EASE_DRAWER } as const;

// Row labels crossfade while the rail collapses/expands (beui): they leave
// fast so they are gone before the width clips them, and come back slightly
// delayed so the panel has already opened around them.
export const LABEL_ENTER_TRANSITION = { duration: 0.2, delay: 0.08, ease: EASE_OUT } as const;
export const LABEL_EXIT_TRANSITION = { duration: 0.12, ease: EASE_OUT } as const;
