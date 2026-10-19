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
 * Run with `npm run test:ui`, which starts the Vite dev server, waits until it
 * actually serves this app, and checks the API on :8080 that the dev server
 * proxies `/api` to. That API needs a local, migrated database. Set
 * EVERROOT_WEB_URL (and EVERROOT_API_URL) to reuse servers you already run.
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
/**
 * Console errors are fatal, and they are how a broken origin or a broken asset
 * reached production last time.
 *
 * Exactly one condition is exempt, and it is an environment condition rather
 * than an application fault: the suggestion service answering 503. This suite
 * runs the API with no HF_TOKEN, so that request is expected to fail here — and
 * only here. It is recorded and printed rather than hidden, and the checks
 * themselves assert what the page does with the failure.
 */
const toleratedErrors = [];
function toleratedConsoleError(text, url) {
  return /status of 503/.test(text) && /\/api\/v1\/recommendations/.test(url);
}

for (const vp of VIEWPORTS) {
  const context = await browser.newContext({
    viewport: { width: vp.width, height: vp.height },
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();
  page.on('console', (message) => {
    if (message.type() !== 'error') return;
    const url = message.location()?.url ?? '';
    if (toleratedConsoleError(message.text(), url)) {
      toleratedErrors.push(`[${vp.name}] ${message.text()} (${url})`);
      return;
    }
    consoleErrors.push(`[${vp.name}] ${message.text()}`);
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
  // The page asks for suggestions before it falls back to the botanical
  // reference, and this suite runs against a live API. A state built by
  // intercepting explain/trees would otherwise be taken over by whatever the
  // provider happens to suggest at that moment — a passing check that was only
  // ever passing because the provider happened to be off. Registered before
  // setup so a test that does want suggestions can override it.
  await page.route('**/api/v1/recommendations', (route) =>
    json(route, {
      status: 'no_suggestions',
      suggestions: [],
      evidence: {
        culturalAssociation: 'not_queryable',
        botanicalIdentity: 'unavailable',
        habitat: 'no_reviewed_record',
        ecologicalRole: 'no_reviewed_record',
        documentedStories: 'no_reviewed_record',
        personalMeaning: 'user_owned',
      },
      promptVersion: 'stub',
      model: 'stub-model',
      disclaimer: 'Stubbed for this check.',
    })
  );
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
    assert.doesNotMatch(note, /no reviewed record for this combination|no record for this selection/);
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
    // An unreadable reference is not an empty one. Reporting it as "0 available"
    // would state a failure as a fact about Everroot's records, which is how the
    // production 503 read as "Everroot has nothing".
    const meta = await text(page, '.cat__toggle-meta');
    assert.match(meta, /unavailable/);
    assert.doesNotMatch(meta, /0 available/);
    await page.locator('.cat__toggle').click();
    await page.locator('#catalogue-panel').waitFor({ state: 'visible' });
    assert.match(await text(page, '.cat__panel'), /temporarily unavailable/);
    assert.equal(await page.locator('.cat__panel button:has-text("Try again")').count(), 1);
    // A broken reference must not break the shortlist.
    assert.ok((await page.locator('.shortlist__list .opt').count()) >= 1);
  },
});

/**
 * The AI-first path: suggestions drawn from the visitor's answers, with no
 * verified cultural record behind them. Two things have to be true at once —
 * the shortlist must be filled by the model rather than by the catalogue, and
 * nothing on the page may let a generated idea read as a reviewed record.
 */
await edgeState('ai-suggestions-are-labelled-as-generated', {
  setup: async (page) => {
    await page.route('**/api/v1/recommendations', (route) =>
      json(route, {
        status: 'ok',
        suggestions: [
          {
            name: 'Neem',
            scientificName: 'Azadirachta indica',
            whyItMatches: 'It suits the courtyard you wrote about, and it asks for little care.',
            botanicalDetail: 'A tree in the mahogany family, native to the Indian subcontinent.',
            growingRequirements: ['Full sun', 'Deep soil'],
            limitations: ['Fast surface roots'],
          },
          {
            name: 'Jamun',
            scientificName: 'Syzygium cumini',
            whyItMatches: 'A long-lived tree for a place that matters to you.',
            botanicalDetail: 'A tree in the myrtle family, native to the Indian subcontinent.',
            growingRequirements: ['Moist soil'],
            limitations: [],
          },
        ],
        evidence: {
          culturalAssociation: 'no_reviewed_match',
          botanicalIdentity: 'unavailable',
          habitat: 'no_reviewed_record',
          ecologicalRole: 'no_reviewed_record',
          documentedStories: 'no_reviewed_record',
          personalMeaning: 'user_owned',
        },
        promptVersion: 'v1.0-structured-suggestions',
        model: 'stub-model',
        disclaimer: 'Machine-written.',
      })
    );
  },
  check: async (page) => {
    assert.equal(await page.locator('.shortlist__list .opt').count(), 2);

    // Every card leads with what it is.
    const tags = page.locator('.shortlist__list .opt .tag');
    assert.match(await tags.first().innerText(), /AI-generated suggestion/);
    assert.equal(await tags.count(), 2);

    // What the visitor wrote is what it answered.
    assert.match(await text(page, '.summary__lede'), /suggested from your answers/);
    assert.doesNotMatch(await text(page, '.summary__lede'), /Rather than guess/);
    assert.match(await text(page, 'body'), /Botanical records only/);

    // The note says where the text came from, in full: written for this
    // visitor's answers, with no reviewed record behind it — and it says so
    // without arguing with the tradition they chose.
    const note = await text(page, '.note');
    assert.match(note, /Chosen for you from your answers/);
    assert.match(note, /not read from a reviewed record/);
    assert.match(note, /no citation behind these/);
    assert.doesNotMatch(note, /never a cultural association/);

    // Opening a card keeps the same promises in place.
    await page.locator('.shortlist__list .opt button:has-text("Explore this tree")').first().click();
    await page.waitForSelector('.shortlist__list .opt__more');
    const card = await page.locator('.shortlist__list .opt').first().innerText();
    assert.match(card, /Source — none/);
    assert.match(card, /Full sun/);
    assert.match(card, /Fast surface roots/);
  },
});

/**
 * A model that finds nothing says so, and the page explains it instead of
 * presenting a silent shortlist. This is the honest empty state: distinct from
 * an outage, and never dressed as one.
 */
await edgeState('ai-suggestions-no-genuine-fit', {
  setup: async (page) => {
    await page.route('**/api/v1/recommendations', (route) =>
      json(route, {
        status: 'no_suggestions',
        suggestions: [],
        evidence: {
          culturalAssociation: 'not_queryable',
          botanicalIdentity: 'unavailable',
          habitat: 'no_reviewed_record',
          ecologicalRole: 'no_reviewed_record',
          documentedStories: 'no_reviewed_record',
          personalMeaning: 'user_owned',
        },
        promptVersion: 'v1.0-structured-suggestions',
        model: 'stub-model',
        disclaimer: 'Machine-written.',
      })
    );
  },
  check: async (page) => {
    // The reference shortlist still fills the page, so a "no fit" is never a blank.
    assert.ok((await page.locator('.shortlist__list .opt').count()) >= 1);
    const note = await text(page, '.note');
    assert.match(note, /did not find a tree it would stand behind/i);
  },
});

/**
 * The model provider breaking is an error, not an empty answer. It is stated
 * where the visitor is looking, the reference shortlist stands in behind it,
 * and no part of the page turns the failure into "Everroot has nothing" or
 * into a claim about missing cultural records.
 */
await edgeState('suggestions-provider-failure-is-shown', {
  setup: async (page) => {
    await page.route('**/api/v1/recommendations', (route) =>
      json(route, { error: 'the model provider failed' }, 502)
    );
  },
  check: async (page) => {
    const noteBody = page.locator('.note__body');
    await noteBody.waitFor({ state: 'visible', timeout: 30000 });
    const note = await noteBody.innerText();
    assert.match(note, /Tree suggestions are unavailable right now\. Please try again\./);

    // The shortlist is still filled by the botanical records, so a provider
    // failure never leaves a blank page.
    assert.ok((await page.locator('.shortlist__list .opt').count()) >= 1);

    const body = await page.locator('body').innerText();
    // Nothing was written by a model here, so nothing is presented as written
    // by one, and nothing is presented as verified either.
    assert.doesNotMatch(body, /AI-generated suggestion/);
    assert.doesNotMatch(body, /Verified against a reviewed source/);
    assert.match(body, /Botanical records only|not Nakshatra recommendations/);
  },
});

/**
 * Ask About a Tree is a separate capability from discovery, and it has to keep
 * working beside it: a question is answered, and the answer is labelled as
 * machine-written rather than as a record.
 */
await edgeState('ask-about-a-tree-still-answers', {
  setup: async (page) => {
    await page.route('**/api/v1/questions', (route) =>
      json(route, {
        status: 'ok',
        interactionId: 'interaction-stub',
        question: 'What family does Jamun belong to?',
        answer: 'Jamun is Syzygium cumini, in the myrtle family, Myrtaceae.',
        promptVersion: 'v1.0-structured-answers',
        model: 'stub-model',
        disclaimer: 'General knowledge only.',
      })
    );
  },
  check: async (page) => {
    const ask = page.locator('input[placeholder*="What family does Jamun"]');
    await ask.waitFor({ state: 'visible' });
    await ask.fill('What family does Jamun belong to?');
    await page.locator('button.btn--primary:has-text("Ask")').click();
    await page.waitForSelector('text=Jamun is Syzygium cumini');

    const body = await page.locator('body').innerText();
    assert.match(body, /Machine-written from general botanical knowledge/);
    // A general-knowledge answer is never presented as a reviewed record.
    assert.doesNotMatch(body, /Verified against a reviewed source/);
  },
});

/* ------------------------------ discovery catalogue: 404, 503 and recovery */

/**
 * A suggestion reply from a model that answered. Held here rather than inline
 * so the catalogue-failure checks can prove the two calls are independent: the
 * reviewed catalogue 503s while this one succeeds.
 */
const SUGGESTIONS_FIXTURE = {
  status: 'ok',
  suggestions: [
    {
      name: 'Neem',
      scientificName: 'Azadirachta indica',
      whyItMatches: 'It suits the courtyard you wrote about, and it asks for little care.',
      botanicalDetail: 'A tree in the mahogany family, native to the Indian subcontinent.',
      growingRequirements: ['Full sun', 'Deep soil'],
      limitations: ['Fast surface roots'],
    },
    {
      name: 'Jamun',
      scientificName: 'Syzygium cumini',
      whyItMatches: 'A long-lived tree for a place that matters to you.',
      botanicalDetail: 'A tree in the myrtle family, native to the Indian subcontinent.',
      growingRequirements: ['Moist soil'],
      limitations: [],
    },
  ],
  evidence: {
    // Nothing was queried for this visitor, so nothing is claimed.
    culturalAssociation: 'not_queryable',
    botanicalIdentity: 'unavailable',
    habitat: 'no_reviewed_record',
    ecologicalRole: 'no_reviewed_record',
    documentedStories: 'no_reviewed_record',
    personalMeaning: 'user_owned',
  },
  promptVersion: 'v1.0-structured-suggestions',
  model: 'stub-model',
  disclaimer: 'Machine-written.',
};

/**
 * The wait and the arrival. While the model is working a bird crosses the
 * garden, and when the answer arrives it comes to rest on the tree that was
 * chosen — the information is what the bird found, not a list that happens to
 * appear beside a spinner.
 *
 * These contexts run with reduced motion, so the scene must go straight to the
 * finished picture: the bird already perched, the tree named for anyone using
 * assistive tech, and the cards actually shown. A visitor who asked for no
 * movement still gets the whole answer.
 */
await edgeState('the-bird-lands-on-the-chosen-tree', {
  setup: async (page) => {
    await page.route('**/api/v1/recommendations', (route) => json(route, SUGGESTIONS_FIXTURE));
  },
  check: async (page) => {
    const scene = page.locator('.shortlist__flight svg');
    await scene.waitFor({ state: 'visible', timeout: 30000 });

    // The scene says which tree it is resting on, so the picture and the
    // shortlist cannot drift apart.
    assert.match(await scene.getAttribute('aria-label'), /resting on Neem/i);
    assert.match(await text(page, '.shortlist__flight'), /here is why it was chosen for you/i);

    // And the arrival really did reveal the answer rather than replace it.
    const cards = page.locator('.shortlist__list .opt');
    assert.equal(await cards.count(), 2);
    assert.match(await cards.first().innerText(), /Neem/);
  },
});

/**
 * The catalogue the discovery flow loads at the tradition step. In production
 * these three calls were landing on the web app itself and returning HTML 404s,
 * so the states below are the ones a visitor actually met.
 */
async function openTraditionStep(page) {
  await page.goto(`${BASE}/discover?intent=explore`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.step__actions-end button', { timeout: 30000 });
  await page.locator('.step__actions-end button').last().click();
}

async function discoveryCatalogue(state, routes) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();
  page.on('pageerror', (error) => consoleErrors.push(`[discovery:${state}] ${error.message}`));
  const calls = [];
  await routes(page, calls);
  await openTraditionStep(page);
  return { context, page, calls };
}

await (async () => {
  const state = 'catalogue-404-then-recovery';
  const fixture = [{ slug: 'nakshatra-tree-association', name: 'Nakshatra tree association' }];
  const { context, page, calls } = await discoveryCatalogue(state, async (p, collected) => {
    // The first attempt fails the way the deployed frontend did: the origin
    // answers with an HTML page instead of the API. The retry succeeds with a
    // stubbed body, so the check does not depend on the database.
    let attempt = 0;
    await p.route('**/api/v1/traditions', (route) => {
      attempt += 1;
      collected.push(route.request().url());
      if (attempt <= 2) {
        return route.fulfill({ status: 404, contentType: 'text/html', body: '<!doctype html><h1>404</h1>' });
      }
      return json(route, fixture);
    });
    await p.route('**/api/v1/nakshatras', (route) => {
      attempt += 1;
      if (attempt <= 2) {
        return route.fulfill({ status: 404, contentType: 'text/html', body: '<!doctype html><h1>404</h1>' });
      }
      return json(route, [{ name: 'Rohini', ordinal: 4 }]);
    });
  });

  await step(`${state}: a 404 names the configuration instead of an empty catalogue`, async () => {
    const notice = page.locator('.notice--warn');
    await notice.waitFor({ state: 'visible', timeout: 20000 });
    const copy = await notice.innerText();
    assert.match(copy, /address configured/i);
    assert.match(copy, /PUBLIC_API_URL/);
    assert.doesNotMatch(copy, /no traditions|nothing on record/i);
    assert.equal(await notice.locator('button:has-text("Try again")').count(), 1);
  });

  await step(`${state}: retrying loads the catalogue again`, async () => {
    await page.locator('.notice--warn button:has-text("Try again")').click();
    const cards = page.locator('.choices .choice');
    await cards.first().waitFor({ state: 'visible', timeout: 20000 });
    assert.ok((await cards.count()) >= 1, 'the retry must render the traditions');
    assert.match(await text(page, '.choices'), /Nakshatra/i);
  });

  await step(`${state}: re-entering the step does not refetch the catalogue`, async () => {
    const before = calls.length;
    // Back to the first question through the progress control, then forward
    // again: the catalogue is already in hand and must not be requested twice.
    await page.locator('.steps__control').first().click();
    await page.waitForSelector('.step__actions-end button', { timeout: 20000 });
    await page.locator('.step__actions-end button').last().click();
    await page.locator('.choices .choice').first().waitFor({ state: 'visible', timeout: 20000 });
    await page.waitForTimeout(300);
    assert.equal(calls.length, before, 'the catalogue request was repeated');
  });

  await page.screenshot({ path: path.join(SHOTS, `${state}.png`), fullPage: true });
  await context.close();
})();

const CATALOGUE_503_BODY = {
  error: 'the reviewed catalogue is unavailable right now; please try again shortly',
};

await (async () => {
  const state = 'catalogue-503-unavailable';
  const { context, page } = await discoveryCatalogue(state, async (p) => {
    await p.route('**/api/v1/traditions', (route) => json(route, CATALOGUE_503_BODY, 503));
    await p.route('**/api/v1/nakshatras', (route) => json(route, CATALOGUE_503_BODY, 503));
    // The suggestion call answers while the reviewed catalogue does not. They
    // are separate services, and only one of them is needed to keep going.
    await p.route('**/api/v1/recommendations', (route) => json(route, SUGGESTIONS_FIXTURE));
  });

  await step(`${state}: an unavailable catalogue is reported as unavailable, not as missing`, async () => {
    const notice = page.locator('.notice--warn');
    await notice.waitFor({ state: 'visible', timeout: 20000 });
    const copy = await notice.innerText();
    assert.match(copy, /503/);
    assert.match(copy, /unavailable/i);
    // This is a database problem, not a misconfigured origin.
    assert.doesNotMatch(copy, /PUBLIC_API_URL/);
    assert.doesNotMatch(copy, /no traditions|nothing on record/i);
    // And it is a warning rather than a dead end.
    assert.match(copy, /continue without choosing a tradition/i);
    assert.equal(await page.locator('.choices .choice').count(), 0);
  });

  await step(`${state}: an optional catalogue does not hold the visitor at the step`, async () => {
    const next = page.locator('.step__actions-end button').last();
    assert.equal(
      await next.isEnabled(),
      true,
      'an unavailable cultural catalogue must not gate the whole flow'
    );
    assert.match(await next.innerText(), /Skip and see my result/);
    await next.click();
    await page.waitForURL(/\/results\//, { timeout: 30000 });
    await page.waitForSelector('.shortlist__list .opt', { timeout: 30000 });
  });

  await step(`${state}: the suggestions arrive without the reviewed records`, async () => {
    assert.equal(await page.locator('.shortlist__list .opt').count(), 2);
    const tags = page.locator('.shortlist__list .opt .tag');
    assert.match(await tags.first().innerText(), /AI-generated suggestion/);
    // Nothing was consulted for this visitor, so the page says what the trees
    // rest on instead of implying a reviewed association either way.
    assert.match(await text(page, '.summary__lede'), /not a reviewed cultural record/);
    assert.match(await text(page, '.pill'), /Botanical records only/);
    const body = await page.locator('body').innerText();
    assert.doesNotMatch(body, /Verified against a reviewed source/);
  });

  await page.screenshot({ path: path.join(SHOTS, `${state}.png`), fullPage: true });
  await context.close();
})();

/**
 * The same outage on the third step of the cultural path. The warning is still
 * shown, and the step is still one the visitor can leave by pressing the
 * button — an optional dataset never strands anyone mid-flow.
 */
await (async () => {
  const state = 'catalogue-503-step-three-navigation';
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'reduce',
  });
  const page = await context.newPage();
  page.on('pageerror', (error) => consoleErrors.push(`[discovery:${state}] ${error.message}`));
  await page.route('**/api/v1/traditions', (route) => json(route, CATALOGUE_503_BODY, 503));
  await page.route('**/api/v1/nakshatras', (route) => json(route, CATALOGUE_503_BODY, 503));
  await page.route('**/api/v1/recommendations', (route) => json(route, SUGGESTIONS_FIXTURE));

  await step(`${state}: step three warns and still moves forward`, async () => {
    await page.goto(`${BASE}/discover?intent=culture`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('.step__actions-end button', { timeout: 30000 });
    await page.locator('.step__actions-end button').last().click();

    // Step two asks which kind of tradition, and one of them leads to step three.
    const choice = page.locator('.choice', { hasText: 'Nakshatra or astrological traditions' });
    await choice.waitFor({ state: 'visible', timeout: 20000 });
    await choice.click();
    await page.locator('.step__actions-end button').last().click();

    await page.waitForSelector('.step__count', { timeout: 20000 });
    assert.match(await text(page, '.step__count'), /step 3 of 3/i);

    const notice = page.locator('.notice--warn');
    await notice.waitFor({ state: 'visible', timeout: 20000 });
    assert.match(await notice.innerText(), /503/);

    const next = page.locator('.step__actions-end button').last();
    assert.equal(await next.isEnabled(), true, 'step three must stay navigable');
    assert.match(await next.innerText(), /Skip and see everything on record/);
    await next.click();

    await page.waitForURL(/\/results\//, { timeout: 30000 });
    await page.waitForSelector('.shortlist__list .opt', { timeout: 30000 });
    assert.equal(await page.locator('.shortlist__list .opt').count(), 2);
  });

  await page.screenshot({ path: path.join(SHOTS, `${state}.png`), fullPage: true });
  await context.close();
})();

/**
 * What the model may add beside its suggestions, and what it may not.
 *
 * A mature size decides whether a tree is even possible in the visitor's space.
 * A cultural note may only restate a verdict the server computed, so the card
 * still says it is not a cultural association. A follow-up question is shown as
 * a question — beside the answer it did not wait for, with nowhere to type a
 * reply, because answering one is what "Change my answers" is for.
 */
await (async () => {
  const state = 'model-optional-extras';
  const NOTE = 'Everroot has no reviewed record for this tradition and Nakshatra.';
  const QUESTION = 'How much direct sun does the spot get through the day?';
  const fixture = {
    ...SUGGESTIONS_FIXTURE,
    evidence: { ...SUGGESTIONS_FIXTURE.evidence, culturalAssociation: 'no_reviewed_match' },
    suggestions: [
      {
        name: 'Neem',
        scientificName: 'Azadirachta indica',
        whyItMatches: 'It suits the courtyard you wrote about, and it asks for little care.',
        botanicalDetail: 'A tree in the mahogany family, native to the Indian subcontinent.',
        matureSize: '15–20 m',
        growingRequirements: ['Full sun', 'Deep soil'],
        limitations: ['Fast surface roots'],
        culturalNote: NOTE,
      },
    ],
    followUpQuestion: QUESTION,
  };

  const { context, page } = await discoveryCatalogue(state, async (p) => {
    await p.route('**/api/v1/traditions', (route) => json(route, CATALOGUE_503_BODY, 503));
    await p.route('**/api/v1/nakshatras', (route) => json(route, CATALOGUE_503_BODY, 503));
    await p.route('**/api/v1/recommendations', (route) => json(route, fixture));
  });

  await step(`${state}: a mature size is shown as a fact that decides the space`, async () => {
    const next = page.locator('.step__actions-end button').last();
    await next.click();
    await page.waitForURL(/\/results\//, { timeout: 30000 });
    await page.waitForSelector('.shortlist__list .opt', { timeout: 30000 });

    const card = page.locator('.shortlist__list .opt').first();
    // The facts live behind "Explore this tree", which is how the shortlist
    // stays a list rather than a wall of text.
    await card.locator('button[aria-expanded]').click();
    await card.locator('.opt__facts').waitFor({ state: 'visible', timeout: 20000 });

    const details = await card.innerText();
    assert.match(details, /Mature size/);
    assert.match(details, /15.?20 m/);
    // Above the care notes, because it rules the tree in or out before care is
    // worth discussing.
    assert.ok(
      details.indexOf('Mature size') < details.indexOf('Growing'),
      'mature size should come before the growing needs'
    );
  });

  await step(`${state}: the model's sentence explains the record, never the tree`, async () => {
    const card = page.locator('.shortlist__list .opt').first();
    assert.match(await card.innerText(), new RegExp(NOTE));
    // The verdict itself is still the server's, and it was not upgraded by a
    // machine writing a sentence about it.
    assert.match(await card.innerText(), /Not a cultural association/);
    assert.match(await text(page, '.pill'), /Botanical records only/);
  });

  await step(`${state}: a question is shown as a question, not as a form`, async () => {
    const ask = page.locator('.summary__ask');
    await ask.waitFor({ state: 'visible', timeout: 20000 });
    assert.match(await ask.innerText(), /One thing that would help/);
    assert.match(await ask.innerText(), new RegExp(QUESTION));
    // It arrived beside the suggestions, which are still on the page.
    assert.equal(await page.locator('.shortlist__list .opt').count(), 1);
    // And there is nowhere to answer it here: it is an offer, not a step.
    assert.equal(await ask.locator('input, textarea, select, button').count(), 0);
  });

  await page.screenshot({ path: path.join(SHOTS, `${state}.png`), fullPage: true });
  await context.close();
})();

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
if (toleratedErrors.length) {
  console.log(
    `\nexpected 503s from the suggestion service (${toleratedErrors.length}) — this suite runs the API with no HF_TOKEN:`
  );
  for (const error of toleratedErrors.slice(0, 5)) console.log(`  ${error}`);
}

if (failures.length || consoleErrors.length) process.exit(1);
console.log('\nall browser checks passed with no console errors');
