<script lang="ts">
  import { page } from '$app/state';
  import Icon from '../lib/components/Icon.svelte';
  import { absoluteUrl, OG_IMAGE_PATH, SITE_ALTERNATE_NAME, SITE_NAME } from '../lib/site';

  const canonical = $derived(absoluteUrl('/', page.url.origin));
  const socialImage = $derived(absoluteUrl(OG_IMAGE_PATH, page.url.origin));

  const steps = [
    {
      id: 'one',
      title: 'Tell us what matters to you',
      copy: 'Choose an intention, or simply browse. No account, no birth date, and no personal details are needed to begin.',
      icon: 'sprout',
    },
    {
      id: 'two',
      title: 'Discover trees and the stories connected to them',
      copy: 'Everroot looks for reviewed associations between a tradition, a region, and a tree, then shows you the sources behind them.',
      icon: 'book',
    },
    {
      id: 'three',
      title: 'Explore the evidence, or keep a tree that matters',
      copy: 'Read what each source actually supports, save a tree to this device, and see plainly where there is no verified planting partner yet.',
      icon: 'layers',
    },
  ];

  const kinds = [
    {
      id: 'tradition',
      icon: 'scale',
      title: 'Documented tradition',
      copy: 'A claim that appears in a reviewed source, shown together with that source, its region, and its review status.',
    },
    {
      id: 'botanical',
      icon: 'leaf',
      title: 'Botanical facts',
      copy: 'Species identifications, kept separate from cultural meaning and labelled when they are not definitive.',
    },
    {
      id: 'personal',
      icon: 'sparkle',
      title: 'Your own meaning',
      copy: 'What you choose a tree to represent. Personal symbolism is never presented as a cultural rule.',
    },
    {
      id: 'uncertain',
      icon: 'info',
      title: 'Uncertainty',
      copy: 'Where communities differ, or the record is thin, Everroot says so instead of filling the gap.',
    },
  ];
</script>

<svelte:head>
  <title>Everroot — Discover the tree that holds your story</title>
  <meta name="description" content="Find a tree connected to your story, your traditions, or someone you want to remember. Everroot shows reviewed cultural associations with their sources, and is honest about what is not yet recorded." />
  <link rel="canonical" href={canonical} />

  <meta property="og:type" content="website" />
  <meta property="og:url" content={canonical} />
  <meta property="og:title" content="Everroot — Discover the tree that holds your story" />
  <meta property="og:description" content="Find a tree connected to your story, your traditions, or someone you want to remember. Reviewed associations, with the sources attached." />
  <meta property="og:image" content={socialImage} />
  <meta property="og:image:width" content="1200" />
  <meta property="og:image:height" content="630" />
  <meta property="og:site_name" content={SITE_NAME} />

  <meta name="twitter:card" content="summary_large_image" />
  <meta name="twitter:url" content={canonical} />
  <meta name="twitter:title" content="Everroot — Discover the tree that holds your story" />
  <meta name="twitter:description" content="Find a tree connected to your story, your traditions, or someone you want to remember." />

  <script type="application/ld+json">
    {JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'WebSite',
      name: SITE_NAME,
      alternateName: SITE_ALTERNATE_NAME,
      description:
        'An evidence-led tree discovery portal. Find a tree connected to your story, your traditions, or someone you want to remember, with reviewed cultural sources attached.',
      url: canonical,
      inLanguage: 'en',
      publisher: { '@type': 'Organization', name: SITE_NAME, url: canonical },
    })}
  </script>
</svelte:head>

