#!/usr/bin/env node
const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

const binPath = path.join(__dirname, 'wick');

if (!fs.existsSync(binPath)) {
  console.error('[wick] binary not found. Did postinstall run?');
  console.error('Try: npm rebuild @wicklim/wick');
  process.exit(1);
}

const child = spawn(binPath, process.argv.slice(2), {
  stdio: 'inherit'
});

child.on('exit', (code) => process.exit(code));
child.on('error', (err) => {
  console.error('[wick] failed to launch binary:', err.message);
  process.exit(1);
});
