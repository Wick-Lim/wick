#!/usr/bin/env node
const https = require('https');
const fs = require('fs');
const os = require('os');
const path = require('path');
const tar = require('tar');

function mapOS() {
  const p = os.platform();
  if (p === 'darwin' || p === 'linux') return p;
  console.error(`Unsupported platform: ${p}`);
  process.exit(1);
}
function mapArch() {
  const a = os.arch();
  if (a === 'x64') return 'amd64';
  if (a === 'arm64') return 'arm64';
  console.error(`Unsupported arch: ${a}`);
  process.exit(1);
}

async function download(url, dest) {
  await new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https.get(url, res => {
      if (res.statusCode !== 200) {
        reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        return;
      }
      res.pipe(file);
      file.on('finish', () => file.close(resolve));
    }).on('error', reject);
  });
}

(async () => {
  try {
    const version = process.env.npm_package_version; // e.g. 0.1.0
    const osName = mapOS();
    const arch = mapArch();
    const filename = `wick_${version}_${osName}_${arch}.tar.gz`;
    const url = `https://github.com/wicklim/wick/releases/download/v${version}/${filename}`;
    const pkgRoot = path.join(__dirname, '..');
    const distDir = path.join(pkgRoot, 'dist');
    const binDir = path.join(pkgRoot, 'bin');
    fs.mkdirSync(distDir, { recursive: true });
    fs.mkdirSync(binDir, { recursive: true });
    const tgz = path.join(distDir, filename);

    console.log(`[wick] downloading ${url}`);
    await download(url, tgz);

    console.log(`[wick] extracting ${filename}`);
    await tar.x({ file: tgz, cwd: distDir });

    const srcBin = path.join(distDir, 'wick');
    let finalSrc = srcBin;
    if (!fs.existsSync(srcBin)) {
      const candidates = [];
      const walk = dir => {
        for (const e of fs.readdirSync(dir)) {
          const p = path.join(dir, e);
          const st = fs.statSync(p);
          if (st.isDirectory()) walk(p); else if (e === 'wick') candidates.push(p);
        }
      };
      walk(distDir);
      if (candidates.length === 0) throw new Error('wick binary not found after extraction');
      finalSrc = candidates[0];
    }
    fs.copyFileSync(finalSrc, path.join(binDir, 'wick'));
    fs.chmodSync(path.join(binDir, 'wick'), 0o755);
    console.log('[wick] install complete');
  } catch (err) {
    console.error('[wick] install failed:', err.message);
    process.exit(1);
  }
})();
