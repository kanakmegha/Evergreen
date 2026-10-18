<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Icon from '../../lib/components/Icon.svelte';
  import SelectableCard from '../../lib/components/SelectableCard.svelte';
  import StepProgress from '../../lib/components/StepProgress.svelte';
  import { absoluteUrl } from '../../lib/site';
  import {
    FOLLOW_UPS,
    INTENTIONS,
    NAKSHATRA_TRADITION,
    clearFlow,
    emptyAnswers,
    fetchNakshatras,
    fetchRegions,
    fetchTraditions,
    intention as intentionInfo,
    loadFlow,
    pathFor,
    resolveIntentionId,
    saveFlow,
    saveRecord,
    uid,
  } from '../../lib/discovery';
  import type {
    DiscoveryRecord,
    FlowAnswers,
    IntentionId,
    Nakshatra,
    StepId,
    Tradition,
  } from '../../lib/discovery';

  const STEP_LABELS: Record<StepId, string> = {
    intention: 'What matters',
    followup: 'A little more',
    tradition: 'Tradition',
    nakshatra: 'Nakshatra',
  };

  let answers = $state<FlowAnswers>(emptyAnswers());
  let step = $state<StepId>('intention');
  let hydrated = $state(false);
  let heading = $state<HTMLHeadingElement | null>(null);

  let traditions = $state<Tradition[]>([]);
  let nakshatras = $state<Nakshatra[]>([]);
  let regions = $state<string[]>([]);
  let catalogState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let catalogError = $state('');

  let nakshatraQuery = $state('');
  let finishing = $state(false);

  const path = $derived(pathFor(answers));
  const currentIndex = $derived(Math.max(0, path.indexOf(step)));
  const isLastStep = $derived(currentIndex === path.length - 1);
  const progress = $derived(path.map((id) => ({ id, label: STEP_LABELS[id] })));
  const follow = $derived(answers.intention ? FOLLOW_UPS[answers.intention] ?? null : null);

  const question = $derived(
    step === 'intention'
      ? 'What would you like your tree to represent?'
      : step === 'followup'
        ? follow?.question ?? 'Tell us a little more.'
        : step === 'tradition'
          ? 'Which tradition would you like to explore?'
          : 'Which Nakshatra should we look under?'
  );

  const filteredNakshatras = $derived.by(() => {
    const q = nakshatraQuery.trim().toLowerCase();
    if (!q) return nakshatras;
    return nakshatras.filter((n) => n.name.toLowerCase().includes(q));
  });

  const canContinue = $derived.by(() => {
    if (step === 'intention') return answers.intention !== null;
    if (step === 'followup') return follow?.choices ? answers.followUpChoice !== null : true;
    if (step === 'tradition') return answers.tradition !== null;
    return true; // the nakshatra step is always skippable
  });

  const continueLabel = $derived.by(() => {
    if (step === 'nakshatra') {
      return answers.nakshatra
        ? 'See the associations on record'
        : 'Skip and see everything on record';
    }
    if (!isLastStep) return 'Continue';
    if (step === 'followup' && follow && !follow.choices) {
      const blank = !answers.followUpNote.trim() && answers.followUpPicks.length === 0;
      return blank ? 'Skip this step' : 'See my result';
    }
    return 'See my result';
  });

  /* ------------------------------------------------------------- actions */

  function chooseIntention(value: string) {
    const next = value as IntentionId;
    if (answers.intention === next) return;
    // A different intention invalidates the path-specific answers, but never the
    // words the user wrote about their own meaning.
    answers.intention = next;
    answers.followUpChoice = null;
    answers.followUpPicks = [];
    answers.tradition = null;
    answers.traditionName = '';
    answers.region = '';
    answers.nakshatra = null;
  }

  function chooseFollowUp(value: string) {
    answers.followUpChoice = value;
    if (value === 'nakshatra') {
      answers.tradition = NAKSHATRA_TRADITION;
      answers.traditionName = 'Nakshatra tree association';
    } else {
      answers.tradition = null;
      answers.traditionName = '';
      answers.region = '';
      answers.nakshatra = null;
    }
  }

  function chooseTradition(slug: string) {
    answers.tradition = slug;
    answers.traditionName = traditions.find((t) => t.slug === slug)?.name ?? slug;
    if (slug !== NAKSHATRA_TRADITION) {
      answers.region = '';
      answers.nakshatra = null;
    }
  }

  function chooseNakshatra(name: string) {
    answers.nakshatra = name;
  }

  function togglePick(label: string) {
    const picks = answers.followUpPicks;
    answers.followUpPicks = picks.includes(label)
      ? picks.filter((p) => p !== label)
      : [...picks, label];
  }

  function enter(target: StepId) {
    step = target;
    if (target === 'tradition' || target === 'nakshatra') void ensureCatalog();
    if (target === 'nakshatra') void loadRegions();
  }

  function next() {
    if (!canContinue) return;
    const list = pathFor(answers);
    const i = list.indexOf(step);
    if (i === list.length - 1) {
      void finish();
      return;
    }
    enter(list[i + 1]);
  }

  function back() {
    const list = pathFor(answers);
    const i = list.indexOf(step);
    if (i > 0) enter(list[i - 1]);
  }

  function startOver() {
    const hasProgress = Boolean(
      answers.intention || answers.followUpNote.trim() || answers.followUpPicks.length
    );
    if (
      hasProgress &&
      !window.confirm('Start over? Your current choices and any notes you have written will be cleared.')
    ) {
      return;
    }
    answers = emptyAnswers();
    step = 'intention';
    nakshatraQuery = '';
    clearFlow();
  }

  async function finish() {
    if (finishing) return;
    finishing = true;
    const record: DiscoveryRecord = {
      id: uid(),
      createdAt: new Date().toISOString(),
      answers: $state.snapshot(answers),
      discovery: null,
      error: null,
    };
    saveRecord(record);
    // Keep the answers so "change my answers" returns without data loss, but
    // reopen the journey at its first question.
    saveFlow({ answers: $state.snapshot(answers), step: 'intention' });
    await goto(`/results/${record.id}`);
  }

  /* ------------------------------------------------------------- catalog */

  async function ensureCatalog() {
    if (catalogState === 'ready' || catalogState === 'loading') return;
    catalogState = 'loading';
    catalogError = '';
    try {
      const [t, n] = await Promise.all([fetchTraditions(), fetchNakshatras()]);
      traditions = t;
      nakshatras = n;
      catalogState = 'ready';
    } catch (error) {
      catalogError =
        error instanceof Error ? error.message : 'The catalogue could not be loaded.';
      catalogState = 'error';
    }
  }

  async function reloadCatalog() {
    catalogState = 'idle';
    await ensureCatalog();
  }

  async function loadRegions() {
    const slug = answers.tradition;
    if (!slug) return;
    try {
      regions = await fetchRegions(slug);
    } catch {
      regions = [];
    }
  }

  /* ------------------------------------------------------------- lifecycle */

  onMount(() => {
    const stored = loadFlow();
    if (stored) {
      answers = stored.answers;
      step = stored.step;
    }

    const alias = resolveIntentionId(page.url.searchParams.get('intent'));
    if (alias && answers.intention !== alias) {
      answers = { ...emptyAnswers(), intention: alias };
      step = 'intention';
    }

    // Never strand the user on a step that is not part of the current path.
    const list = pathFor(answers);
    if (!list.includes(step)) step = list[list.length - 1];

    hydrated = true;
    if (step === 'tradition' || step === 'nakshatra') void ensureCatalog();
    if (step === 'nakshatra') void loadRegions();
  });

  // Persist the in-progress journey so Back, refresh and a return visit keep it.
  $effect(() => {
    if (!hydrated) return;
    saveFlow({ answers: $state.snapshot(answers), step });
  });

  // Move focus to the new question, but only on an actual step change. Arriving
  // on the page should not paint a focus ring around the heading.
  let announcedFirstStep = false;
  let lastAnnouncedStep: StepId | null = null;

  $effect(() => {
    const current = step;
    if (!hydrated) return;
    if (!announcedFirstStep) {
      announcedFirstStep = true;
      return;
    }
    if (lastAnnouncedStep === current) return;
    lastAnnouncedStep = current;
    heading?.focus({ preventScroll: true });
  });
