/**
 * Cache invalidation has exactly one entry point: `invalidate()` in
 * `src/api/cache-coherence`. Calling TanStack Query's `invalidateQueries` or
 * `removeQueries` directly anywhere else is what produced two dialects of the
 * same policy — a hardcoded key list in the Admin mutations, and hand-rolled
 * key arithmetic in the Sites mutations.
 *
 * This rule reports those direct calls outside the policy directory. It reads
 * the callee name rather than resolving the receiver, so `foo.invalidateQueries`
 * and `queryClient["invalidateQueries"]` are both caught.
 */

const POLICY_DIRECTORY = "src/api/cache-coherence/";
const FORBIDDEN = new Set(["invalidateQueries", "removeQueries"]);

function calleeName(node) {
  if (!node) return undefined;
  if (node.type === "Identifier") return node.name;
  if (node.type === "MemberExpression" && !node.computed) {
    return node.property.type === "Identifier" ? node.property.name : undefined;
  }
  if (node.type === "MemberExpression" && node.computed) {
    return node.property.type === "Literal" ? String(node.property.value) : undefined;
  }
  return undefined;
}

const rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "Route cache invalidation through the shared policy module instead of calling invalidateQueries or removeQueries directly.",
    },
    schema: [],
    messages: {
      usePolicy:
        "Call invalidate() from @/api/cache-coherence instead of {{method}}() directly. Name what changed, not which keys to refresh.",
    },
  },
  create(context) {
    const filename = context.filename ?? context.getFilename?.() ?? "";
    if (filename.split("\\").join("/").includes(POLICY_DIRECTORY)) {
      return {};
    }
    return {
      CallExpression(node) {
        const name = calleeName(node.callee);
        if (!FORBIDDEN.has(name)) return;
        context.report({
          node: node.callee,
          messageId: "usePolicy",
          data: { method: name },
        });
      },
    };
  },
};

const plugin = {
  meta: { name: "stealth" },
  rules: { "single-cache-invalidation": rule },
};

export default plugin;
