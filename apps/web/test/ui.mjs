/**
 * Browser checks for the results page.
 *
 * These drive the real app in a real browser against the running dev servers, so
 * they test the thing a visitor actually sees. The main path is exercised
 * end-to-end against the real API; the states the dev database cannot produce on
 * demand (one record, no common names, an empty reference, a dead provider) are
 * produced by intercepting the API response, which is the only honest way to
 * reach them without inventing database rows.
 *
 * Run with `npm run test:ui`. Requires the API on :8080 and Vite on :5173.
 */

import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { chromium } from 'playwright';

const BASE = process.env.EVERROOT_WEB_URL ?? 'http://127.0.0.1:5173';
const SHOTS =
  process.env.EVERROOT_SHOTS ??
  '/private/var/folders/tj/njl61_s90wv4m4pkpzbz3zbr0000gn/T/opencode/shots/results-rework';

const VIEWPORTS = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'mobile', width: 390, height: 844 },
  { name: 'small', width: 320, height: 700 },
];

// The single no-match notice. It reframes the absence as a deliberate choice
// ("rather than guess") rather than a failed lookup, and must appear once.
const NO_MATCH_NOTICE = /Rather than guess[\s\S]*not Nakshatra recommendations/;

let passed = 0;
const failures = [];
const notes = [];

const STEP_TIMEOUT_MS = 90_000;

async function step(name, fn) {
  let timer;
  try {
    await Promise.race([
      fn(),
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`step timed out after ${STEP_TIMEOUT_MS}ms`)), STEP_TIMEOUT_MS);
      }),
    ]);
    passed += 1;
    console.log(`  ok  ${name}`);
  } catch (error) {
    failures.push({ name, message: error?.message ?? String(error) });
    console.log(`  FAIL ${name}\n       ${error?.message ?? error}`);
  } finally {
    clearTimeout(timer);
  }
}

/* --------------------------------------------------------------- helpers */

function tree(overrides = {}) {
  return {
    speciesId: 'aaaaaaaa-0000-0000-0000-00000000000a',
    acceptedName: 'Ficus religiosa',
    authorship: 'L.',
    family: 'Moraceae',
    genus: 'Ficus',
    order: 'Rosales',
    rank: 'SPECIES',
    taxonStatus: 'ACCEPTED',
    protologue: 'L. (1753). In: Sp. Pl. 2: 1059.',
    commonNames: [
      { name: 'peepal', language: 'eng' },
      { name: 'sacred fig', language: 'eng' },
    ],
    authority: {
      dataset: 'GBIF Backbone Taxonomy',
      doi: '10.15468/39omei',
      url: 'https://www.gbif.org/species/5361911',
      taxonKey: 5361911,
    },
    sources: [],
    recordType: 'botanical_authority',
    culturalClaim: 'none',
    ...overrides,
  };
}

function explainBody(trees, explanation = { status: 'ok', reason: '', text: '', model: 'stub' }) {
  return {
    status: 'ok',
    task: 'explore_selection',
    evidence: {
      culturalAssociation: 'no_reviewed_match',
      botanicalIdentity: 'authority_verified',
      habitat: 'no_reviewed_record',
      ecologicalRole: 'no_reviewed_record',
      documentedStories: 'no_reviewed_record',
      personalMeaning: 'user_owned',
    },
    trees,
    explanation,
  };
}

async function json(route, body, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    headers: { 'access-control-allow-origin': '*' },
    body: JSON.stringify(body),
  });
}

