// Builds dist/aex.exe: a Node single executable application (SEA) running scripts/aex.js.
// Steps: bundle to one CJS file (esbuild), make the SEA blob, copy node.exe, set its icon and
// version info (Windows), inject the blob.
import { execFileSync } from 'node:child_process';
import { copyFileSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { setTimeout } from 'node:timers/promises';
import path from 'node:path';
import { parseArgs } from 'node:util';
import { Resvg } from '@resvg/resvg-js';
import { build } from 'esbuild';
import postject from 'postject';
import rcedit from 'rcedit';
import { run } from '../lib/cli.js';
import { appRoot } from '../lib/paths.js';

const DIST = path.join(appRoot, 'dist');
const BUILD = path.join(DIST, 'build');
const bundleFile = path.join(BUILD, 'aex.cjs');
const blobFile = path.join(BUILD, 'aex.blob');
const seaConfigFile = path.join(BUILD, 'sea-config.json');
const defaultExeFile = path.join(DIST, process.platform === 'win32' ? 'aex.exe' : 'aex');
const logoFile = path.join(appRoot, 'assets', 'logo.svg');
const iconFile = path.join(BUILD, 'aex.ico');

const ICON_SIZES = [16, 24, 32, 48, 64, 128, 256];

// Renders the SVG at each size and packs the PNGs into one .ico (PNG entries, Vista+).
function svgToIco(svg) {
  const pngs = ICON_SIZES.map((size) => new Resvg(svg, { fitTo: { mode: 'width', value: size } }).render().asPng());
  const header = Buffer.alloc(6 + 16 * pngs.length);
  header.writeUInt16LE(1, 2); // type: icon
  header.writeUInt16LE(pngs.length, 4);
  let offset = header.length;
  pngs.forEach((png, i) => {
    const entry = 6 + 16 * i;
    const size = ICON_SIZES[i];
    header.writeUInt8(size % 256, entry); // width, 0 means 256
    header.writeUInt8(size % 256, entry + 1); // height
    header.writeUInt16LE(1, entry + 4); // color planes
    header.writeUInt16LE(32, entry + 6); // bits per pixel
    header.writeUInt32LE(png.length, entry + 8);
    header.writeUInt32LE(offset, entry + 12);
    offset += png.length;
  });
  return Buffer.concat([header, ...pngs]);
}

// Antivirus often holds a freshly written or just-run exe for a moment
// (rcedit: "Unable to commit changes", fs: EPERM/EBUSY), so file steps get a few tries.
async function retry(step, attempts = 4) {
  for (let attempt = 1; ; attempt++) {
    try {
      return await step();
    } catch (e) {
      if (attempt === attempts) throw e;
      await setTimeout(1000 * attempt);
    }
  }
}

run(async (argv) => {
  // --out <file> builds elsewhere, e.g. while dist/aex.exe is running.
  const { values } = parseArgs({ args: argv, options: { out: { type: 'string' } } });
  const exeFile = values.out ? path.resolve(values.out) : defaultExeFile;

  rmSync(BUILD, { recursive: true, force: true });
  mkdirSync(BUILD, { recursive: true });

  await build({
    entryPoints: [path.join(appRoot, 'scripts', 'aex.js')],
    outfile: bundleFile,
    bundle: true,
    platform: 'node',
    format: 'cjs',
    target: `node${process.versions.node.split('.')[0]}`,
    define: {
      'import.meta.url': 'undefined',
      __EMBEDDED_ENV__: JSON.stringify(readFileSync(path.join(appRoot, '.env'), 'utf8')),
    },
    logLevel: 'warning',
  });

  writeFileSync(
    seaConfigFile,
    JSON.stringify({ main: bundleFile, output: blobFile, disableExperimentalSEAWarning: true }, null, 2),
  );
  execFileSync(process.execPath, ['--experimental-sea-config', seaConfigFile], { stdio: 'inherit' });

  // Assembled in the build folder and moved into place last, so a failed build never leaves a
  // plain node.exe behind as the exe.
  const tmpExe = path.join(BUILD, path.basename(exeFile));
  copyFileSync(process.execPath, tmpExe);
  // Before injecting: rcedit rewrites the resource section the blob goes into.
  if (process.platform === 'win32') {
    writeFileSync(iconFile, svgToIco(readFileSync(logoFile)));
    const { version } = JSON.parse(readFileSync(path.join(appRoot, 'package.json'), 'utf8'));
    const resources = {
      icon: iconFile,
      'file-version': version,
      'product-version': version,
      'version-string': {
        FileDescription: 'aex tools',
        ProductName: 'aex tools',
        InternalName: 'aex',
        OriginalFilename: 'aex.exe',
        LegalCopyright: '',
      },
    };
    await retry(() => rcedit(tmpExe, resources));
  }
  await postject.inject(tmpExe, 'NODE_SEA_BLOB', readFileSync(blobFile), {
    sentinelFuse: 'NODE_SEA_FUSE_fce680ab2cc467b6e072b8b5df1996b2',
  });

  mkdirSync(path.dirname(exeFile), { recursive: true });
  try {
    await retry(() => {
      rmSync(exeFile, { force: true });
      renameSync(tmpExe, exeFile);
    });
  } catch (e) {
    if (e.code === 'EPERM' || e.code === 'EBUSY') throw new Error(`${exeFile} is in use; close it and build again`);
    throw e;
  }
  console.log(`Built ${exeFile}`);
});
