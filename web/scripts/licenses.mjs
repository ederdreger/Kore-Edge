import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
const root = resolve(import.meta.dirname, '..');
const output = resolve(root, '../internal/webui/dist/licenses');
await mkdir(output, { recursive: true });
const packages = ['react', 'react-dom', 'lucide-react'];
const notices = [];
for (const name of packages) {
  const pkg = JSON.parse(await readFile(resolve(root, 'node_modules', name, 'package.json'), 'utf8'));
  const license = await readFile(resolve(root, 'node_modules', name, 'LICENSE'), 'utf8');
  await writeFile(resolve(output, `${name}.txt`), license);
  notices.push(`${name} ${pkg.version} — ${pkg.license}\n${pkg.homepage ?? ''}`);
}
await writeFile(resolve(output, 'NOTICE.txt'), `Kore Edge dashboard runtime dependencies\n\n${notices.join('\n\n')}\n`);
