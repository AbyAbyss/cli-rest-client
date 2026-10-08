// Turns the HTML frames written by `go test -tags screenshots` into PNGs.
// Usage: node scripts/screenshots.cjs <html-dir> <png-dir>
// Needs Playwright: npm install -g playwright && npx playwright install chromium
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');

(async () => {
  const [src, dst] = process.argv.slice(2);
  if (!src || !dst) {
    console.error('usage: node scripts/screenshots.cjs <html-dir> <png-dir>');
    process.exit(2);
  }
  fs.mkdirSync(dst, { recursive: true });
  const browser = await chromium.launch(
    process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {});
  const page = await browser.newPage({ deviceScaleFactor: 2 });
  for (const file of fs.readdirSync(src).filter(f => f.endsWith('.html')).sort()) {
    await page.goto('file://' + path.resolve(src, file));
    await page.evaluate(() => document.fonts.ready);
    const out = path.join(dst, file.replace(/\.html$/, '.png'));
    await page.locator('body').screenshot({ path: out });
    console.log('wrote', out);
  }
  await browser.close();
})();
