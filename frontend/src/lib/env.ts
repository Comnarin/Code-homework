/** Internal address of the Go API. Used only by server code (Server Components, next.config). */
export const API_URL = process.env.API_URL ?? "http://localhost:8080";

/** Shows the test-only controls (reset the Red set limit). Must match the API's flag. */
export const TEST_TOOLS_ENABLED = process.env.ENABLE_TEST_TOOLS === "true";
