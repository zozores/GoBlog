#!/bin/bash
set -e
cd "$(dirname "$0")/.."
npm i
npx --no-install node build/generateIcons.mjs
