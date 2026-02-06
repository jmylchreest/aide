/**
 * Memory Capture and Retrieval Test
 *
 * Tests the full flow:
 * 1. Clean database
 * 2. Store memories via CLI (simulating what the hook does)
 * 3. Retrieve via search
 * 4. Update with newer memories
 * 5. Verify newer memories are returned
 */

import { execFileSync } from 'child_process';
import { existsSync, rmSync, mkdirSync } from 'fs';
import { join } from 'path';
import { describe, it, expect, beforeAll, afterAll } from 'vitest';

const PROJECT_ROOT = join(__dirname, '..');
const TEST_DB_DIR = join(PROJECT_ROOT, '.aide-test');
const TEST_DB_PATH = join(TEST_DB_DIR, 'memory', 'store.db');
const AIDE_BINARY = join(PROJECT_ROOT, 'bin', 'aide');

// Helper to run aide CLI with test database
function aide(args: string[]): string {
  const env = {
    ...process.env,
    AIDE_MEMORY_DB: TEST_DB_PATH,
  };
  try {
    return execFileSync(AIDE_BINARY, args, {
      env,
      encoding: 'utf-8',
      timeout: 10000,
    }).trim();
  } catch (err: any) {
    throw new Error(`aide ${args.join(' ')} failed: ${err.stderr || err.message}`);
  }
}

describe('Memory Capture and Retrieval', () => {
  beforeAll(() => {
    // Clean up any existing test database
    if (existsSync(TEST_DB_DIR)) {
      rmSync(TEST_DB_DIR, { recursive: true });
    }

    // Create fresh directories
    mkdirSync(join(TEST_DB_DIR, 'memory'), { recursive: true });

    // Verify aide binary exists
    if (!existsSync(AIDE_BINARY)) {
      throw new Error(`aide binary not found at ${AIDE_BINARY}`);
    }
  });

  afterAll(() => {
    // Clean up test database
    if (existsSync(TEST_DB_DIR)) {
      rmSync(TEST_DB_DIR, { recursive: true });
    }
  });

  it('should start with empty database', () => {
    const result = aide(['memory', 'list', '--format=json']);
    expect(result).toBe('[]');
  });

  it('should store favourite food memory', () => {
    const result = aide([
      'memory', 'add',
      '--category=learning',
      '--tags=preferences,food',
      "User's favourite food is cabbage. They mentioned this as a strong preference."
    ]);

    expect(result).toContain('Added memory:');
  });

  it('should store favourite colour memory', () => {
    const result = aide([
      'memory', 'add',
      '--category=learning',
      '--tags=preferences,colour',
      "User's favourite colour is green. This is their preferred colour for UI elements."
    ]);

    expect(result).toContain('Added memory:');
  });

  it('should have 2 memories after adding', () => {
    const result = aide(['memory', 'list', '--format=json']);
    const memories = JSON.parse(result);

    expect(memories.length).toBe(2);

    const foodMemory = memories.find((m: any) => m.content.includes('cabbage'));
    const colourMemory = memories.find((m: any) => m.content.includes('green'));

    expect(foodMemory).toBeDefined();
    expect(foodMemory.category).toBe('learning');
    expect(foodMemory.tags).toContain('preferences');
    expect(foodMemory.tags).toContain('food');

    expect(colourMemory).toBeDefined();
    expect(colourMemory.category).toBe('learning');
    expect(colourMemory.tags).toContain('preferences');
    expect(colourMemory.tags).toContain('colour');
  });

  it('should retrieve memories by selecting "colour"', () => {
    // Using 'select' for substring matching (simpler than full-text search)
    const result = aide(['memory', 'select', 'colour', '--limit=10']);

    expect(result).toContain('green');
  });

  it('should retrieve memories by selecting "food"', () => {
    const result = aide(['memory', 'select', 'food', '--limit=10']);

    expect(result).toContain('cabbage');
  });

  it('should retrieve both when selecting "favourite"', () => {
    const result = aide(['memory', 'select', 'favourite', '--limit=10']);

    expect(result).toContain('cabbage');
    expect(result).toContain('green');
  });

  it('should add updated food preference', async () => {
    // Wait a moment to ensure different timestamps
    await new Promise(resolve => setTimeout(resolve, 100));

    const result = aide([
      'memory', 'add',
      '--category=learning',
      '--tags=preferences,food',
      "User's favourite food has changed to pizza. They now prefer Italian cuisine."
    ]);

    expect(result).toContain('Added memory:');
  });

  it('should add updated colour preference', async () => {
    await new Promise(resolve => setTimeout(resolve, 100));

    const result = aide([
      'memory', 'add',
      '--category=learning',
      '--tags=preferences,colour',
      "User's favourite colour has changed to blue. They prefer cooler tones now."
    ]);

    expect(result).toContain('Added memory:');
  });

  it('should have 4 memories after updates', () => {
    const result = aide(['memory', 'list', '--format=json']);
    const memories = JSON.parse(result);

    expect(memories.length).toBe(4);
  });

  it('should find both old and new food preferences when selecting', () => {
    const result = aide(['memory', 'select', 'food', '--limit=10']);

    expect(result).toContain('pizza');
    expect(result).toContain('cabbage');
  });

  it('should find both old and new colour preferences when selecting', () => {
    const result = aide(['memory', 'select', 'colour', '--limit=10']);

    expect(result).toContain('blue');
    expect(result).toContain('green');
  });

  it('should filter by food tag', () => {
    const result = aide(['memory', 'list', '--tags=food', '--format=json']);
    const memories = JSON.parse(result);

    expect(memories.length).toBe(2);
    memories.forEach((m: any) => {
      expect(m.content.toLowerCase()).toMatch(/cabbage|pizza/);
    });
  });

  it('should filter by colour tag', () => {
    const result = aide(['memory', 'list', '--tags=colour', '--format=json']);
    const memories = JSON.parse(result);

    expect(memories.length).toBe(2);
    memories.forEach((m: any) => {
      expect(m.content.toLowerCase()).toMatch(/green|blue/);
    });
  });

  it('should filter by category and preferences tag', () => {
    const result = aide(['memory', 'list', '--category=learning', '--tags=preferences', '--format=json']);
    const memories = JSON.parse(result);

    expect(memories.length).toBe(4);
    memories.forEach((m: any) => {
      expect(m.category).toBe('learning');
      expect(m.tags).toContain('preferences');
    });
  });
});