/** Walk the real discovery flow until the results page is reached. */
async function waitForStep(page) {
  await page.waitForSelector('.step__actions-end button', { timeout: 30000 });
  for (let i = 0; i < 40; i += 1) {
    if (/\/results\//.test(page.url())) return;
    const next = page.locator('.step__actions-end button').last();
    if (await next.isEnabled().catch(() => false)) break;
    // The page is server-rendered, so a click can land before Svelte hydrates.
    // Keep choosing until the step says it can advance.
    const choice = page.locator('.choice').first();
    if (await choice.count()) await choice.click({ force: true }).catch(() => {});
    await page.waitForTimeout(200);
  }
  if (!/\/results\//.test(page.url())) {
    await page.locator('.step__actions-end button').last().click();
    await page.waitForTimeout(300);
  }
}

async function reachResults(page) {
  await page.goto(`${BASE}/discover`, { waitUntil: 'domcontentloaded' });
  for (let attempt = 0; attempt < 12; attempt += 1) {
    if (/\/results\//.test(page.url())) break;
    await waitForStep(page);
  }
  await page.waitForURL(/\/results\//, { timeout: 30000 });
  await settle(page);
  return page.url();
}

/** Wait until the page has stopped fetching, so an assertion cannot race the
 *  explanation call and observe a half-updated shortlist. */
async function settle(page) {
  await page.waitForSelector('.shortlist__list .opt', { timeout: 30000 });
  await page.waitForLoadState('networkidle').catch(() => {});
  await page.waitForTimeout(250);
}

/** The exact record `finish()` writes before navigating, so a viewport can be
 *  checked without re-walking the flow. The API calls are still real. */
const SEED_ID = 'browser-check-record';

async function seedRecord(page) {
  await page.addInitScript((id) => {
    const record = {
      id,
      createdAt: new Date().toISOString(),
      answers: {
        intention: 'culture',
        followUpChoice: null,
        followUpNote: '',
        followUpPicks: [],
        tradition: 'nakshatra-tree-association',
        traditionName: 'Nakshatra tree association',
        region: '',
        nakshatra: 'Rohini',
      },
      discovery: null,
      error: null,
    };
    try {
      window.localStorage.setItem(`everroot.discovery.v1.${id}`, JSON.stringify(record));
      window.localStorage.setItem('everroot.discovery.index.v1', JSON.stringify([id]));
    } catch {
      /* the page will fall back to its not-found state, which the check would catch */
    }
  }, SEED_ID);
}

async function openSeededResult(page) {
  await page.goto(`${BASE}/results/${SEED_ID}`, { waitUntil: 'domcontentloaded' });
  await settle(page);
}

async function overflow(page) {
  return page.evaluate(() => {
    const doc = document.documentElement;
    const offenders = [];
    if (doc.scrollWidth > window.innerWidth + 1) {
      for (const el of document.querySelectorAll('body *')) {
        const box = el.getBoundingClientRect();
        if (box.right > window.innerWidth + 1 || box.left < -1) {
          offenders.push(`${el.tagName.toLowerCase()}.${el.className}`.slice(0, 90));
          if (offenders.length > 4) break;
        }
      }
    }
    return { scrollWidth: doc.scrollWidth, innerWidth: window.innerWidth, offenders };
  });
}

const text = (page, selector) => page.locator(selector).first().innerText();

/* ------------------------------------------------------------------ run */

fs.mkdirSync(SHOTS, { recursive: true });

const browser = await chromium.launch();
const consoleErrors = [];

for (const vp of VIEWPORTS) {
  const context = await browser.newContext({
    viewport: { width: vp.width, height: vp.height },
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(`[${vp.name}] ${message.text()}`);
  });
  page.on('pageerror', (error) => consoleErrors.push(`[${vp.name}] ${error.message}`));

  console.log(`\n== ${vp.name} (${vp.width}x${vp.height})`);

  if (vp.name === 'desktop') {
    await step(`${vp.name}: the flow reaches a result and it is not an error page`, async () => {
      await reachResults(page);
      assert.match(page.url(), /\/results\//);
      assert.equal(await text(page, '#summary-title'), 'Your trees to explore');
    });
  } else {
    await seedRecord(page);
    await step(`${vp.name}: a stored result renders the shortlist`, async () => {
      await openSeededResult(page);
      assert.equal(await text(page, '#summary-title'), 'Your trees to explore');
      assert.match(await text(page, '.summary__lede'), NO_MATCH_NOTICE);
    });
  }

  await step(`${vp.name}: one short heading plus one short notice, not a report`, async () => {
    const body = await page.locator('body').innerText();
    assert.equal(
      body.split('not Nakshatra recommendations').length - 1,
      1,
      'the no-match explanation must appear exactly once'
    );
    assert.match(await text(page, '.summary__lede'), NO_MATCH_NOTICE);
    // The old page repeated the same warning as a heading, an association line
    // and a per-tree caveat. None of those should exist any more.
    assert.equal(await page.locator('.explore__association').count(), 0);
    assert.equal(await page.locator('.explore__distinction').count(), 0);
  });

  await step(`${vp.name}: the shortlist holds at most three options and no more`, async () => {
    const count = await page.locator('.shortlist__list .opt').count();
    assert.ok(count >= 1 && count <= 3, `shortlist had ${count} options`);
    notes.push(`${vp.name}: shortlist showed ${count} option(s)`);
  });

  await step(`${vp.name}: each option shows a name, an honest reason and one primary action`, async () => {
    const cards = page.locator('.shortlist__list .opt');
    for (let i = 0; i < (await cards.count()); i += 1) {
      const card = cards.nth(i);
      const title = await card.locator('.opt__title').innerText();
      assert.ok(title.trim().length > 0, `card ${i} has no name`);
      const why = await card.locator('.opt__why').innerText();
      const words = why.split(/\s+/).filter(Boolean).length;
      assert.ok(words >= 15 && words <= 40, `card ${i} reason was ${words} words`);
      assert.match(why, /botanical reference/);
      // The scientific name and its authorship must not run together.
      const sci = card.locator('.opt__sci');
      if (await sci.count()) {
        assert.match(
          await sci.innerText(),
          /^[A-Z][A-Za-z.-]+ [a-z-]+(\s|$)/,
          `card ${i} scientific name and authorship ran together`
        );
      }
      assert.equal(await card.locator('button[aria-expanded]').count(), 1);
      assert.equal(await card.locator('button[aria-pressed]').count(), 1);
    }
  });

  await step(`${vp.name}: nothing important is hidden until it is asked for`, async () => {
    assert.equal(await page.locator('#catalogue-panel').count(), 0);
    assert.equal(await page.locator('.cat__toggle').getAttribute('aria-expanded'), 'false');
    assert.match(await text(page, '.cat__toggle-meta'), /available/);
    assert.equal(await page.locator('details.panel[open]').count(), 0);
    assert.equal(await page.locator('.opt__more').count(), 0);
  });

  await step(`${vp.name}: no horizontal overflow`, async () => {
    const result = await overflow(page);
    assert.equal(
      result.scrollWidth <= result.innerWidth + 1,
      true,
      `scrollWidth ${result.scrollWidth} vs ${result.innerWidth}; ${result.offenders.join(', ')}`
    );
  });

  await page.screenshot({ path: path.join(SHOTS, `${vp.name}-results.png`), fullPage: true });
  await page.screenshot({ path: path.join(SHOTS, `${vp.name}-first-screen.png`) });

  /* ------------------------------------------------ interactions (once) */

  if (vp.name === 'desktop') {
    await step('desktop: "Explore this tree" expands a why-this-option section', async () => {
      const first = page.locator('.shortlist__list .opt').first();
      // The toggle is addressed by its state, because its label changes when it
      // is open. The save button is the one that carries aria-pressed.
      const toggle = first.locator('button[aria-expanded]');
      assert.equal(await toggle.innerText(), 'Explore this tree');
      assert.equal(await toggle.getAttribute('aria-expanded'), 'false');
      await toggle.click();
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
      assert.equal(await toggle.innerText(), 'Hide details');
      const panel = first.locator('.opt__more');
      await panel.waitFor({ state: 'visible' });
      assert.equal(await panel.locator('.opt__more-title').innerText(), 'WHY THIS OPTION?');
      const labels = await panel.locator('dt').allInnerTexts();
      assert.deepEqual(labels, [
        'Why it was included',
        'What is actually known',
        'Cultural association',
        'Source',
        'Not on record yet',
      ]);
      assert.equal(await panel.locator('.opt__verdict').innerText(), 'Not a cultural association');
      const href = await panel.locator('.opt__source-title a').first().getAttribute('href');
      assert.match(href ?? '', /^https?:\/\//);
      await page.screenshot({
        path: path.join(SHOTS, 'desktop-why-this-option.png'),
        fullPage: true,
      });
      await toggle.click();
      assert.equal(await first.locator('.opt__more').count(), 0);
    });

    await step('desktop: only one option detail is open at a time', async () => {
      const cards = page.locator('.shortlist__list .opt');
      await cards.nth(0).locator('button[aria-expanded]').click();
      await cards.nth(1).locator('button[aria-expanded]').click();
      assert.equal(await cards.nth(0).locator('.opt__more').count(), 0);
      assert.equal(await cards.nth(1).locator('.opt__more').count(), 1);
      await cards.nth(1).locator('button[aria-expanded]').click();
    });

    await step('desktop: the catalogue is collapsed, counts itself and opens on request', async () => {
      const closedHeight = await page.evaluate(() => document.body.scrollHeight);
      await page.locator('.cat__toggle').click();
      const panel = page.locator('#catalogue-panel');
      await panel.waitFor({ state: 'visible' });
      assert.equal(await page.locator('.cat__toggle').getAttribute('aria-expanded'), 'true');
      const rows = await page.locator('.cat__row').count();
      assert.ok(rows >= 1, 'the reference should list its records');
      assert.match(await text(page, '.cat__toggle-meta'), new RegExp(`${rows} available`));
      assert.match(await text(page, '.cat__panel'), /Ordered by family, then accepted name/);
      assert.match(await text(page, '.cat__panel'), /no suggestion that/);
      assert.equal(await page.locator('#tree-search').count(), 1);
      await page.screenshot({ path: path.join(SHOTS, 'desktop-catalogue.png'), fullPage: true });

      await page.locator('.cat__toggle').click();
      await page.waitForTimeout(150);
      assert.equal(await page.locator('#catalogue-panel').count(), 0);
      const reopenedHeight = await page.evaluate(() => document.body.scrollHeight);
      assert.ok(closedHeight <= reopenedHeight, 'collapsing must not grow the page');
    });

    await step('desktop: searching the catalogue filters, and a miss says so plainly', async () => {
      await page.locator('.cat__toggle').click();
      await page.locator('#tree-search').fill('neem');
      await page.waitForTimeout(150);
      const hits = await page.locator('.cat__row').count();
      assert.ok(hits >= 1 && hits < 18, `search returned ${hits} rows`);
      assert.match(await text(page, '.cat__item'), /neem|Azadirachta/i);

      await page.locator('#tree-search').fill('quercus-not-in-the-reference');
      await page.waitForTimeout(150);
      assert.equal(await page.locator('.cat__row').count(), 0);
      assert.match(await text(page, '.cat__panel'), /No record matches/);
      await page.screenshot({ path: path.join(SHOTS, 'desktop-catalogue-empty.png') });

      await page.locator('#tree-search').fill('');
      await page.waitForTimeout(150);
      assert.ok((await page.locator('.cat__row').count()) >= 1);
    });

    await step('desktop: a catalogue row expands by keyboard into the same card', async () => {
      const row = page.locator('.cat__row').first();
      await row.focus();
      assert.equal(await page.evaluate(() => document.activeElement?.classList.contains('cat__row')), true);
      await page.keyboard.press('Enter');
      await page.waitForTimeout(150);
      assert.equal(await row.getAttribute('aria-expanded'), 'true');
      const nested = page.locator('.cat__row-panel .opt');
      assert.equal(await nested.count(), 1);
      assert.equal(await nested.locator('.opt__more-title').innerText(), 'WHY THIS OPTION?');
      await page.screenshot({ path: path.join(SHOTS, 'desktop-catalogue-row.png'), fullPage: true });
      await page.keyboard.press('Enter');
      await page.waitForTimeout(150);
      assert.equal(await page.locator('.cat__row-panel .opt').count(), 0);

      // A species already in the shortlist is marked in the catalogue, not duplicated silently.
      assert.match(await text(page, '.cat__row'), /in your shortlist/);
      await page.locator('.cat__toggle').click();
    });

    await step('desktop: saving a tree persists in this browser only', async () => {
      const card = page.locator('.shortlist__list .opt').first();
      const save = card.locator('button[aria-pressed]');
      assert.equal(await save.innerText(), 'Save this tree');
      await save.click();
      await page.waitForSelector('.shortlist .save-status');
      assert.match(await text(page, '.shortlist .save-status'), /saved in this browser only/);
      assert.match(await text(page, '.shortlist .save-status'), /does not publish or plant/);
      assert.match(await text(page, '.result__saved'), /1 tree is saved in this browser/);
      assert.equal(await save.getAttribute('aria-pressed'), 'true');

      const url = page.url();
      await page.reload();
      await settle(page);
      assert.equal(page.url(), url);
      const reloaded = page.locator('.shortlist__list .opt').first().locator('button[aria-pressed]');
      assert.equal(await reloaded.innerText(), 'Saved on this device');

      await reloaded.click();
      await page.waitForTimeout(150);
      assert.match(await text(page, '.shortlist .save-status'), /was removed from this browser/);
    });

    await step('desktop: the dedication is optional, secondary and stays in the browser', async () => {
      const details = page.locator('#dedication');
      assert.equal(await details.getAttribute('open'), null);
      await details.locator('summary').click();
      await page.waitForTimeout(150);
      await page.locator('#dedication-text').fill('For the ones who kept the garden going.');
      assert.match(await text(page, '#dedication-hint'), /in this browser/);
      assert.match(await text(page, '.dedication__card'), /For the ones who kept the garden going\./);
      // innerText reflects CSS text-transform, so this label arrives uppercased.
      assert.match(await text(page, '.dedication__card'), /private · not published/i);
      await page.locator('#dedication button:has-text("Save in this browser")').click();
      await page.waitForSelector('#dedication .save-status');
      assert.match(await text(page, '#dedication .save-status'), /not been published or shared/);
      assert.match(await text(page, '.dedication__caveat'), /Nothing here creates, reserves or plants a tree/);
      await page.screenshot({ path: path.join(SHOTS, 'desktop-dedication.png'), fullPage: true });

      // The dedication survives a reload, and the #dedication deep link opens it.
      await page.goto(`${page.url().split('#')[0]}#dedication`, { waitUntil: 'domcontentloaded' });
      await settle(page);
      assert.notEqual(await page.locator('#dedication').getAttribute('open'), null);
      assert.equal(
        await page.locator('#dedication-text').inputValue(),
        'For the ones who kept the garden going.'
      );
    });

    await step('desktop: everything secondary can be collapsed and the page gets shorter', async () => {
      const collapsed = await page.evaluate(() => document.body.scrollHeight);
      await page.locator('details.panel').first().locator('summary').click();
      await page.locator('.cat__toggle').click();
      await page.waitForTimeout(200);
      const expanded = await page.evaluate(() => document.body.scrollHeight);
      assert.ok(
        expanded > collapsed,
        `expanding secondary sections should grow the page (${collapsed} -> ${expanded})`
      );
      notes.push(`desktop: page height ${collapsed}px collapsed -> ${expanded}px expanded`);
      await page.locator('.cat__toggle').click();
      await page.waitForTimeout(150);
    });

    await step('desktop: "Change my answers" keeps the answers and can re-run', async () => {
      await page.locator('a:has-text("Change my answers")').click();
      await page.waitForURL(/\/discover/);
      const heading = await page.locator('#step-q').innerText();
      assert.ok(heading.length > 0, 'the flow should reopen where it was left');
      await waitForStep(page);
      for (let i = 0; i < 12 && !/\/results\//.test(page.url()); i += 1) {
        await waitForStep(page);
      }
      await page.waitForURL(/\/results\//, { timeout: 30000 });
      await settle(page);
      notes.push('desktop: a re-run after changing answers produced a fresh result');
    });
  }

  await context.close();
}

/* -------------------------------------------------------- edge states */

async function edgeState(name, { setup, check }) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();
  page.on('pageerror', (error) => consoleErrors.push(`[edge:${name}] ${error.message}`));
  await setup(page);
  await seedRecord(page);
  await step(`edge: ${name}`, async () => {
    await openSeededResult(page);
    await check(page);
  });
  await page.screenshot({ path: path.join(SHOTS, `edge-${name}.png`), fullPage: true });
  await context.close();
}

await edgeState('provider-error-keeps-the-shortlist', {
  setup: async (page) => {
    await page.route('**/api/v1/explain', (route) =>
      json(
        route,
        explainBody(
          [
            tree(),
            tree({
              speciesId: 'b'.repeat(8),
              acceptedName: 'Azadirachta indica',
              genus: 'Azadirachta',
              commonNames: [{ name: 'neem', language: 'eng' }],
            }),
          ],
          {
            status: 'unavailable',
            reason: 'provider_error',
            text: '',
            model: '',
          }
        )
      )
    );
  },
  check: async (page) => {
    assert.equal(await page.locator('.shortlist__list .opt').count(), 2);
    const note = await text(page, '.note');
    assert.match(note, /provider|unavailable|reach/i);
    // A provider failure must never be reported as missing cultural data.
    const body = await page.locator('body').innerText();
    assert.doesNotMatch(note, /no record for this selection/);
    assert.equal(body.split('not Nakshatra recommendations').length - 1, 1);
  },
});

await edgeState('explain-unreachable-falls-back-to-the-reference', {
  setup: async (page) => {
    await page.route('**/api/v1/explain', (route) => route.abort('failed'));
  },
  check: async (page) => {
    const count = await page.locator('.shortlist__list .opt').count();
    assert.ok(count >= 1, 'the shortlist must not depend on the provider');
    assert.match(await text(page, '.note'), /unaffected|could not be reached/i);
  },
});

await edgeState('one-botanical-record', {
  setup: async (page) => {
    const only = tree();
    await page.route('**/api/v1/explain', (route) => json(route, explainBody([only])));
    await page.route('**/api/v1/trees*', (route) =>
      json(route, { status: 'ok', ordering: 'family, then accepted name', query: '', count: 1, trees: [only] })
    );
  },
  check: async (page) => {
    assert.equal(await page.locator('.shortlist__list .opt').count(), 1);
    assert.match(await text(page, '.shortlist'), /all of them/);
    assert.match(await text(page, '.cat__toggle-meta'), /1 available/);
  },
});

await edgeState('missing-common-names', {
  setup: async (page) => {
    const unnamed = tree({ commonNames: [], acceptedName: 'Shorea robusta', genus: 'Shorea' });
    await page.route('**/api/v1/explain', (route) => json(route, explainBody([unnamed])));
    await page.route('**/api/v1/trees*', (route) =>
      json(route, { status: 'ok', ordering: 'family, then accepted name', query: '', count: 1, trees: [unnamed] })
    );
  },
  check: async (page) => {
    const title = page.locator('.shortlist__list .opt__title').first();
    assert.equal(await title.innerText(), 'Shorea robusta');
    assert.match(await title.getAttribute('class'), /opt__title--botanical/);
    // No placeholder that would read as a broken record.
    const card = await page.locator('.shortlist__list .opt').first().innerText();
    assert.doesNotMatch(card, /unknown|not available|n\/a/i);
  },
});

await edgeState('empty-reference', {
  setup: async (page) => {
    await page.route('**/api/v1/trees*', (route) =>
      json(route, { status: 'ok', ordering: 'family, then accepted name', query: '', count: 0, trees: [] })
    );
  },
  check: async (page) => {
    assert.match(await text(page, '.cat__toggle-meta'), /0 available/);
    await page.locator('.cat__toggle').click();
    await page.locator('#catalogue-panel').waitFor({ state: 'visible' });
    assert.match(await text(page, '.cat__panel'), /no records yet/);
    assert.match(await text(page, '.cat__panel'), /will not invent one/);
    // The shortlist still exists, because it comes from the explanation call.
    assert.ok((await page.locator('.shortlist__list .opt').count()) >= 1);
  },
});

await edgeState('reference-unavailable', {
  setup: async (page) => {
    await page.route('**/api/v1/trees*', (route) =>
      json(route, { status: 'ok', error: 'botanical catalogue unavailable' }, 503)
    );
  },
  check: async (page) => {
    assert.match(await text(page, '.cat__toggle-meta'), /0 available/);
    await page.locator('.cat__toggle').click();
    await page.locator('#catalogue-panel').waitFor({ state: 'visible' });
    assert.match(await text(page, '.cat__panel'), /temporarily unavailable/);
    assert.equal(await page.locator('.cat__panel button:has-text("Try again")').count(), 1);
    // A broken reference must not break the shortlist.
    assert.ok((await page.locator('.shortlist__list .opt').count()) >= 1);
  },
});

await browser.close();

/* --------------------------------------------------------------- report */

console.log('\n---- browser checks ----');
console.log(`passed: ${passed}`);
console.log(`failed: ${failures.length}`);
for (const note of notes) console.log(`note: ${note}`);
for (const failure of failures) console.log(`FAILED: ${failure.name}\n  ${failure.message}`);
if (consoleErrors.length) {
  console.log(`\nconsole errors (${consoleErrors.length}):`);
  for (const error of consoleErrors.slice(0, 20)) console.log(`  ${error}`);
}

if (failures.length || consoleErrors.length) process.exit(1);
console.log('\nall browser checks passed with no console errors');
