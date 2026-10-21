/**
 * What the waiting-scene has to keep true, checked against the component
 * itself.
 *
 * The bird and the sun are drawn in one inline SVG, so there is no rendered
 * frame to screenshot here — but the geometry that decides whether anything is
 * clipped is all in the file, and it is the kind of thing that drifts silently:
 * a keyframe nudged two hundred units to the right leaves the animation
 * running against an empty box with nothing to say so.
 *
 * `test/run-tests.mjs` compiles this file for node, so it reads the component
 * as text. Every assertion is against a value that is also re-stated below, and
 * the shapes the extents were derived from are pinned too, so changing the
 * bird's drawing fails the test rather than quietly invalidating it.
 */
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const component = fs.readFileSync(
  path.join(here, '..', 'src', 'lib', 'components', 'BirdScene.svelte'),
  'utf8'
);

/* -------------------------------------------------------------- the drawing */

const VIEWBOX = { width: 800, height: 240 };

/**
 * How far the bird's drawing reaches from the point its transform moves.
 *
 * Derived from the shapes in the component, which are asserted below so a
 * change to the bird fails here rather than passing against stale numbers:
 *
 *   tail   `M-16 -1 L-28 -6 L-27 4 Z`  -> x -28..-16, y -6..4
 *   body   ellipse rx 14 ry 7           -> x -14..14,  y -7..7
 *   head   circle  r 6 at (12,-5)       -> x 6..18,    y -11..1
 *   beak   `M17 -5 L25 -3 L17 -1 Z`    -> x 17..25,   y -5..-1
 *   wing   `M-2 -2 Q -12 -22 ...`      -> x -12..12,  y -22..-2
 *
 * A quadratic Bézier never leaves the hull of its control points, so the
 * wing's -22 is a real bound on the drawing and not an approximation of one.
 */
const BIRD = { left: 28, right: 25, top: 22, bottom: 7 };

/** Half-extents about the bird's centre, which is where the rotation is about. */
const HALF = { x: (BIRD.left + BIRD.right) / 2, y: (BIRD.top + BIRD.bottom) / 2 };

/** The shapes above, so the extents cannot go stale without this test saying so. */
const BIRD_SHAPES = [
  'M-16 -1 L-28 -6 L-27 4 Z', // tail: the leftmost point of the bird
  'M-2 -2 Q -12 -22 6 -16 Q 12 -12 2 -3 Z', // near wing: the highest point
  'M17 -5 L25 -3 L17 -1 Z', // beak: the rightmost point
  '<ellipse cx="0" cy="0" rx="14" ry="7"', // body: the lowest point
];

test('the extents above still describe the bird that is drawn', () => {
  for (const shape of BIRD_SHAPES) {
    assert.ok(
      component.includes(shape),
      `the bird's drawing changed (${shape} is gone); re-derive BIRD before trusting these bounds`
    );
  }
});

/* --------------------------------------------------------------- the flight */

interface Frame {
  x: number;
  y: number;
  degrees: number;
}

/** Every transform in one `@keyframes` block, in the order the CSS declares them. */
function framesOf(name: string): Frame[] {
  const start = component.indexOf(`@keyframes ${name}`);
  assert.notEqual(start, -1, `@keyframes ${name} is missing`);

  const open = component.indexOf('{', start);
  let depth = 0;
  let end = open;
  for (; end < component.length; end += 1) {
    if (component[end] === '{') depth += 1;
    if (component[end] === '}') depth -= 1;
    if (depth === 0) break;
  }
  const body = component.slice(open + 1, end);

  const frames: Frame[] = [];
  const transform = /translate\(\s*(-?[\d.]+)px\s*,\s*(-?[\d.]+)px\s*\)\s*rotate\(\s*(-?[\d.]+)deg\s*\)/g;
  for (const match of body.matchAll(transform)) {
    frames.push({ x: Number(match[1]), y: Number(match[2]), degrees: Number(match[3]) });
  }
  assert.ok(frames.length >= 2, `@keyframes ${name} has no transforms to check`);
  return frames;
}

/**
 * Whether the whole bird is on the drawing at this frame.
 *
 * The bird turns as it goes, and a turn swings its corners out past the
 * rectangle its extents describe, so the reach is widened by the rotation
 * before it is compared against the drawing.
 */
function frameIsInside({ x, y, degrees }: Frame): boolean {
  const radians = (Math.abs(degrees) * Math.PI) / 180;
  const reachX = HALF.x * Math.cos(radians) + HALF.y * Math.sin(radians);
  const reachY = HALF.x * Math.sin(radians) + HALF.y * Math.cos(radians);
  return (
    x - reachX >= 0 &&
    x + reachX <= VIEWBOX.width &&
    y - reachY >= 0 &&
    y + reachY <= VIEWBOX.height
  );
}

test('the bird is inside the drawing at every frame of the crossing', () => {
  const frames = framesOf('fly-across');
  const outside = frames.filter((frame) => !frameIsInside(frame));
  assert.deepEqual(
    outside,
    [],
    `frames leave the 800x240 drawing, so the bird is clipped or invisible: ${JSON.stringify(outside)}`
  );
});

test('the bird is inside the drawing at every frame of the landing', () => {
  const frames = framesOf('land');
  const outside = frames.filter((frame) => !frameIsInside(frame));
  assert.deepEqual(
    outside,
    [],
    `frames leave the 800x240 drawing: ${JSON.stringify(outside)}`
  );
});

