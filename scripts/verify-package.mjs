/** Isolated package-boundary fixture; never opens the application's data directory. */
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import JSZip from 'jszip';
import { createPluginArchive } from './package-plugin.mjs';
const root = await fs.mkdtemp(path.join(os.tmpdir(), 'canvepp-package-check-'));
try {
  const files = {
    'README.md': 'Fixture project',
    'package.json': '{"name":"fixture"}',
    '.env.example': 'EXAMPLE_ONLY=',
    '.env.local': 'DUMMY_TEST_VALUE=private-fixture',
    'src/index.ts': 'export const fixture = true;',
    'public/icon.svg': '<svg/>',
    'my-private-library/canvepp.sqlite': 'fixture-private-database',
    'my-private-library/assets/private.png': 'fixture-private-image',
    'src/private-store/canvepp.sqlite': 'fixture-nested-database',
    'src/private-store/assets/private.png': 'fixture-nested-image',
    'src/store-from-next-env/canvepp.sqlite': 'fixture-environment-database',
    'src/store-from-next-env/assets/private.png': 'fixture-environment-image',
    '.data/canvepp.sqlite': 'fixture-default-database',
    'artifacts/report.json': '{"private":true}',
    'unknown-directory/private.txt': 'fixture-unknown-private-data',
  };
  for (const [name, content] of Object.entries(files)) {
    const file = path.join(root, name);
    await fs.mkdir(path.dirname(file), { recursive: true });
    await fs.writeFile(file, content);
  }
  const archive = await JSZip.loadAsync(await createPluginArchive(root, { dataDir: 'src/private-store' }));
  const names = Object.keys(archive.files);
  assert(names.includes('README.md') && names.includes('src/index.ts') && names.includes('public/icon.svg'));
  assert(names.includes('.env.example'));
  assert(!names.some(name => /^(?:my-private-library|unknown-directory|\.data|artifacts)\//.test(name)));
  assert(!names.some(name => name.startsWith('src/private-store/')));
  assert(!names.some(name => name.startsWith('src/store-from-next-env/')));
  assert(!names.includes('.env.local'));
  assert(!names.some(name => name.includes('canvepp.sqlite')));
  await fs.symlink(path.join(root, 'my-private-library/assets/private.png'), path.join(root, 'src/private-link.png'));
  await assert.rejects(createPluginArchive(root, { dataDir: 'src/private-store' }), /symlink/);
  console.log(JSON.stringify({ ok: true, customDataExcluded: true, nestedConfiguredDataExcluded: true, nestedDetectedDataExcluded: true, unknownDirectoryExcluded: true, credentialFileExcluded: true, symlinkRejected: true, files: names.filter(name => !archive.files[name].dir).length }));
} finally {
  await fs.rm(root, { recursive: true, force: true });
}
