// SPDX-License-Identifier: Apache-2.0
// Optional development check: compile classic JS from rendered HTML snapshots.
// This never executes scripts, makes requests or reads authentication settings.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { Script } from 'node:vm';

const directory = process.argv[2];
if (!directory || !statSync(directory).isDirectory()) {
  throw new Error('Usage: node tools/check-rendered-js.mjs <snapshot-directory>');
}

function decodeAttribute(value) {
  const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: '\u00a0' };
  // Decode once, as HTML attribute parsing does; do not double-decode &amp;quot;.
  return value.replace(/&(#x[0-9a-f]+|#[0-9]+|amp|lt|gt|quot|apos|nbsp);/gi, (match, entity) => {
    if (entity[0] !== '#') return named[entity.toLowerCase()] ?? match;
    const hex = entity[1].toLowerCase() === 'x';
    const point = Number.parseInt(entity.slice(hex ? 2 : 1), hex ? 16 : 10);
    return point > 0 && point <= 0x10ffff ? String.fromCodePoint(point) : '\ufffd';
  });
}

const result = { pages: 0, scripts: 0, handlers: 0, skippedModules: 0, errors: [] };
function check(code, file, kind) {
  try {
    new Script(kind === 'handler' ? `(function(event){${code}\n})` : code, { filename: file });
    result[kind === 'handler' ? 'handlers' : 'scripts']++;
  } catch (error) {
    result.errors.push({ file, kind, error: error.message });
  }
}

for (const file of readdirSync(directory).filter(name => name.endsWith('.html')).sort()) {
  const html = readFileSync(join(directory, file), 'utf8');
  result.pages++;
  for (const match of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script\s*>/gi)) {
    if (/\bsrc\s*=/.test(match[1]) || /application\/(?:ld\+)?json/i.test(match[1]) || !match[2].trim()) continue;
    if (/\btype\s*=\s*["']module["']/i.test(match[1])) {
      result.skippedModules++;
      continue;
    }
    check(match[2], file, 'script');
  }
  // A string that will produce HTML inside a script is not yet a DOM handler.
  const markup = html.replace(/<(script|style)\b[^>]*>[\s\S]*?<\/\1\s*>/gi, '');
  for (const match of markup.matchAll(/\bon(?:click|submit|change|input|keydown|keyup|focus|blur)\s*=\s*("([^"]*)"|'([^']*)')/gi)) {
    check(decodeAttribute(match[2] ?? match[3]), file, 'handler');
  }
}

if (result.pages === 0) throw new Error('No rendered HTML snapshots found');
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (result.errors.length) process.exitCode = 1;
