/**
 * AIDE Debug Logger
 *
 * Provides timing and tracing for hooks and operations.
 * Disabled by default - enable with AIDE_DEBUG=1 environment variable.
 *
 * Usage:
 *   import { Logger } from '../lib/logger';
 *   const log = new Logger('hook-name', cwd);
 *   log.start('operation');
 *   // ... do work
 *   log.end('operation');
 *   log.flush();  // Write all logs to file
 *
 * Enable logging:
 *   AIDE_DEBUG=1 claude
 */

import { existsSync, mkdirSync, appendFileSync, writeFileSync } from 'fs';
import { join } from 'path';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
  timestamp: string;
  level: LogLevel;
  source: string;
  message: string;
  duration?: number;
  data?: unknown;
}

interface TimingEntry {
  start: number;
  label: string;
}

export class Logger {
  private enabled: boolean;
  private source: string;
  private cwd: string;
  private logDir: string;
  private logFile: string;
  private entries: LogEntry[] = [];
  private timings: Map<string, TimingEntry> = new Map();
  private sessionStart: number;

  constructor(source: string, cwd?: string) {
    const debugEnv = process.env.AIDE_DEBUG || '';
    this.enabled = debugEnv === '1' || debugEnv === 'true';
    this.source = source;
    this.cwd = cwd || process.cwd();
    this.logDir = join(this.cwd, '.aide', '_logs');
    this.logFile = join(this.logDir, 'startup.log');
    this.sessionStart = Date.now();

    if (this.enabled) {
      this.ensureLogDir();
    }
  }

  /**
   * Check if logging is enabled
   */
  isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * Ensure log directory exists
   */
  private ensureLogDir(): void {
    if (!existsSync(this.logDir)) {
      try {
        mkdirSync(this.logDir, { recursive: true });
      } catch {
        // Disable logging if we can't create directory
        this.enabled = false;
      }
    }
  }

  /**
   * Format timestamp for logging
   */
  private formatTimestamp(): string {
    return new Date().toISOString();
  }

