/**
 * The organization plan, as the console names it.
 *
 * One mapping for the whole product: the organization header and the navigation
 * panel both show this chip, and two label tables would drift. The keys are the
 * API's own enum values; anything unknown is shown verbatim rather than
 * silently mapped to a friendlier lie.
 */

const PLAN_LABELS: Record<string, string> = {
  free: "Free",
  pro: "Pro",
  enterprise: "Enterprise",
};

export function planLabel(planKey: string | null | undefined) {
  if (!planKey) return null;
  return PLAN_LABELS[planKey] ?? planKey;
}

/**
 * Real usage against the plan limit, as one sentence. It rides on the plan
 * chip's tooltip so a compact row can keep its shape without dropping the
 * number.
 */
export function planUsageText(used: number, limit: number): string {
  if (limit < 0)
    return `${used} ${used === 1 ? "project" : "projects"}, unlimited`;
  return `${used} of ${limit} ${limit === 1 ? "project" : "projects"} used`;
}
