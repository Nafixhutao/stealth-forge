// Test stub for the "server-only" import guard. Vitest resolves bare modules
// directly with Node's conditions, so the guard package would throw at import
// time; the alias in vitest.config.ts points "server-only" here for unit tests
// only — production builds keep the real package.
export {};
