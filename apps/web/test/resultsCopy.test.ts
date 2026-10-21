/**
 * The copy the results page is not allowed to use any more.
 *
 * One message used to appear whenever a visitor's cultural answers found
 * nothing — "Your answers are outside Everroot's reviewed cultural dataset…"
 * — and it was wrong twice over. It blamed the visitor's choices for an
 * outcome that had nothing to do with them, and it put the storage layer's
 * vocabulary in front of somebody who had only been asked what a tree should
 * mean to them.
 *
 * The page still has to say what it contains. It just says it in the visitor's
 * terms, and a missing cultural match is never dressed as a failure.
 *
 * `test/run-tests.mjs` compiles this file for node, so it reads the route as
 * text. The check is static on purpose: the message was a constant, and the
 * way to stop a constant coming back is to look for it.
 */
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const route = fs.readFileSync(
  path.join(here, '..', 'src', 'routes', 'results', '[sessionId]', '+page.svelte'),
  'utf8'
);

/** The message the visitor used to be shown, in full. */
const REMOVED = [
  'Your answers are outside Everroot’s reviewed cultural dataset',
  'You skipped the Nakshatra step, so there is no cultural record to check',
  'There is no reviewed cultural association on record for this selection yet',
  'Rather than guess, Everroot shows the botanical records it can stand behind',
];

test('the cultural-dataset fallback message is gone', () => {
  for (const fragment of REMOVED) {
    assert.ok(
      !route.includes(fragment),
      `the removed copy is back: "${fragment.slice(0, 60)}…"`
    );
  }
});

test('the storage layer’s vocabulary is never spoken to the visitor', () => {
  // Internal terms are fine in a comment and wrong in a sentence a visitor
  // reads. Every occurrence below has to be inside one.
  const lines = route.split('\n');
  const spoken = lines.filter(
    (line) =>
      /reviewed cultural dataset|reviewed dataset/.test(line) &&
      !/^\s*(\/\/|\/\*|\*)/.test(line)
  );
  assert.deepEqual(spoken, [], `internal vocabulary reached the markup:\n${spoken.join('\n')}`);
});

test('no step blames the visitor for skipping a cultural answer', () => {
  // The nakshatra step is optional and the tradition step is optional. A page
  // that reads their absence as a shortfall is telling a visitor who declined
  // to answer that they did something wrong.
  const lines = route.split('\n').filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line));
  const blame = lines.filter((line) =>
    /because you skipped|you skipped the|outside .{0,30}(dataset|record)/i.test(line)
  );
  assert.deepEqual(blame, [], `the lede still points at the visitor: ${blame.join(' | ')}`);
});

test('the lede describes what the page holds rather than what it lacks', () => {
  // Whatever fills the shortlist, the sentence above it is a description —
  // never a warning. The `--notice` treatment it used to carry is gone with
  // the framing that needed it.
  assert.ok(
    !route.includes('summary__lede--notice'),
    'the lede is no longer styled as a notice'
  );
  assert.ok(
    !/ledeIsNotice/.test(route),
    'the flag that decided when to dress the lede as a warning is gone'
  );
  assert.ok(
    route.includes('The lede is a description, never a warning'),
    'the intent should be stated where the next reader will find it'
  );
});

test('what the page does say stays inside the visitor’s own terms', () => {
  // The replacement copy is checked too, so a rewrite cannot quietly reintroduce
  // the same framing under different words.
  const body = route
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line))
    .join('\n');
  assert.doesNotMatch(body, /catalogue is unavailable|dataset/i);
  assert.ok(
    /from the botanical reference that fits? your answers/.test(body),
    'the shortlist should still be described in one plain sentence'
  );
});

test('a missing cultural record is still reported honestly, just not as a verdict', () => {
  // Nothing here stops the page telling the truth about what backs it. The
  // evidence pill stays: it is a statement of scope, not a complaint.
  assert.ok(route.includes("text: 'Botanical records only'"), 'the scope pill remains');
  assert.ok(
    route.includes("text: 'Reviewed association on record'"),
    'and it still reports a reviewed record when there is one'
  );
  // The model's own answer is still labelled as written rather than sourced.
  assert.ok(
    route.includes('general botany written by the AI model'),
    'a suggestion is still identified as machine-written'
  );
});