  /**
   * Format duration in human-readable form
   */
  private formatDuration(ms: number): string {
    if (ms < 1) return '<1ms';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  /**
   * Add a log entry
   */
  private addEntry(level: LogLevel, message: string, duration?: number, data?: unknown): void {
    if (!this.enabled) return;

    this.entries.push({
      timestamp: this.formatTimestamp(),
      level,
      source: this.source,
      message,
      duration,
      data,
    });
  }

  /**
   * Log a debug message
   */
  debug(message: string, data?: unknown): void {
    this.addEntry('debug', message, undefined, data);
  }

  /**
   * Log an info message
   */
  info(message: string, data?: unknown): void {
    this.addEntry('info', message, undefined, data);
  }

  /**
   * Log a warning
   */
  warn(message: string, data?: unknown): void {
    this.addEntry('warn', message, undefined, data);
  }

  /**
   * Log an error
   */
  error(message: string, data?: unknown): void {
    this.addEntry('error', message, undefined, data);
  }

  /**
   * Start timing an operation
   */
  start(label: string): void {
    if (!this.enabled) return;

    this.timings.set(label, {
      start: performance.now(),
      label,
    });
    this.addEntry('debug', `→ ${label}`);
  }

  /**
   * End timing an operation and log duration
   */
  end(label: string, data?: unknown): number {
    if (!this.enabled) return 0;

    const timing = this.timings.get(label);
    if (!timing) {
      this.warn(`end() called for unknown timing: ${label}`);
      return 0;
    }

    const duration = performance.now() - timing.start;
    this.timings.delete(label);
    this.addEntry('info', `← ${label}`, duration, data);
    return duration;
  }

  /**
   * Time an operation (sync or async)
   */
  async time<T>(label: string, fn: () => T | Promise<T>): Promise<T> {
    this.start(label);
    try {
      const result = await fn();
      this.end(label);
      return result;
    } catch (err) {
      this.end(label, { error: String(err) });
      throw err;
    }
  }

  /**
   * Time a synchronous operation
   */
  timeSync<T>(label: string, fn: () => T): T {
    this.start(label);
    try {
      const result = fn();
      this.end(label);
      return result;
    } catch (err) {
      this.end(label, { error: String(err) });
      throw err;
    }
  }

  /**
   * Format entries for file output
   */
  private formatEntries(): string {
    const lines: string[] = [];
    const sessionDuration = Date.now() - this.sessionStart;

    lines.push('');
    lines.push(`${'='.repeat(60)}`);
    lines.push(`[${this.source}] Session started at ${new Date(this.sessionStart).toISOString()}`);
    lines.push(`${'='.repeat(60)}`);

    for (const entry of this.entries) {
      const levelTag = entry.level.toUpperCase().padEnd(5);
      const durationStr = entry.duration !== undefined ? ` (${this.formatDuration(entry.duration)})` : '';
      lines.push(`${entry.timestamp} ${levelTag} ${entry.message}${durationStr}`);

      if (entry.data) {
        const dataStr = JSON.stringify(entry.data, null, 2)
          .split('\n')
          .map(l => `    ${l}`)
          .join('\n');
        lines.push(dataStr);
      }
    }

    lines.push(`${'─'.repeat(60)}`);
    lines.push(`Total: ${this.formatDuration(sessionDuration)}`);
    lines.push('');

    return lines.join('\n');
  }

  /**
   * Flush all log entries to file
   */
  flush(): void {
    if (!this.enabled || this.entries.length === 0) return;

    try {
      this.ensureLogDir();
      appendFileSync(this.logFile, this.formatEntries());
      this.entries = [];
    } catch {
      // Silently fail - logging should not break the hook
    }
  }

  /**
   * Write to a custom log file (relative to .aide/_logs/)
   */
  writeToFile(filename: string, content: string): void {
    if (!this.enabled) return;

    try {
      this.ensureLogDir();
      const filepath = join(this.logDir, filename);
      writeFileSync(filepath, content);
    } catch {
      // Silently fail
    }
  }

  /**
   * Append to a custom log file (relative to .aide/_logs/)
   */
  appendToFile(filename: string, content: string): void {
    if (!this.enabled) return;

    try {
      this.ensureLogDir();
      const filepath = join(this.logDir, filename);
      appendFileSync(filepath, content);
    } catch {
      // Silently fail
    }
  }

  /**
   * Get the log directory path
   */
  getLogDir(): string {
    return this.logDir;
  }

  /**
   * Get the main log file path
   */
  getLogFile(): string {
    return this.logFile;
  }

  /**
   * Create a child logger with a sub-source name
   */
  child(subSource: string): Logger {
    const childLogger = new Logger(`${this.source}:${subSource}`, this.cwd);
    return childLogger;
  }
}

/**
 * Create a singleton logger instance for quick use
 */
let defaultLogger: Logger | null = null;

export function getLogger(source: string, cwd?: string): Logger {
  if (!defaultLogger || defaultLogger['source'] !== source) {
    defaultLogger = new Logger(source, cwd);
  }
  return defaultLogger;
}

/**
 * Quick check if debug logging is enabled
 */
export function isDebugEnabled(): boolean {
  const debugEnv = process.env.AIDE_DEBUG || '';
  return debugEnv === '1' || debugEnv === 'true';
}

// Debug log state - tracks cwd for file-based logging
let debugLogCwd = process.cwd();

/**
 * Set the working directory for debug logging.
 * Call this after parsing stdin to use project-local logs.
 */
export function setDebugCwd(cwd: string): void {
  debugLogCwd = cwd;
}

/**
 * Log a debug message to .aide/_logs/debug.log
 *
 * Usage:
 *   import { debug, setDebugCwd } from '../lib/logger';
 *   debug('hook-name', 'Starting...');
 *   // After parsing stdin:
 *   setDebugCwd(data.cwd);
 *   debug('hook-name', 'Parsed input');
 *
 * Note: Does NOT write to stderr (which triggers Claude Code error reporting).
 * All output goes to the debug.log file only when AIDE_DEBUG=1 is set.
 */
export function debug(source: string, msg: string): void {
  if (!isDebugEnabled()) return;

  const logDir = join(debugLogCwd, '.aide', '_logs');
  try {
    if (!existsSync(logDir)) {
      mkdirSync(logDir, { recursive: true });
    }
    const logFile = join(logDir, 'debug.log');
    const paddedSource = source.padEnd(16);
    const line = `[${new Date().toISOString()}] [${paddedSource}] ${msg}\n`;
    appendFileSync(logFile, line);
  } catch { /* ignore */ }
}
