<!--
  The moment between asking and being answered.

  While the model works, a bird crosses a garden. When the answer arrives it
  comes to rest on one tree — the visitor's tree — and only then does the page
  show what was found. The scene is drawn rather than filmed: it is one small
  SVG, it costs no request, and it can be recoloured by the same variables the
  rest of the page uses.

  Everything here is decoration over a real state. The suggestion call is not
  slowed down for it, and a visitor who has asked for reduced motion gets the
  finished picture with no movement at all.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    /**
     * flying  — the model has not answered yet, so the bird is crossing.
     * landing — an answer arrived and the bird is coming to rest on it.
     * landed  — the bird is perched, and the information may now be read.
     */
    phase?: 'flying' | 'landing' | 'landed';
    /** The name of the tree the bird settles on, read by assistive tech. */
    treeName?: string;
    children?: Snippet;
  }

  let { phase = 'flying', treeName = '', children }: Props = $props();

  // Where the bird comes to rest, and which tree it is: the same tree the
  // highlight rings, so the picture and the shortlist agree.
  const PERCH = { x: 500, y: 84 };
</script>

<div class="scene" class:scene--landed={phase === 'landed'} data-phase={phase}>
  <svg
    viewBox="0 0 800 240"
    role="img"
    aria-label={phase === 'landed' && treeName
      ? `A garden scene, with a bird resting on ${treeName}.`
      : 'A garden scene, with a bird crossing it while Everroot looks for trees.'}
    preserveAspectRatio="xMidYMax meet"
  >
    <defs>
      <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="#fdfcf7" />
        <stop offset="62%" stop-color="#eef2e6" />
        <stop offset="100%" stop-color="#e3ead9" />
      </linearGradient>
    </defs>

    <rect width="800" height="240" fill="url(#sky)" />
    <!-- The one sun. Nothing else in the scene is allowed to be a sun: when
         the bird lands the picture must still have exactly this many. -->
    <circle cx="686" cy="50" r="26" fill="#e8c98f" opacity="0.85" />

    <!-- Distant treeline, then the garden it sits in. -->
    <path
      d="M0 150 Q 70 122 140 142 T 290 136 Q 360 116 430 138 T 580 132 Q 660 114 730 138 T 800 130 L800 240 L0 240 Z"
      fill="#c4cfb6"
    />
    <path
      d="M0 178 Q 90 160 180 176 T 360 170 Q 450 154 540 174 T 720 168 Q 770 160 800 168 L800 240 L0 240 Z"
      fill="#a9b38d"
    />
    <rect y="196" width="800" height="44" fill="#8a9a85" />

    <!--
      The garden's trees. The one at x=500 is the tree Everroot chose, and it
      is the only one the bird ever lands on.
    -->
    <g class="tree tree--far">
      <rect x="96" y="150" width="7" height="48" fill="#8c5a32" />
      <circle cx="99" cy="140" r="30" fill="#3a6b55" />
      <circle cx="80" cy="152" r="21" fill="#416f59" />
      <circle cx="118" cy="152" r="21" fill="#356349" />
    </g>
    <g class="tree tree--far">
      <rect x="214" y="158" width="6" height="40" fill="#8c5a32" />
      <circle cx="217" cy="150" r="24" fill="#416f59" />
      <circle cx="201" cy="160" r="17" fill="#3a6b55" />
      <circle cx="233" cy="160" r="17" fill="#356349" />
    </g>
    <g class="tree tree--far">
      <rect x="352" y="152" width="7" height="46" fill="#8c5a32" />
      <circle cx="355" cy="142" r="28" fill="#356349" />
      <circle cx="336" cy="154" r="19" fill="#3a6b55" />
      <circle cx="374" cy="154" r="19" fill="#416f59" />
    </g>
    <g class="tree tree--far">
      <rect x="666" y="156" width="6" height="42" fill="#8c5a32" />
      <circle cx="669" cy="148" r="25" fill="#3a6b55" />
      <circle cx="652" cy="158" r="18" fill="#416f59" />
      <circle cx="686" cy="158" r="18" fill="#356349" />
    </g>

    <!-- The chosen tree: drawn a little larger, and ringed once it is chosen.
         The marker is an outline rather than a filled glow on purpose. A warm
         disc fading in behind the bird would be a second sun in a scene that
         already has one, and a visitor would have to work out which of the two
         the bird had flown in from. -->
    <g class="tree tree--chosen">
      <rect x="493" y="146" width="13" height="54" fill="#8c5a32" />
      <circle cx="500" cy="128" r="40" fill="#244f3d" />
      <circle cx="470" cy="146" r="28" fill="#2b5a45" />
      <circle cx="530" cy="146" r="28" fill="#1b3d2e" />
      <circle cx="486" cy="112" r="22" fill="#3a6b55" />
      <circle cx="516" cy="114" r="20" fill="#2f6049" />
      <!-- Drawn last, so it rings the canopy rather than hiding behind it, and
           sized to clear the canopy's widest foliage. -->
      <circle class="chosen-ring" cx="500" cy="128" r="64" fill="none" stroke="#d9a44b" stroke-width="3" />
    </g>

    <!-- The bird. Its wings beat while it flies and fold when it settles. -->
    <g class="bird" class:bird--perched={phase === 'landed'}>
      <g class="bird__body">
        <path class="bird__tail" d="M-16 -1 L-28 -6 L-27 4 Z" fill="#1b3d2e" />
        <ellipse cx="0" cy="0" rx="14" ry="7" fill="#244f3d" />
        <circle cx="12" cy="-5" r="6" fill="#2b5a45" />
        <path d="M17 -5 L25 -3 L17 -1 Z" fill="#d9a44b" />
        <circle cx="13.5" cy="-6.5" r="1.3" fill="#183d32" />
        <path class="wing wing--near" d="M-2 -2 Q -12 -22 6 -16 Q 12 -12 2 -3 Z" fill="#1b3d2e" />
        <path class="wing wing--far" d="M0 -2 Q -10 -19 7 -14 Q 12 -10 3 -3 Z" fill="#3a6b55" />
      </g>
    </g>
  </svg>

  {#if children}
    <div class="scene__caption">{@render children()}</div>
  {/if}
</div>

<style>
  .scene {
    position: relative;
    overflow: hidden;
    border-radius: var(--radius-control, 14px);
    border: 1px solid var(--color-line, #e1e2d8);
    background: var(--surface-soft, #fdfcf7);
  }

  /*
    The scene is drawn at its own proportions and nothing is cropped out of it.
    A fixed height with `slice` scaled the drawing to cover the box, which cut
    a different slice away at every width: on a phone it trimmed the sides, and
    on a desktop it trimmed so much from the top that the sun and the bird's
    whole flight path sat above the visible edge. Matching the height to the
    drawing's ratio instead means all 800 units are on screen at every width,
    and the bird cannot leave the box simply by leaving the drawing.

    Below about 500px the ratio would make the scene shorter than the caption
    needs, so a floor holds it open. That band is the scene's own background
    colour, which is also the palest colour in the sky, so it reads as sky.
  */
  .scene svg {
    display: block;
    width: 100%;
    height: auto;
    aspect-ratio: 800 / 240;
    min-height: 150px;
  }

  /*
    Rotation is about the bird itself. Left at an SVG element's default origin
    it would swing around the centre of the whole drawing instead, so every
    turn in the keyframes would carry the bird up and out of frame rather than
    tipping it in place.
  */
  .bird {
    transform-box: fill-box;
    transform-origin: 50% 50%;
    transform: translate(44px, 66px);
  }

  .scene[data-phase='flying'] .bird {
    animation: fly-across 7.5s linear infinite;
  }

  .scene[data-phase='landing'] .bird {
    animation: land 1.6s cubic-bezier(0.33, 0.9, 0.3, 1) forwards;
  }

  .scene[data-phase='landed'] .bird {
    transform: translate(500px, 84px);
  }

  .wing {
    transform-box: fill-box;
    transform-origin: 50% 100%;
    animation: flap 0.34s ease-in-out infinite;
  }

  .wing--far {
    animation-duration: 0.34s;
    opacity: 0.85;
  }

  .bird--perched .wing,
  .scene[data-phase='landed'] .wing {
    animation: none;
    transform: scaleY(0.42) translateY(1px);
  }

  /* The ring only exists once the tree is the answer, so the picture agrees
     with the shortlist rather than decorating a tree nobody chose. */
  .chosen-ring {
    opacity: 0;
    transition: opacity 0.7s ease 0.7s;
  }

  .scene--landed .chosen-ring {
    opacity: 1;
  }

  .scene__caption {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 0.65rem 0.9rem;
    background: linear-gradient(to top, rgba(253, 252, 247, 0.96), rgba(253, 252, 247, 0));
    font-size: 0.9rem;
    color: var(--color-charcoal-soft, #5a645b);
    line-height: 1.5;
  }

  /*
    The bird's own drawing runs from -28 to 25 units wide and -22 to 7 tall
    around the point these numbers move, and a five-degree turn adds a little
    more on every side. Kept between 34 and 756 across and 26 and 215 down,
    the whole bird — tail, beak and both wings — is inside the 800 x 240
    drawing at every frame, including the first and the last. The old path
    began at -90 and finished at 890, so it started and ended with nothing on
    screen at all, and the loop appeared to pause rather than cross.
  */
  @keyframes fly-across {
    0% {
      transform: translate(44px, 66px) rotate(-5deg);
    }
    25% {
      transform: translate(210px, 44px) rotate(3deg);
    }
    50% {
      transform: translate(380px, 72px) rotate(-4deg);
    }
    75% {
      transform: translate(566px, 42px) rotate(4deg);
    }
    100% {
      transform: translate(756px, 64px) rotate(-2deg);
    }
  }

  @keyframes land {
    0% {
      transform: translate(44px, 64px) rotate(-5deg);
    }
    55% {
      transform: translate(300px, 40px) rotate(4deg);
    }
    84% {
      transform: translate(468px, 62px) rotate(-3deg);
    }
    100% {
      transform: translate(500px, 84px) rotate(0deg);
    }
  }

  @keyframes flap {
    0%,
    100% {
      transform: scaleY(1) rotate(0deg);
    }
    50% {
      transform: scaleY(0.3) rotate(-6deg);
    }
  }

  /* The finished picture, with nothing moving: the answer is still readable
     and the bird is still on the tree. */
  @media (prefers-reduced-motion: reduce) {
    .scene[data-phase='flying'] .bird,
    .scene[data-phase='landing'] .bird {
      animation: none;
      transform: translate(500px, 84px);
    }

    .wing {
      animation: none;
      transform: scaleY(0.42);
    }

    .chosen-ring {
      transition: none;
    }
  }
</style>