</script>

<svelte:head>
  <title>Find your tree — Everroot</title>
  <meta
    name="description"
    content="Begin with what matters to you. Everroot asks about your intention first, then looks for reviewed tree associations with their sources attached. No account, no birth date."
  />
  <link rel="canonical" href={absoluteUrl('/discover', page.url.origin)} />
</svelte:head>

<div class="discover">
  <div class="container">
    <div class="discover__inner">
      <StepProgress steps={progress} current={currentIndex} ongo={(i) => enter(path[i])} />

      <div class="step">
        <p class="step__count">
          {path.length > 1 ? `Step ${currentIndex + 1} of ${path.length}` : `Step ${currentIndex + 1}`}
        </p>
        <h1 id="step-q" class="step__question" tabindex="-1" bind:this={heading}>{question}</h1>

        {#if step === 'intention'}
          <p class="step__intro">
            There's no wrong answer. You can change direction at any time, and nothing you choose
            here asks for a birth date, a birth time, or a birthplace.
          </p>

          <div class="choices" role="radiogroup" aria-labelledby="step-q">
            {#each INTENTIONS as item (item.id)}
              <SelectableCard
                name="intention"
                value={item.id}
                selected={answers.intention}
                label={item.label}
                description={item.description}
                icon={item.icon}
                onselect={chooseIntention}
              />
            {/each}
          </div>
        {:else if step === 'followup'}
          {#if follow}
            {#if follow.intro}<p class="step__intro">{follow.intro}</p>{/if}

            {#if follow.choices}
              <div class="choices choices--single" role="radiogroup" aria-labelledby="step-q">
                {#each follow.choices as choice (choice.value)}
                  <SelectableCard
                    name="followup"
                    value={choice.value}
                    selected={answers.followUpChoice}
                    label={choice.label}
                    description={choice.description}
                    icon={choice.icon}
                    onselect={chooseFollowUp}
                  />
                {/each}
              </div>
            {:else}
              <div class="field">
                <label for="follow-note">{follow.prompt}</label>
                <textarea
                  id="follow-note"
                  class="textarea"
                  maxlength={400}
                  bind:value={answers.followUpNote}
                  placeholder={follow.placeholder ?? ''}
                  aria-describedby="follow-hint"
                ></textarea>
                <p class="field-hint" id="follow-hint">
                  {follow.hint} {answers.followUpNote.length}/400 characters.
                </p>
              </div>

              {#if follow.quickPicks}
                <div class="picks">
                  <p class="picks__label" id="picks-label">Or start with one of these</p>
                  <div class="picks__row" role="group" aria-labelledby="picks-label">
                    {#each follow.quickPicks as pick (pick)}
                      <button
                        type="button"
                        class="pick"
                        aria-pressed={answers.followUpPicks.includes(pick)}
                        onclick={() => togglePick(pick)}
                      >
                        {#if answers.followUpPicks.includes(pick)}
                          <Icon name="check" size={14} strokeWidth={2.8} />
                        {/if}{pick}
                      </button>
                    {/each}
                  </div>
                </div>
              {/if}
            {/if}

            {#if follow.note}
              <p class="step__note">
                <Icon name="info" size={18} />
                <span>{follow.note}</span>
              </p>
            {/if}
          {/if}
        {:else if step === 'tradition'}
          <p class="step__intro">
            Only traditions that have published, reviewed records are listed here.
          </p>

          {#if catalogState === 'loading'}
            <p class="step__status" role="status">Loading the traditions on record…</p>
          {:else if catalogState === 'error'}
            <div class="notice notice--warn" role="alert">
              <p><strong>The catalogue could not be loaded.</strong> {catalogError}</p>
              <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadCatalog}>
                Try again
              </button>
            </div>
          {:else if traditions.length === 0}
            <div class="notice notice--info">
              <p>No traditions with reviewed records are published yet.</p>
            </div>
          {:else}
            <div class="choices" role="radiogroup" aria-labelledby="step-q">
              {#each traditions as item (item.slug)}
                <SelectableCard
                  name="tradition"
                  value={item.slug}
                  selected={answers.tradition}
                  label={item.name}
                  description={item.slug === NAKSHATRA_TRADITION
                    ? 'Associations between the 27 nakshatras and particular trees, recorded with sources and a review status.'
                    : 'No reviewed records are published for this tradition yet.'}
                  icon={item.slug === NAKSHATRA_TRADITION ? 'sun' : 'book'}
                  onselect={chooseTradition}
                />
              {/each}
            </div>
          {/if}
        {:else if step === 'nakshatra'}
          <p class="step__intro">
            Choose one if you know it. If you don't, skip this step — Everroot will show you
            everything else that is on record. You never need a birth date or a birth time.
          </p>

          {#if catalogState === 'loading'}
            <p class="step__status" role="status">Loading the nakshatras…</p>
          {:else if catalogState === 'error'}
            <div class="notice notice--warn" role="alert">
              <p><strong>The catalogue could not be loaded.</strong> {catalogError}</p>
              <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadCatalog}>
                Try again
              </button>
            </div>
          {:else}
            <div class="field">
              <label for="nak-search">Search the 27 nakshatras</label>
              <input
                id="nak-search"
                class="input"
                type="search"
                bind:value={nakshatraQuery}
                placeholder="Start typing a name"
                autocomplete="off"
              />
            </div>

            {#if filteredNakshatras.length === 0}
              <p class="step__status" role="status">
                No nakshatra matches “{nakshatraQuery}”. Try a different spelling, or skip this step.
              </p>
            {:else}
              <div class="nak-grid" role="radiogroup" aria-labelledby="step-q">
                {#each filteredNakshatras as item (item.name)}
                  <SelectableCard
                    dense
                    name="nakshatra"
                    value={item.name}
                    selected={answers.nakshatra}
                    label={item.name}
                    badge={String(item.ordinal)}
                    onselect={chooseNakshatra}
                  />
                {/each}
              </div>
            {/if}

            {#if regions.length > 0}
              <div class="field region">
                <label for="region-select">Region (optional)</label>
                <select id="region-select" class="select" bind:value={answers.region}>
                  <option value="">Any region</option>
                  {#each regions as region (region)}
                    <option value={region}>{region}</option>
                  {/each}
                </select>
                <p class="field-hint">
                  A region only narrows which reviewed records are shown. Leaving it blank keeps
                  everything.
                </p>
              </div>
            {/if}
          {/if}
        {/if}

        <footer class="step__actions">
          <button
            class="btn btn--ghost btn--md"
            type="button"
            onclick={back}
            disabled={currentIndex === 0}
          >
            <Icon name="arrow-left" size={18} /> Back
          </button>

          <div class="step__actions-end">
            <button class="btn btn--soft btn--md" type="button" onclick={startOver}>
              Start over
            </button>
            <button
              class="btn btn--primary btn--md"
              type="button"
              onclick={next}
              disabled={!canContinue || finishing}
            >
              {continueLabel}
              <Icon name="arrow-right" size={18} />
            </button>
          </div>
        </footer>
      </div>

      <p class="discover__foot">
        Nothing you write here is published. Personal notes stay in this browser.
      </p>
    </div>
  </div>
</div>

<style>
  .discover {
    padding-block: clamp(1.75rem, 4vw, 3rem) var(--space-section);
    background: var(--color-ivory);
    min-height: 62vh;
  }

  .discover__inner {
    max-width: 900px;
    margin-inline: auto;
    display: grid;
    gap: 1.5rem;
  }

  .step {
    background: var(--surface);
    border: 1px solid var(--color-line);
    border-radius: var(--radius-card);
    box-shadow: var(--shadow-soft);
    padding: clamp(1.35rem, 3.4vw, 2.5rem);
  }

  .step__count {
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--color-sage);
  }

  .step__question {
    margin-top: 0.5rem;
    font-size: clamp(1.45rem, 3.1vw, 2.1rem);
    line-height: 1.2;
    letter-spacing: -0.02em;
  }

  .step__question:focus-visible {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: 4px;
    border-radius: 6px;
  }

  .step__intro {
    margin-top: 0.7rem;
    color: var(--color-charcoal-soft);
    font-size: 1rem;
    line-height: 1.7;
    max-width: 62ch;
  }

  .step__note {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    margin-top: 1.4rem;
    padding: 0.9rem 1.05rem;
    background: #f6f5ec;
    border: 1px solid var(--color-line);
    border-radius: var(--radius-control);
    color: #5b6552;
    font-size: 0.92rem;
    line-height: 1.65;
  }

  .step__note :global(svg) {
    flex: none;
    margin-top: 0.12rem;
    color: var(--color-earth);
  }

  .step__status {
    margin-top: 1.25rem;
    color: var(--color-charcoal-soft);
  }

  .choices {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 310px), 1fr));
    gap: 0.7rem;
    margin-top: 1.5rem;
  }

  .choices--single {
    grid-template-columns: minmax(0, 1fr);
  }

  .field {
    margin-top: 1.5rem;
  }

  .picks {
    margin-top: 1.15rem;
  }

  .picks__label {
    font-size: 0.85rem;
    font-weight: 650;
    color: var(--color-charcoal-soft);
    margin-bottom: 0.5rem;
  }

  .picks__row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }

  .pick {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 40px;
    padding: 0.45rem 0.9rem;
    border-radius: var(--radius-pill);
    border: 1px solid var(--color-line);
    background: var(--surface);
    color: #3c4a3c;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  }

  .pick:hover {
    border-color: #bcc8b6;
    background: #f7f9f3;
  }

  .pick[aria-pressed='true'] {
    background: #e8efe4;
    border-color: var(--color-forest);
    color: var(--color-forest);
  }

  .nak-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 200px), 1fr));
    gap: 0.5rem;
    margin-top: 1.25rem;
    max-height: 24rem;
    overflow-y: auto;
    padding: 0.25rem 0.35rem 0.25rem 0.25rem;
  }

  .region {
    max-width: 24rem;
  }

  .retry {
    margin-top: 0.75rem;
  }

  .step__actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
    margin-top: 2rem;
    padding-top: 1.4rem;
    border-top: 1px solid var(--color-line-soft);
  }

  .step__actions-end {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .discover__foot {
    text-align: center;
    color: #79836f;
    font-size: 0.85rem;
  }

  @media (max-width: 620px) {
    .step__actions {
      flex-direction: column-reverse;
      align-items: stretch;
    }

    .step__actions-end {
      flex-direction: column-reverse;
      align-items: stretch;
    }

    .step__actions .btn {
      width: 100%;
    }

    .nak-grid {
      max-height: none;
      overflow: visible;
      grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr));
    }
  }
</style>