test('the crossing begins and ends on the drawing, so the loop never pauses on nothing', () => {
  const frames = framesOf('fly-across');
  const [first, last] = [frames[0], frames[frames.length - 1]];
  // The path used to run from -90 to 890, which is outside the drawing at both
  // ends: the animation appeared to stall at each end of its 7.5s loop.
  assert.ok(first.x > 0, `the first frame starts off the left edge at x=${first.x}`);
  assert.ok(
    last.x + HALF.x + HALF.y < VIEWBOX.width,
    `the last frame runs off the right edge at x=${last.x}`
  );
  assert.ok(last.x > first.x, 'the bird should still cross from left to right');
});

test('the bird lands on the chosen tree and the landed state agrees with it', () => {
  const perch = framesOf('land').pop();
  assert.ok(perch);
  assert.equal(perch.x, 500, 'the perch is the chosen tree at x=500');
  assert.equal(perch.y, 84, 'the perch is just above the chosen tree canopy');

  const landed = component.match(
    /\.scene\[data-phase='landed'\] \.bird \{[^}]*translate\(\s*(-?[\d.]+)px\s*,\s*(-?[\d.]+)px\s*\)/
  );
  assert.ok(landed, 'the landed state has no transform');
  assert.equal(landed[1], String(perch.x), 'the landed bird and the landing keyframe disagree');
  assert.equal(landed[2], String(perch.y), 'the landed bird and the landing keyframe disagree');
});

/* --------------------------------------------------------------- the scene */

test('nothing is cropped out of the drawing at any width', () => {
  assert.ok(
    component.includes('preserveAspectRatio="xMidYMax meet"'),
    'the drawing must be fitted, not covered: `slice` scaled it up and cut a different piece away at every width'
  );
  assert.ok(
    !component.includes('preserveAspectRatio') || !/"[^"]*slice[^"]*"/.test(component),
    'no `slice` remains — it is what cropped the sun and the whole flight path off the top at desktop widths'
  );
  // A height matched to the drawing's own ratio is what makes `meet` land on an
  // exact fit instead of letterboxing. The floor only opens the box out on the
  // narrowest screens, where the ratio would otherwise crowd the caption.
  assert.ok(component.includes('aspect-ratio: 800 / 240'), 'the height follows the drawing');
  assert.ok(component.includes('min-height: 150px'), 'a floor keeps the caption readable on a phone');
  assert.ok(
    !component.includes('clamp(150px, 26vw, 230px)'),
    'a fixed height is what forced the crop in the first place'
  );
});

test('the rotation is about the bird rather than about the middle of the scene', () => {
  assert.ok(
    component.includes('transform-box: fill-box'),
    'without this, a turn swings the bird around the centre of the whole drawing and out of frame'
  );
});

test('the scene has exactly one sun, and landing does not add another', () => {
  const suns = component.match(/<circle[^>]*#e8c98f/g) ?? [];
  assert.equal(suns.length, 1, `expected one sun, found ${suns.length}: ${suns.join(' | ')}`);

  // The landing emphasis used to be a warm filled disc fading in behind the
  // bird's perch — a second sun beside the one already in the sky. It is an
  // outline now, which reads as "this one" rather than as a second light.
  const marker = component.match(/<circle class="chosen-ring"[^>]*\/>/);
  assert.ok(marker, 'the chosen tree has no marker');
  assert.match(marker[0], /fill="none"/, 'the landing marker must not be a filled disc');
  assert.match(marker[0], /stroke=/, 'the landing marker is drawn as an outline');
  assert.ok(
    !component.includes('chosenGlow'),
    'the gradient behind the old glow is gone with it'
  );
  assert.ok(
    !/fill="url\(#chosenGlow\)"/.test(component),
    'nothing in the scene may be filled like a second sun'
  );
});

test('the landing marker rings the whole tree rather than cutting through it', () => {
  const marker = component.match(/<circle class="chosen-ring"[^>]*r="(\d+)"[^>]*\/>/);
  assert.ok(marker, 'the chosen ring has no radius');
  const radius = Number(marker[1]);
  // The canopy's widest foliage reaches 530 + 28 = 558 and its highest point
  // 112 - 22 = 90, both measured from the tree's centre at (500, 128), so a
  // ring inside those bounds is a line drawn across the leaves.
  assert.ok(radius > 58, `a ring of r=${radius} would cut through the canopy`);

  // And it has to be drawn after the canopy, or the marker is simply hidden
  // behind the foliage it is meant to mark.
  const canopy = component.indexOf('cx="530" cy="146" r="28"');
  const ring = component.indexOf('class="chosen-ring"');
  assert.ok(canopy !== -1 && ring > canopy, 'the marker is drawn behind the canopy');
});

test('the marker and the sun are not the same colour-and-shape combination', () => {
  const sun = component.match(/<circle cx="686" cy="50" r="26"[^>]*\/>/);
  assert.ok(sun, 'the sun is gone');
  // A pale filled disc of a given size is what a sun is. Anything presented as
  // a selection marker has to differ from that on fill, or a visitor has to
  // work out which of two warm circles the bird flew in from.
  assert.match(sun[0], /fill="#e8c98f"/);
  const marker = component.match(/<circle class="chosen-ring"[^>]*\/>/);
  assert.ok(marker);
  assert.doesNotMatch(marker[0], /fill="#e8c98f"/);
});
