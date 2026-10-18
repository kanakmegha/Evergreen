<script lang="ts">
  import '../app.css';
  import { afterNavigate } from '$app/navigation';
  import { page } from '$app/state';

  let { children } = $props();

  let menuOpen = $state(false);
  let toggleEl: HTMLButtonElement | null = $state(null);
  let panelEl: HTMLDivElement | null = $state(null);

  const path = $derived(page.url.pathname);
  const isHome = $derived(path === '/');
  const isDiscover = $derived(path.startsWith('/discover') || path.startsWith('/results'));

  afterNavigate(() => {
    menuOpen = false;
  });

  function openMenu() {
    menuOpen = true;
    queueMicrotask(() => {
      panelEl?.querySelector<HTMLAnchorElement>('a')?.focus();
    });
  }

  function closeMenu(returnFocus = false) {
    menuOpen = false;
    if (returnFocus) toggleEl?.focus();
  }

  function onHeaderKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && menuOpen) {
      event.preventDefault();
      closeMenu(true);
    }
  }
</script>

<svelte:head>
  <meta name="theme-color" content="#f7f6ef" />
  <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
</svelte:head>

<svelte:window onkeydown={onHeaderKeydown} />

<a class="skip-link" href="#main">Skip to content</a>

<header class="site-header">
  <div class="container header__inner">
    <a href="/" class="brand" aria-label="Everroot — home">
      <span class="brand__mark" aria-hidden="true">
        <svg viewBox="0 0 40 40">
          <path d="M20 34V18m0 7c-7 0-11-4-11-10 7 0 11 3 11 10Zm0-5c0-8 4-13 12-13 0 8-4 13-12 13Z" />
          <path d="M9 34h22" />
        </svg>
      </span>
      <span class="brand__text">
        <span class="brand__name">Everroot</span>
        <span class="brand__tag">Knowledge with roots</span>
      </span>
    </a>

    <nav class="nav" aria-label="Primary">
      <ul class="nav__list">
        <li>
          <a href="/" class="nav__link" aria-current={isHome ? 'page' : undefined}>Home</a>
        </li>
        <li>
          <a href="/discover" class="nav__link" aria-current={isDiscover ? 'page' : undefined}>Discover</a>
        </li>
      </ul>
    </nav>

    <div class="header__actions">
      <a href="/discover" class="btn btn--primary btn--sm header__cta">Find my tree</a>
      <button
        class="menu-toggle"
        type="button"
        bind:this={toggleEl}
        aria-expanded={menuOpen}
        aria-controls="site-menu"
        aria-label={menuOpen ? 'Close navigation menu' : 'Open navigation menu'}
        onclick={() => (menuOpen ? closeMenu() : openMenu())}
      >
        {#if menuOpen}
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" aria-hidden="true"><path d="M6.2 6.2 17.8 17.8" /><path d="M17.8 6.2 6.2 17.8" /></svg>
        {:else}
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" aria-hidden="true"><path d="M4 7h16" /><path d="M4 12h16" /><path d="M4 17h16" /></svg>
        {/if}
      </button>
    </div>
  </div>

  {#if menuOpen}
    <div class="mobile-menu" id="site-menu" bind:this={panelEl}>
      <div class="container mobile-menu__inner">
        <ul class="mobile-menu__list">
          <li>
            <a href="/" class="mobile-menu__link" aria-current={isHome ? 'page' : undefined} onclick={() => closeMenu()}>Home</a>
          </li>
          <li>
            <a href="/discover" class="mobile-menu__link" aria-current={isDiscover ? 'page' : undefined} onclick={() => closeMenu()}>Discover</a>
          </li>
        </ul>
        <a href="/discover" class="btn btn--primary btn--md btn--block" onclick={() => closeMenu()}>Find my tree</a>
      </div>
    </div>
  {/if}
</header>

<main class="page-main" id="main">{@render children()}</main>

<footer class="site-footer">
  <div class="container site-footer__inner">
    <div class="site-footer__lead">
      <p class="site-footer__title">Everroot</p>
      <p class="site-footer__copy">
        Everroot makes room for the sources, places, and people behind each association.
        Cultural claims appear only with evidence and context attached.
      </p>
    </div>
    <nav class="site-footer__nav" aria-label="Footer">
      <a href="/">Home</a>
      <a href="/discover">Find my tree</a>
      <a href="/trees">Tree reference</a>
    </nav>
    <p class="site-footer__note">
      Early research portal · Nothing here is a prediction, and no tree is planted without a
      verified planting partner.
    </p>
  </div>
</footer>

<style>
  .site-header {
    position: sticky;
    top: 0;
    z-index: 60;
    background: #fdfcf7;
    border-bottom: 1px solid var(--color-line-soft);
  }

  .header__inner {
    display: flex;
    align-items: center;
    gap: 1rem;
    min-height: var(--header-height);
  }

  /* Brand ------------------------------------------------------------- */
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    flex: none;
    padding: 0.35rem 0.25rem;
    border-radius: var(--radius-control);
    color: var(--color-ink);
  }

  .brand__mark {
    display: grid;
    place-items: center;
  }

  .brand__mark svg {
    width: 40px;
    height: 40px;
    fill: none;
    stroke: #4f7350;
    stroke-width: 2.1;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .brand__text {
    display: flex;
    flex-direction: column;
    gap: 0.05rem;
    line-height: 1.05;
  }

  .brand__name {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: 1.4rem;
    letter-spacing: 0.005em;
    color: #14392c;
  }

  .brand__tag {
    font-size: 0.635rem;
    font-weight: 600;
    letter-spacing: 0.155em;
    text-transform: uppercase;
    color: #74806d;
  }

  .brand:hover .brand__mark svg {
    stroke: var(--color-forest);
  }

  /* Primary nav ------------------------------------------------------- */
  .nav {
    margin-inline: auto;
  }

  .nav__list {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .nav__link {
    display: inline-flex;
    align-items: center;
    position: relative;
    padding: 0.55rem 0.85rem;
    border-radius: 10px;
    font-size: 0.97rem;
    font-weight: 650;
    color: var(--color-charcoal-soft);
    transition: color 0.15s ease, background-color 0.15s ease;
  }

  .nav__link:hover {
    color: var(--color-forest);
    background: rgba(36, 79, 61, 0.07);
  }

  .nav__link[aria-current='page'] {
    color: var(--color-forest);
  }

  .nav__link[aria-current='page']::after {
    content: '';
    position: absolute;
    left: 0.85rem;
    right: 0.85rem;
    bottom: 0.12rem;
    height: 2px;
    border-radius: 2px;
    background: var(--color-forest);
  }

  .header__actions {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex: none;
    margin-left: auto;
  }

  .menu-toggle {
    display: none;
    place-items: center;
    width: 46px;
    height: 46px;
    border-radius: var(--radius-control);
    border: 1px solid var(--color-line);
    background: var(--surface);
    color: var(--color-forest);
    cursor: pointer;
  }

  .menu-toggle:hover {
    background: #f2f4ea;
  }

  /* Mobile disclosure menu ------------------------------------------- */
  .mobile-menu {
    border-top: 1px solid var(--color-line-soft);
    background: #fdfcf7;
    animation: menuIn 0.18s ease-out;
  }

  @keyframes menuIn {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  .mobile-menu__inner {
    display: grid;
    gap: 0.75rem;
    padding-block: 0.9rem 1.15rem;
  }

  .mobile-menu__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.15rem;
  }

  .mobile-menu__link {
    display: block;
    padding: 0.8rem 0.6rem;
    border-radius: 10px;
    font-weight: 650;
    color: var(--color-charcoal-soft);
  }

  .mobile-menu__link:hover {
    background: rgba(36, 79, 61, 0.07);
    color: var(--color-forest);
  }

  .mobile-menu__link[aria-current='page'] {
    color: var(--color-forest);
    background: rgba(36, 79, 61, 0.07);
  }

  /* Main + footer ----------------------------------------------------- */
  .page-main {
    display: block;
    min-height: 60vh;
  }

  .site-footer {
    background: #eaece1;
    border-top: 1px solid #d9dccf;
    color: #677263;
    padding-block: 3rem 2.25rem;
  }

  .site-footer__inner {
    display: grid;
    gap: 1.25rem;
  }

  .site-footer__title {
    font-family: var(--font-display);
    font-size: 1.25rem;
    color: #2f4a39;
  }

  .site-footer__copy {
    max-width: 58ch;
    margin-top: 0.35rem;
    line-height: 1.7;
  }

  .site-footer__nav {
    display: flex;
    flex-wrap: wrap;
    gap: 1.25rem;
    font-weight: 650;
  }

  .site-footer__nav a {
    color: var(--color-forest);
  }

  .site-footer__nav a:hover {
    text-decoration: underline;
  }

  .site-footer__note {
    font-size: 0.82rem;
    color: #78836f;
    border-top: 1px solid #d9dccf;
    padding-top: 1.1rem;
  }

  @media (max-width: 880px) {
    .nav {
      display: none;
    }

    .header__cta {
      display: none;
    }

    .menu-toggle {
      display: grid;
    }

    .header__inner {
      justify-content: space-between;
    }
  }

  @media (max-width: 420px) {
    .brand__name {
      font-size: 1.22rem;
    }

    .brand__tag {
      font-size: 0.58rem;
      letter-spacing: 0.12em;
    }

    .brand__mark svg {
      width: 34px;
      height: 34px;
    }
  }
</style>
