import fs from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import JSZip from 'jszip';

// Packaging starts from reviewed source entries, never from the whole workspace.
// Unknown sibling directories can contain projects, databases or private images.
const rootFiles = [
  'README.md', 'CHANGELOG.md', 'AGENTS.md', 'package.json', 'package-lock.json',
  'next.config.ts', 'tsconfig.json', 'next-env.d.ts',
  'vitest.config.mts', 'vitest.config.ts', 'plugin.json', 'mcp.json',
  '.mcp.json', '.npmrc', '.gitignore', '.env.example',
];
const sourceDirectories = ['src', 'tests', 'scripts', 'docs', 'skills', '.codex-plugin', 'public'];
const excludedNames = new Set(['node_modules', '.next', '.data', '.git', 'artifacts', '.DS_Store']);
const excludedFile = name => (name.startsWith('.env') && name !== '.env.example') || name.endsWith('.tsbuildinfo') || /^canvepp\.sqlite(?:-(?:wal|shm))?$/.test(name);

export async function createPluginArchive(root, { dataDir = process.env.CANVEPP_DATA_DIR } = {}) {
  root = path.resolve(root);
  const privateDirectory = path.resolve(root, dataDir || '.data');
  if (privateDirectory === root) throw new Error('Refusing to package a workspace configured as the data directory.');
  const zip = new JSZip();
  async function add(relative) {
    const file = path.resolve(root, relative);
    // Exclude an explicitly configured data location even if it is nested in src/.
    if (file === privateDirectory || file.startsWith(privateDirectory + path.sep)) return;
    let stat;
    try { stat = await fs.lstat(file); }
    catch (error) { if (error.code === 'ENOENT') return; throw error; }
    if (stat.isSymbolicLink()) throw new Error(`Refusing symlink in plugin sources: ${relative}`);
    if (stat.isDirectory()) {
      const entries = (await fs.readdir(file)).sort();
      // Also detect a store configured by Next's .env.local, which this script
      // intentionally never reads. A data directory is never plugin source.
      if (entries.some(name => /^canvepp\.sqlite(?:-(?:wal|shm))?$/.test(name))) {
        if (sourceDirectories.includes(relative)) throw new Error(`Data directory overlaps plugin sources: ${relative}`);
        return;
      }
      for (const name of entries) {
        if (excludedNames.has(name) || excludedFile(name)) continue;
        await add(path.posix.join(relative, name));
      }
    } else if (stat.isFile()) {
      zip.file(relative, await fs.readFile(file));
    } else {
      throw new Error(`Unsupported entry in plugin sources: ${relative}`);
    }
  }
  for (const file of rootFiles) await add(file);
  for (const directory of sourceDirectories) await add(directory);
  return zip.generateAsync({ type: 'nodebuffer', compression: 'DEFLATE' });
}

async function main() {
  const root = path.resolve(import.meta.dirname, '..');
  const archive = await createPluginArchive(root);
  await fs.mkdir(path.join(root, 'artifacts'), { recursive: true });
  const output = path.join(root, 'artifacts', `canvepp-plugin-${JSON.parse(await fs.readFile(path.join(root, 'package.json'), 'utf8')).version}.zip`);
  await fs.writeFile(output, archive);
  console.log(output);
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) await main();