<!-- Hero — two balanced columns on the shared content grid -->
<section class="hero" aria-labelledby="hero-title">
  <div class="hero__wash" aria-hidden="true"></div>

  <div class="container hero__grid">
    <div class="hero__copy">
      <p class="eyebrow">An evidence-led tree discovery</p>
      <h1 id="hero-title">Some things we plant grow beyond our lifetime.</h1>
      <p class="hero__lede">
        Find a tree connected to your story, your traditions, or someone you want to
        remember. Begin with what matters to you.
      </p>

      <div class="hero__actions">
        <a href="/discover" class="btn btn--primary btn--lg">Find my tree</a>
        <a href="#how" class="btn btn--soft btn--lg">How Everroot works</a>
      </div>

      <p class="hero__note">
        <Icon name="shield" size={17} />
        <span>No account needed · No birth date · You control what you share</span>
      </p>
    </div>

    <div class="hero__visual">
      <div class="tree-stage" aria-hidden="true">
        <svg viewBox="0 0 800 800" class="tree-svg">
          <defs>
            <radialGradient id="sunGlow" cx="70%" cy="16%" r="62%">
              <stop offset="0%" stop-color="#fbeec2" stop-opacity="0.9" />
              <stop offset="55%" stop-color="#e8c98a" stop-opacity="0.3" />
              <stop offset="100%" stop-color="#e8c98a" stop-opacity="0" />
            </radialGradient>
            <linearGradient id="barkGrad" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stop-color="#573f2e" />
              <stop offset="42%" stop-color="#7f5f43" />
              <stop offset="100%" stop-color="#4a3626" />
            </linearGradient>
            <radialGradient id="canopyBack" cx="46%" cy="28%" r="74%">
              <stop offset="0%" stop-color="#7c916a" />
              <stop offset="100%" stop-color="#5a7451" />
            </radialGradient>
            <radialGradient id="canopyMid" cx="44%" cy="28%" r="74%">
              <stop offset="0%" stop-color="#94a87a" />
              <stop offset="100%" stop-color="#6d865e" />
            </radialGradient>
            <radialGradient id="canopyFront" cx="40%" cy="26%" r="74%">
              <stop offset="0%" stop-color="#a9bb8b" />
              <stop offset="100%" stop-color="#8ba072" />
            </radialGradient>
            <radialGradient id="moteGrad">
              <stop offset="0%" stop-color="#fff7dc" stop-opacity="0.95" />
              <stop offset="60%" stop-color="#f2dda4" stop-opacity="0.4" />
              <stop offset="100%" stop-color="#f2dda4" stop-opacity="0" />
            </radialGradient>
            <linearGradient id="waterGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stop-color="#c6dad7" />
              <stop offset="34%" stop-color="#9dbcbe" />
              <stop offset="100%" stop-color="#789ba1" />
            </linearGradient>
            <filter id="organic" x="-35%" y="-35%" width="170%" height="170%">
              <feTurbulence type="fractalNoise" baseFrequency="0.009 0.013" numOctaves="4" seed="7" result="noise" />
              <feDisplacementMap in="SourceGraphic" in2="noise" scale="40" xChannelSelector="R" yChannelSelector="G" />
            </filter>
            <filter id="groundBlur" x="-30%" y="-80%" width="160%" height="260%">
              <feGaussianBlur stdDeviation="9" />
            </filter>
            <clipPath id="trunkClip">
              <path d="M356 656 C 370 606 376 556 381 500 C 386 440 390 370 393 300 L 407 300 C 410 370 414 440 419 500 C 424 556 430 606 444 656 Z" />
            </clipPath>
          </defs>

          <!-- warm daylight -->
          <circle cx="560" cy="90" r="200" fill="url(#sunGlow)" />

          <!-- ground -->
          <g filter="url(#groundBlur)">
            <ellipse cx="400" cy="662" rx="198" ry="24" fill="#7d8a6f" opacity="0.2" />
            <ellipse cx="404" cy="656" rx="116" ry="12" fill="#5d6a50" opacity="0.28" />
          </g>

          <!-- grass -->
          <g class="grass" stroke="#7f9469" stroke-width="4" stroke-linecap="round" fill="none" opacity="0.8">
            <path d="M232 662 q -3 -20 -12 -30" />
            <path d="M244 664 q 2 -22 12 -32" />
            <path d="M556 662 q 3 -20 12 -30" />
            <path d="M568 664 q -2 -22 -12 -32" />
            <path d="M300 668 q -2 -16 -10 -24" />
            <path d="M500 668 q 2 -16 10 -24" />
          </g>

          <!-- brook -->
          <g class="brook">
            <path d="M0 800 L0 698 C 80 704 160 710 240 712 C 340 715 400 718 460 722 C 580 728 690 734 800 740 L800 800 Z" fill="url(#waterGrad)" />
            <path d="M0 698 C 80 704 160 710 240 712 C 340 715 400 718 460 722 C 580 728 690 734 800 740" fill="none" stroke="#e4efea" stroke-width="3" opacity="0.55" />
            <g class="stones">
              <ellipse cx="118" cy="710" rx="25" ry="9" fill="#9aa08e" />
              <ellipse cx="152" cy="715" rx="15" ry="7" fill="#8b917f" />
              <ellipse cx="472" cy="742" rx="19" ry="7" fill="#9aa08e" />
              <ellipse cx="708" cy="752" rx="16" ry="6" fill="#8b917f" />
              <ellipse cx="676" cy="760" rx="10" ry="5" fill="#9aa08e" />
            </g>
            <g class="reeds" stroke="#7f9469" stroke-width="3.5" stroke-linecap="round" fill="none" opacity="0.9">
              <path d="M58 706 q -4 -26 -12 -40" />
              <path d="M66 704 q 3 -28 12 -44" />
              <path d="M74 708 q -1 -22 -8 -34" />
              <path d="M746 744 q 4 -24 12 -36" />
              <path d="M754 742 q -2 -20 -9 -30" />
            </g>
            <g class="ripples" stroke="#eef6f2" stroke-width="2.5" fill="none" stroke-linecap="round">
              <path class="ripple r1" d="M40 738 q 14 -6 28 0 t 28 0 t 28 0" />
              <path class="ripple r2" d="M180 758 q 14 -6 28 0 t 28 0 t 28 0" />
              <path class="ripple r3" d="M300 748 q 12 -5 24 0 t 24 0" />
              <path class="ripple r4" d="M452 762 q 14 -6 28 0 t 28 0 t 28 0" />
              <path class="ripple r5" d="M600 778 q 14 -6 28 0 t 28 0 t 28 0" />
            </g>
          </g>

          <!-- trunk -->
          <g class="trunk-group">
            <path d="M356 656 C 370 606 376 556 381 500 C 386 440 390 370 393 300 L 407 300 C 410 370 414 440 419 500 C 424 556 430 606 444 656 Z" fill="url(#barkGrad)" />
            <g clip-path="url(#trunkClip)" fill="none" stroke="#3f2d1f" stroke-linecap="round" opacity="0.42">
              <path d="M378 654 C 384 566 386 468 392 336" stroke-width="3" />
              <path d="M400 654 C 400 546 400 424 400 306" stroke-width="2.4" />
              <path d="M421 654 C 415 566 411 476 407 344" stroke-width="3" />
              <path d="M390 600 C 394 570 395 540 394 512" stroke-width="2" />
              <path d="M410 590 C 407 562 406 534 407 508" stroke-width="2" />
            </g>
          </g>

          <!-- branches that unfurl on load -->
          <g class="branches" fill="none" stroke="url(#barkGrad)" stroke-linecap="round">
            <path class="branch" pathLength="1" d="M400 316 C 356 298 312 270 268 242" stroke-width="15" />
            <path class="branch" pathLength="1" d="M400 316 C 444 298 488 270 532 242" stroke-width="15" />
            <path class="branch" pathLength="1" d="M398 372 C 356 366 312 356 274 348" stroke-width="11" />
            <path class="branch" pathLength="1" d="M402 372 C 444 366 488 356 526 348" stroke-width="11" />
            <path class="branch" pathLength="1" d="M398 428 C 360 432 320 434 288 438" stroke-width="8" />
            <path class="branch" pathLength="1" d="M402 428 C 440 432 480 434 512 438" stroke-width="8" />
          </g>
          <g class="twigs" fill="none" stroke="url(#barkGrad)" stroke-linecap="round" stroke-width="5">
            <path class="twig" pathLength="1" d="M268 242 C 250 224 240 206 236 186" />
            <path class="twig" pathLength="1" d="M268 242 C 284 222 292 204 294 182" />
            <path class="twig" pathLength="1" d="M532 242 C 550 224 560 206 564 186" />
            <path class="twig" pathLength="1" d="M532 242 C 516 222 508 204 506 182" />
            <path class="twig" pathLength="1" d="M274 348 C 254 334 242 318 236 300" />
            <path class="twig" pathLength="1" d="M274 348 C 290 332 300 316 304 298" />
            <path class="twig" pathLength="1" d="M526 348 C 546 334 558 318 564 300" />
            <path class="twig" pathLength="1" d="M526 348 C 510 332 500 316 496 298" />
            <path class="twig" pathLength="1" d="M288 438 C 274 428 266 416 262 402" />
            <path class="twig" pathLength="1" d="M512 438 C 526 428 534 416 538 402" />
          </g>

          <!-- canopy: three depth layers that sway out of phase -->
          <g class="canopy">
            <g class="sway sway--back">
              <g class="bloom bloom--back">
                <g filter="url(#organic)" fill="url(#canopyBack)">
                  <circle cx="400" cy="175" r="105" />
                  <circle cx="292" cy="235" r="95" />
                  <circle cx="508" cy="232" r="98" />
                  <circle cx="240" cy="335" r="82" />
                  <circle cx="560" cy="332" r="84" />
                  <circle cx="338" cy="400" r="88" />
                  <circle cx="466" cy="402" r="90" />
                  <circle cx="400" cy="305" r="112" />
                </g>
              </g>
            </g>
            <g class="sway sway--mid">
              <g class="bloom bloom--mid">
                <g filter="url(#organic)" fill="url(#canopyMid)">
                  <circle cx="396" cy="205" r="88" />
                  <circle cx="312" cy="268" r="80" />
                  <circle cx="486" cy="264" r="82" />
                  <circle cx="272" cy="348" r="70" />
                  <circle cx="528" cy="344" r="72" />
                  <circle cx="368" cy="416" r="72" />
                  <circle cx="438" cy="418" r="74" />
                  <circle cx="400" cy="335" r="92" />
                </g>
                <g filter="url(#organic)" fill="#5d7647" opacity="0.42">
                  <circle cx="312" cy="342" r="80" />
                  <circle cx="354" cy="410" r="70" />
                  <circle cx="300" cy="264" r="64" />
                  <circle cx="432" cy="414" r="64" />
                </g>
              </g>
            </g>
            <g class="sway sway--front">
              <g class="bloom bloom--front">
                <g filter="url(#organic)" fill="url(#canopyFront)">
                  <circle cx="450" cy="212" r="64" />
                  <circle cx="498" cy="284" r="56" />
                  <circle cx="416" cy="262" r="70" />
                  <circle cx="470" cy="340" r="48" />
                  <circle cx="392" cy="342" r="56" />
                  <circle cx="446" cy="266" r="52" />
                </g>
              </g>
            </g>
            <g class="rimlight" filter="url(#organic)" fill="#dde8c2" opacity="0.42">
              <circle cx="472" cy="182" r="42" />
              <circle cx="522" cy="246" r="30" />
              <circle cx="452" cy="136" r="26" />
              <circle cx="394" cy="124" r="22" />
            </g>
          </g>

          <!-- birdhouse -->
          <g class="birdhouse">
            <rect x="137" y="556" width="6" height="158" fill="#7b5c3e" />
            <path d="M104 500 L 140 464 L 176 500 Z" fill="#6d523a" />
            <rect x="108" y="500" width="64" height="56" rx="5" fill="#96714d" />
            <rect x="108" y="500" width="64" height="9" fill="#8a6a46" />
            <circle cx="140" cy="528" r="11" fill="#3a2c20" />
            <rect x="170" y="540" width="46" height="5" rx="2.5" fill="#7b5c3e" />
            <g class="house-bird" transform="translate(206,540)">
              <g class="house-bird-inner">
                <path d="M7 -8 L 21 -3 L 17 -14 Z" fill="#5f7a9c" />
                <ellipse cx="0" cy="-9" rx="11" ry="7.5" fill="#6f88a8" />
                <ellipse cx="-2" cy="-6" rx="7" ry="5" fill="#c9d3de" />
                <circle cx="-10" cy="-17" r="6" fill="#5f7a9c" />
                <path d="M-14 -18.5 L -25 -16 L -14 -13.5 Z" fill="#e0a13f" />
                <circle cx="-11" cy="-19" r="1.3" fill="#2f3a44" />
                <path d="M-3 -2 L -3 3 M3 -2 L 3 3" stroke="#8a6a46" stroke-width="2" fill="none" />
              </g>
            </g>
          </g>

          <!-- nests with chicks -->
          <g class="nest" transform="translate(264,436)">
            <g class="chicks">
              <g transform="translate(-16,3)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
              <g transform="translate(15,1)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
              <g transform="translate(0,-3)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
            </g>
            <path d="M-30 4 q -14 -6 -22 -18" fill="none" stroke="#8a9a5f" stroke-width="3" stroke-linecap="round" opacity="0.9" />
            <path d="M30 4 q 12 -8 18 -20" fill="none" stroke="#8a9a5f" stroke-width="3" stroke-linecap="round" opacity="0.9" />
            <path class="nest-bowl" d="M-36 -2 C -33 18 -16 29 0 29 C 16 29 33 18 36 -2 C 24 6 -24 6 -36 -2 Z" fill="#a3825a" />
            <path d="M-36 -2 C -24 6 24 6 36 -2" fill="none" stroke="#8a6a42" stroke-width="3" />
            <g stroke="#8a6a42" stroke-width="1.8" fill="none" opacity="0.6">
              <path d="M-30 4 C -22 16 -10 22 0 22" />
              <path d="M-16 -1 C -10 12 -2 22 6 24" />
              <path d="M12 0 C 4 14 -6 22 -14 23" />
              <path d="M30 3 C 22 15 10 21 2 22" />
              <path d="M-36 -2 C -30 10 -22 18 -14 21" />
              <path d="M36 -2 C 30 10 22 18 14 21" />
            </g>
          </g>

          <g class="nest" transform="translate(540,432) scale(0.88)">
            <g class="chicks">
              <g transform="translate(-15,3)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
              <g transform="translate(14,0)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
              <g transform="translate(0,-4)">
                <g class="chick">
                  <circle cx="0" cy="-3" r="8" fill="#e3c06a" />
                  <circle cx="-2.5" cy="-4" r="1.3" fill="#3a2c20" />
                  <path class="jaw jaw-up" d="M-7 -7 L -3 -22 L 3 -8 Z" fill="#e8a13f" />
                  <path class="jaw jaw-low" d="M-1 -8 L 8 -20 L 10 -6 Z" fill="#d98b32" />
                </g>
              </g>
            </g>
            <path d="M-28 4 q -13 -6 -20 -17" fill="none" stroke="#8a9a5f" stroke-width="3" stroke-linecap="round" opacity="0.9" />
            <path d="M28 4 q 11 -8 17 -19" fill="none" stroke="#8a9a5f" stroke-width="3" stroke-linecap="round" opacity="0.9" />
            <path class="nest-bowl" d="M-36 -2 C -33 18 -16 29 0 29 C 16 29 33 18 36 -2 C 24 6 -24 6 -36 -2 Z" fill="#a3825a" />
            <path d="M-36 -2 C -24 6 24 6 36 -2" fill="none" stroke="#8a6a42" stroke-width="3" />
            <g stroke="#8a6a42" stroke-width="1.8" fill="none" opacity="0.6">
              <path d="M-30 4 C -22 16 -10 22 0 22" />
              <path d="M-16 -1 C -10 12 -2 22 6 24" />
              <path d="M12 0 C 4 14 -6 22 -14 23" />
              <path d="M30 3 C 22 15 10 21 2 22" />
              <path d="M-36 -2 C -30 10 -22 18 -14 21" />
              <path d="M36 -2 C 30 10 22 18 14 21" />
            </g>
          </g>

          <!-- bird drinking at the brook -->
          <g class="pecker" transform="translate(300,700)">
            <g class="pecker-legs" stroke="#8a6a46" stroke-width="2.6" stroke-linecap="round" fill="none">
              <path d="M-2 -2 L -4 11 M3 -2 L 5 11" />
              <path d="M-4 11 L -9 13 M-4 11 L 1 12 M5 11 L 0 12 M5 11 L 10 13" />
            </g>
            <g class="pecker-body">
              <path d="M-14 -12 L -31 -21 L -28 -7 Z" fill="#6c7f63" />
              <ellipse cx="-4" cy="-11" rx="13.5" ry="10" fill="#7d8f72" />
              <ellipse cx="-6" cy="-8" rx="8" ry="5.5" fill="#94a684" />
              <g class="pecker-neck">
                <path d="M4 -14 C 12 -16 18 -22 22 -30" stroke="#7d8f72" stroke-width="6.5" fill="none" stroke-linecap="round" />
                <circle cx="25" cy="-34" r="7" fill="#7d8f72" />
                <path d="M30 -37 L 44 -30 L 30 -28 Z" fill="#d9932f" />
                <circle cx="27" cy="-36" r="1.5" fill="#3a2c20" />
              </g>
            </g>
          </g>

          <!-- birds overhead -->
          <g class="flock">
            <g class="bird b1" transform="translate(140,152) scale(1.12)">
              <g class="bird-drift">
                <g class="bird-inner">
                  <path class="wing wl" d="M-4 0 C -12 -11 -26 -15 -38 -9 C -26 -2 -12 3 -4 3 Z" fill="#41524a" />
                  <path class="wing wr" d="M4 0 C 12 -11 26 -15 38 -9 C 26 -2 12 3 4 3 Z" fill="#41524a" />
                  <ellipse cx="0" cy="1" rx="7" ry="5" fill="#4a5c52" />
                  <path d="M-6 1 L -18 6 L -15 -3 Z" fill="#41524a" />
                </g>
              </g>
            </g>
            <g class="bird b2" transform="translate(208,104) scale(0.86)">
              <g class="bird-drift">
                <g class="bird-inner">
                  <path class="wing wl" d="M-4 0 C -12 -11 -26 -15 -38 -9 C -26 -2 -12 3 -4 3 Z" fill="#4b5d54" />
                  <path class="wing wr" d="M4 0 C 12 -11 26 -15 38 -9 C 26 -2 12 3 4 3 Z" fill="#4b5d54" />
                  <ellipse cx="0" cy="1" rx="7" ry="5" fill="#54665c" />
                  <path d="M-6 1 L -18 6 L -15 -3 Z" fill="#4b5d54" />
                </g>
              </g>
            </g>
            <g class="bird b3" transform="translate(690,180) scale(1)">
              <g class="bird-drift">
                <g class="bird-inner">
                  <path class="wing wl" d="M-4 0 C -12 -11 -26 -15 -38 -9 C -26 -2 -12 3 -4 3 Z" fill="#41524a" />
                  <path class="wing wr" d="M4 0 C 12 -11 26 -15 38 -9 C 26 -2 12 3 4 3 Z" fill="#41524a" />
                  <ellipse cx="0" cy="1" rx="7" ry="5" fill="#4a5c52" />
                  <path d="M-6 1 L -18 6 L -15 -3 Z" fill="#41524a" />
                </g>
              </g>
            </g>
          </g>

          <!-- drifting light motes -->
          <g class="motes" fill="url(#moteGrad)">
            <circle class="mote m1" cx="252" cy="300" r="7" />
            <circle class="mote m2" cx="330" cy="196" r="6" />
            <circle class="mote m3" cx="470" cy="372" r="8" />
            <circle class="mote m4" cx="548" cy="268" r="6" />
            <circle class="mote m5" cx="396" cy="150" r="7" />
            <circle class="mote m6" cx="586" cy="404" r="6" />
          </g>

          <!-- leaves that release and fall -->
          <g class="falling" fill="#a9bd85">
            <path class="fleaf f1" d="M250 300 C 259 308 261 322 250 333 C 239 322 237 308 250 300 Z" />
            <path class="fleaf f2" d="M540 258 C 549 266 551 280 540 291 C 529 280 527 266 540 258 Z" />
            <path class="fleaf f3" d="M330 198 C 339 206 341 220 330 231 C 319 220 317 206 330 198 Z" />
            <path class="fleaf f4" d="M470 214 C 479 222 481 236 470 247 C 459 236 457 222 470 214 Z" />
          </g>
        </svg>
      </div>
      <p class="visual-caption">Rooted, patient, always reaching.</p>
    </div>
  </div>
