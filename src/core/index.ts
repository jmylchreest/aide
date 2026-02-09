/**
 * Core module — platform-agnostic aide logic.
 *
 * This module contains shared business logic used by both
 * Claude Code hooks (src/hooks/) and the OpenCode plugin (src/opencode/).
 */

export * from "./types.js";
export * from "./aide-client.js";
export * from "./session-init.js";
export * from "./skill-matcher.js";
export * from "./tool-tracking.js";
export * from "./persistence-logic.js";
export * from "./session-summary-logic.js";
export * from "./pre-compact-logic.js";
export * from "./cleanup.js";