</section>

<!-- How Everroot works — three steps -->
<section id="how" class="how" aria-labelledby="how-title">
  <div class="container">
    <header class="section-head-block">
      <p class="eyebrow">How Everroot works</p>
      <h2 id="how-title" class="section-head">Three steps, and nothing you have to disclose.</h2>
      <p class="section-sub">
        Everroot is a discovery tool, not a prediction. It shows you what is on record — and
        where the record is still thin.
      </p>
    </header>

    <ol class="how__steps">
      {#each steps as step, i}
        <li class="step-card">
          <span class="step-card__icon" aria-hidden="true"><Icon name={step.icon} size={24} /></span>
          <span class="step-card__n" aria-hidden="true">{i + 1}</span>
          <h3>{step.title}</h3>
          <p>{step.copy}</p>
        </li>
      {/each}
    </ol>

    <div class="how__cta">
      <a href="/discover" class="btn btn--primary btn--lg">Find my tree</a>
      <a href="/trees" class="btn btn--soft btn--lg">Browse the tree reference</a>
      <p class="how__cta-note">
        It takes about two minutes, and you can stop at any point. The reference is a plain list of
        tree identities with their sources — no cultural claim attached.
      </p>
    </div>
  </div>
</section>

<!-- What Everroot keeps apart -->
<section class="kinds" aria-labelledby="kinds-title">
  <div class="container kinds__grid">
    <div class="kinds__intro">
      <p class="eyebrow">Trust</p>
      <h2 id="kinds-title" class="section-head">We keep four kinds of knowledge apart.</h2>
      <p class="section-sub">
        Blurring them is how cultural claims get invented. Every result on Everroot says which
        kind of knowledge you are looking at.
      </p>
    </div>

    <ul class="kinds__list">
      {#each kinds as item}
        <li class="kind">
          <span class="kind__icon" aria-hidden="true"><Icon name={item.icon} size={20} /></span>
          <div>
            <h3 class="kind__title">{item.title}</h3>
            <p class="kind__copy">{item.copy}</p>
          </div>
        </li>
      {/each}
    </ul>
  </div>
</section>

<style>
  /* ---------------------------------------------------------------- Hero */
  .hero {
    position: relative;
    overflow: hidden;
    background: var(--color-ivory);
  }

  .hero__wash {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(ellipse 62% 46% at 74% 12%, rgba(217, 164, 75, 0.16), transparent 68%),
      radial-gradient(ellipse 70% 60% at 12% 88%, rgba(138, 154, 133, 0.16), transparent 70%),
      linear-gradient(180deg, #fdfcf7 0%, var(--color-ivory) 62%, #eeece1 100%);
  }

  .hero__grid {
    position: relative;
    z-index: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: clamp(1.8rem, 4.5vw, 4.25rem);
    align-items: center;
    padding-block: clamp(2.25rem, 5.5vh, 4.25rem);
    min-height: min(820px, calc(100svh - var(--header-height)));
  }

  .hero__copy {
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .hero h1 {
    margin-top: 0.9rem;
    font-size: clamp(1.95rem, 5.2vw, 3.3rem);
    line-height: 1.08;
    letter-spacing: -0.03em;
    max-width: 22ch;
  }

  .hero__lede {
    margin-top: 1.15rem;
    font-size: clamp(1.02rem, 1.25vw, 1.16rem);
    line-height: 1.72;
    color: #48533f;
    max-width: 44ch;
  }

  .hero__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-top: 1.75rem;
  }

  .hero__note {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1.5rem;
    font-size: 0.87rem;
    line-height: 1.5;
    color: #6f7a68;
  }

  .hero__note :global(svg) {
    flex: none;
    color: var(--color-forest-soft);
  }

  .hero__visual {
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.35rem;
  }

  .tree-stage {
    width: 100%;
    max-width: 560px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .tree-svg {
    width: 100%;
    height: auto;
    display: block;
    filter: drop-shadow(0 26px 44px rgba(46, 64, 40, 0.15));
  }

  .visual-caption {
    font: italic 1rem var(--font-display);
    color: #79836f;
    text-align: center;
  }

  .hero__copy > * {
    animation: fadeUp 0.85s ease-out both;
  }

  .hero__copy > *:nth-child(1) { animation-delay: 0.05s; }
  .hero__copy > *:nth-child(2) { animation-delay: 0.14s; }
  .hero__copy > *:nth-child(3) { animation-delay: 0.24s; }
  .hero__copy > *:nth-child(4) { animation-delay: 0.34s; }
  .hero__copy > *:nth-child(5) { animation-delay: 0.44s; }

  @keyframes fadeUp {
    from {
      opacity: 0;
      transform: translateY(18px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  /* --------------------------------------------------- Tree: wind + bloom */
  .sway {
    transform-box: view-box;
    transform-origin: 400px 640px;
    will-change: transform;
  }

  .sway--back {
    animation: swayBack 9.5s ease-in-out infinite;
  }

  .sway--mid {
    animation: swayMid 7.6s ease-in-out infinite;
    animation-delay: -1.6s;
  }

  .sway--front {
    animation: swayFront 6.4s ease-in-out infinite;
    animation-delay: -3.1s;
  }

  @keyframes swayBack {
    0%, 100% { transform: rotate(-1.25deg); }
    50% { transform: rotate(1.25deg); }
  }

  @keyframes swayMid {
    0%, 100% { transform: rotate(-0.95deg); }
    50% { transform: rotate(0.95deg); }
  }

  @keyframes swayFront {
    0%, 100% { transform: rotate(-0.7deg) translateX(-2px); }
    50% { transform: rotate(0.7deg) translateX(2px); }
  }

  .bloom {
    transform-box: view-box;
    transform-origin: 400px 305px;
    opacity: 0;
    animation: bloom 1.4s cubic-bezier(0.2, 0.72, 0.28, 1) forwards;
  }

  .bloom--back { animation-delay: 0.45s; }
  .bloom--mid { animation-delay: 0.6s; }
  .bloom--front { animation-delay: 0.75s; }

  @keyframes bloom {
    0% { opacity: 0; transform: scale(0.56); }
    55% { opacity: 1; }
    100% { opacity: 1; transform: scale(1); }
  }

  .trunk-group {
    transform-box: view-box;
    transform-origin: 400px 656px;
    opacity: 0;
    animation: trunkIn 0.95s ease-out 0.05s forwards;
  }

  @keyframes trunkIn {
    0% { opacity: 0; transform: scaleY(0.6) scaleX(0.94); }
    100% { opacity: 1; transform: none; }
  }

  .branch,
  .twig {
    stroke-dasharray: 1;
    stroke-dashoffset: 1;
    animation: draw 1.35s cubic-bezier(0.4, 0.1, 0.2, 1) forwards;
  }

  .branches .branch:nth-child(1) { animation-delay: 0.35s; }
  .branches .branch:nth-child(2) { animation-delay: 0.43s; }
  .branches .branch:nth-child(3) { animation-delay: 0.53s; }
  .branches .branch:nth-child(4) { animation-delay: 0.61s; }
  .branches .branch:nth-child(5) { animation-delay: 0.71s; }
  .branches .branch:nth-child(6) { animation-delay: 0.79s; }
  .twigs .twig { animation-delay: 0.8s; }
  .twigs .twig:nth-child(2n) { animation-delay: 0.88s; }
  .twigs .twig:nth-child(3n) { animation-delay: 0.96s; }

  @keyframes draw {
    to { stroke-dashoffset: 0; }
  }

  .grass {
    opacity: 0;
    animation: softIn 0.9s ease 0.95s forwards;
  }

  @keyframes softIn {
    to { opacity: 0.8; }
  }

  .motes {
    opacity: 0;
    animation: softIn1 0.8s ease 1.2s forwards;
  }

  @keyframes softIn1 {
    to { opacity: 1; }
  }

  .mote {
    transform-box: fill-box;
    transform-origin: center;
    animation: drift 10s ease-in-out infinite;
  }

  .m1 { animation-delay: 0.6s; }
  .m2 { animation-delay: 2.4s; animation-duration: 12s; }
  .m3 { animation-delay: 4.2s; animation-duration: 9s; }
  .m4 { animation-delay: 1.4s; animation-duration: 13s; }
  .m5 { animation-delay: 3.4s; animation-duration: 11s; }
  .m6 { animation-delay: 5.2s; animation-duration: 10.5s; }

  @keyframes drift {
    0% { transform: translate(0, 0) scale(0.5); opacity: 0; }
    22% { opacity: 0.9; }
    70% { opacity: 0.45; }
    100% { transform: translate(28px, -130px) scale(1); opacity: 0; }
  }

  .falling {
    opacity: 0;
    animation: softIn1 0.6s ease 1.5s forwards;
  }

  .fleaf {
    transform-box: fill-box;
    transform-origin: center;
    opacity: 0;
    animation: fall linear infinite;
  }

  .f1 { animation-duration: 12s; animation-delay: 1.5s; }
  .f2 { animation-duration: 15s; animation-delay: 6.5s; }
  .f3 { animation-duration: 13s; animation-delay: 10.5s; }
  .f4 { animation-duration: 16s; animation-delay: 4s; }

  @keyframes fall {
    0% { transform: translate(0, 0) rotate(0); opacity: 0; }
    8% { opacity: 0.85; }
    80% { opacity: 0.5; }
    100% { transform: translate(-48px, 370px) rotate(250deg); opacity: 0; }
  }

  /* Scene pieces fade in with opacity only, so transform attributes survive */
  .brook,
  .birdhouse,
  .nest,
  .flock,
  .pecker {
    opacity: 0;
    animation: sceneIn 1.1s ease forwards;
  }

  .flock { animation-delay: 0.4s; }
  .brook { animation-delay: 0.9s; }
  .birdhouse { animation-delay: 1.02s; }
  .nest { animation-delay: 1.14s; }
  .pecker { animation-delay: 1.26s; }

  @keyframes sceneIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  .ripple { animation: flow 6s ease-in-out infinite; }
  .r1 { animation-delay: 0s; }
  .r2 { animation-delay: 1.3s; }
  .r3 { animation-delay: 2.6s; }
  .r4 { animation-delay: 3.9s; }
  .r5 { animation-delay: 2s; }

  @keyframes flow {
    0% { transform: translateX(0); opacity: 0.08; }
    45% { opacity: 0.6; }
    100% { transform: translateX(34px); opacity: 0.08; }
  }

  .reeds {
    transform-box: view-box;
    transform-origin: 400px 800px;
    animation: reedSway 6.5s ease-in-out infinite;
  }

  @keyframes reedSway {
    0%, 100% { transform: rotate(-0.9deg); }
    50% { transform: rotate(0.9deg); }
  }

  .wl {
    transform-box: fill-box;
    transform-origin: 100% 50%;
    animation: flapL 0.46s ease-in-out infinite;
  }

  .wr {
    transform-box: fill-box;
    transform-origin: 0% 50%;
    animation: flapR 0.46s ease-in-out infinite;
  }

  .b1 .wl, .b1 .wr { animation-delay: -0.1s; }
  .b2 .wl, .b2 .wr { animation-duration: 0.37s; }
  .b3 .wl, .b3 .wr { animation-duration: 0.56s; animation-delay: -0.2s; }

  @keyframes flapL {
    0%, 100% { transform: rotate(-5deg); }
    50% { transform: rotate(27deg); }
  }

  @keyframes flapR {
    0%, 100% { transform: rotate(5deg); }
    50% { transform: rotate(-27deg); }
  }

  .bird-drift { animation: skim 9s ease-in-out infinite alternate; }
  .b1 .bird-drift { animation-duration: 8s; }
  .b2 .bird-drift { animation-duration: 6.5s; animation-delay: -1.5s; }
  .b3 .bird-drift { animation-duration: 11s; animation-delay: -3s; }

  @keyframes skim {
    0% { transform: translate(0, 0); }
    100% { transform: translate(52px, 16px); }
  }

  .bird-inner { animation: bob 2.8s ease-in-out infinite; }
  .b1 .bird-inner { animation-duration: 2.6s; }
  .b2 .bird-inner { animation-duration: 2.2s; animation-delay: -0.7s; }
  .b3 .bird-inner { animation-duration: 3.1s; animation-delay: -1.2s; }

  @keyframes bob {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-7px); }
  }

  .house-bird-inner {
    transform-box: fill-box;
    transform-origin: 50% 100%;
    animation: hop 3.6s ease-in-out infinite;
  }

  @keyframes hop {
    0%, 76%, 100% { transform: translateY(0) rotate(0); }
    85% { transform: translateY(-3px) rotate(-2.5deg); }
    92% { transform: translateY(-1px) rotate(1.5deg); }
  }

  .chick { animation: chickBob 2.3s ease-in-out infinite; }
  .chicks > g:nth-child(1) .jaw-up,
  .chicks > g:nth-child(1) .jaw-low { animation-delay: 0.05s; }
  .chicks > g:nth-child(2) .chick { animation-delay: 0.32s; animation-duration: 2.6s; }
  .chicks > g:nth-child(2) .jaw-up,
  .chicks > g:nth-child(2) .jaw-low { animation-delay: 0.32s; }
  .chicks > g:nth-child(3) .chick { animation-delay: 0.64s; animation-duration: 2.05s; }
  .chicks > g:nth-child(3) .jaw-up,
  .chicks > g:nth-child(3) .jaw-low { animation-delay: 0.64s; }

  @keyframes chickBob {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-3.5px); }
  }

  .jaw { transform-box: fill-box; }
  .jaw-up { transform-origin: 50% 100%; animation: jawUp 1.15s ease-in-out infinite; }
  .jaw-low { transform-origin: 50% 100%; animation: jawLow 1.15s ease-in-out infinite; }

  @keyframes jawUp {
    0%, 100% { transform: rotate(0); }
    50% { transform: rotate(-13deg); }
  }

  @keyframes jawLow {
    0%, 100% { transform: rotate(0); }
    50% { transform: rotate(15deg); }
  }

  .pecker-body { animation: peckerBob 4.6s ease-in-out infinite; }

  @keyframes peckerBob {
    0%, 40%, 100% { transform: translateY(0); }
    56% { transform: translateY(2px); }
  }

  .pecker-neck {
    transform-box: fill-box;
    transform-origin: 0% 100%;
    animation: peck 4.6s ease-in-out infinite;
  }

  @keyframes peck {
    0%, 40%, 100% { transform: rotate(0); }
    52% { transform: rotate(58deg); }
    60% { transform: rotate(50deg); }
    68% { transform: rotate(58deg); }
    78% { transform: rotate(50deg); }
  }

  /* -------------------------------------------------------- How it works */
  .how {
    padding-block: var(--space-section);
    background: var(--surface-muted);
  }

  .section-head-block {
    max-width: 60ch;
  }

  .section-head-block .section-head {
    margin-top: 0.6rem;
  }

  .section-head-block .section-sub {
    margin-top: 0.85rem;
  }

  .how__steps {
    list-style: none;
    margin: 2.75rem 0 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 1.25rem;
    counter-reset: step;
  }

  .step-card {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--color-line);
    border-radius: var(--radius-card);
    padding: 1.75rem 1.6rem 1.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .step-card__icon {
    display: grid;
    place-items: center;
    width: 46px;
    height: 46px;
    border-radius: 13px;
    background: #edf2e9;
    border: 1px solid #dde4d6;
    color: var(--color-forest);
    margin-bottom: 0.35rem;
  }

  .step-card__n {
    position: absolute;
    top: 1.4rem;
    right: 1.5rem;
    font-family: var(--font-display);
    font-size: 2rem;
    line-height: 1;
    color: rgba(36, 79, 61, 0.16);
  }

  .step-card h3 {
    font-size: 1.16rem;
    line-height: 1.35;
    letter-spacing: -0.01em;
  }

  .step-card p {
    color: var(--color-charcoal-soft);
    font-size: 0.97rem;
    line-height: 1.68;
  }

  .how__cta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 1rem;
    margin-top: 2.25rem;
  }

  .how__cta-note {
    color: var(--color-charcoal-soft);
    font-size: 0.92rem;
  }

  /* -------------------------------------------------------------- Trust */
  .kinds {
    padding-block: var(--space-section);
    background: var(--color-ivory);
  }

  .kinds__grid {
    display: grid;
    grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr);
    gap: clamp(1.75rem, 4vw, 3.5rem);
    align-items: start;
  }

  .kinds__intro .section-head {
    margin-top: 0.6rem;
  }

  .kinds__intro .section-sub {
    margin-top: 0.85rem;
  }

  .kinds__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.85rem;
  }

  .kind {
    display: flex;
    gap: 0.9rem;
    align-items: flex-start;
    padding: 1.05rem 1.15rem;
    background: var(--surface-soft);
    border: 1px solid var(--color-line-soft);
    border-radius: var(--radius-control);
  }

  .kind__icon {
    display: grid;
    place-items: center;
    flex: none;
    width: 38px;
    height: 38px;
    border-radius: 10px;
    background: #eef2e8;
    color: var(--color-forest);
  }

  .kind__title {
    font-size: 1.02rem;
    margin-bottom: 0.2rem;
  }

  .kind__copy {
    color: var(--color-charcoal-soft);
    font-size: 0.94rem;
    line-height: 1.65;
  }

  /* -------------------------------------------------------- Responsive */
  @media (min-width: 900px) {
    .hero h1 {
      font-size: clamp(2.5rem, 4.3vw, 4rem);
      max-width: 20ch;
    }
  }

  @media (max-width: 899px) {
    .hero__grid {
      grid-template-columns: minmax(0, 1fr);
      min-height: 0;
      text-align: center;
    }

    .hero__copy {
      align-items: center;
      order: 1;
    }

    .hero h1,
    .hero__lede {
      max-width: 34ch;
    }

    .hero__lede {
      max-width: 46ch;
    }

    .hero__actions {
      justify-content: center;
    }

    .hero__visual {
      order: 2;
      width: 100%;
      max-width: 420px;
      margin-inline: auto;
    }
  }

  @media (max-width: 860px) {
    .how__steps {
      grid-template-columns: minmax(0, 1fr);
    }

    .kinds__grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 560px) {
    .hero__grid {
      padding-block: 1.75rem 2.25rem;
      gap: 1.5rem;
    }

    .hero h1 {
      font-size: clamp(1.8rem, 7.6vw, 2.2rem);
      letter-spacing: -0.02em;
    }

    .hero__lede {
      font-size: 1rem;
      line-height: 1.65;
      margin-top: 0.9rem;
    }

    .hero__actions {
      flex-direction: column;
      align-items: stretch;
      width: 100%;
      margin-top: 1.35rem;
      gap: 0.6rem;
    }

    .hero__note {
      justify-content: center;
      font-size: 0.8rem;
      margin-top: 1.15rem;
    }

    .hero__visual {
      max-width: 280px;
    }

    .how,
    .kinds {
      padding-block: var(--space-section-mobile);
    }

    .how__steps {
      margin-top: 1.9rem;
    }

    .step-card {
      padding: 1.4rem 1.25rem;
    }

    .how__cta {
      flex-direction: column;
      align-items: stretch;
      text-align: center;
    }
  }

  /* --------------------------------------------------- Reduced motion */
  @media (prefers-reduced-motion: reduce) {
    .hero__copy > *,
    .sway,
    .bloom,
    .trunk-group,
    .branch,
    .twig,
    .grass,
    .motes,
    .mote,
    .falling,
    .fleaf,
    .ripple,
    .reeds,
    .wl,
    .wr,
    .bird-drift,
    .bird-inner,
    .house-bird-inner,
    .chick,
    .jaw-up,
    .jaw-low,
    .pecker-body,
    .pecker-neck {
      animation: none !important;
      transition: none !important;
    }

    .bloom,
    .trunk-group,
    .grass,
    .falling,
    .motes,
    .brook,
    .birdhouse,
    .nest,
    .flock,
    .pecker {
      opacity: 1 !important;
    }

    .hero__copy > * {
      opacity: 1 !important;
      transform: none !important;
    }

    .mote,
    .fleaf {
      opacity: 0 !important;
    }

    .branch,
    .twig {
      stroke-dashoffset: 0 !important;
    }
  }
</style>
