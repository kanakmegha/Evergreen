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
    apiErrorMessage,
    clearFlow,
    emptyAnswers,
    fetchNakshatras,
    fetchNakshatrasIn,
    fetchNamingSystems,
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
    NamedNakshatra,
    NamingSystem,
    StepId,
    Tradition,
  } from '../../lib/discovery';

  const STEP_LABELS: Record<StepId, string> = {
    intention: 'What matters',
    clarification: 'Tradition context',
    followup: 'A little more',
    tradition: 'Tradition',
    culture: 'Cultural context',
    nakshatra: 'Nakshatra',
  };

  interface MotivationExample {
    id: string;
    label: string;
    icon: string;
  }

  const MOTIVATION_EXAMPLES: MotivationExample[] = [
    { id: 'culture', label: 'Culture & Heritage', icon: 'globe' },
    { id: 'spirituality', label: 'Spirituality & Faith', icon: 'book' },
    { id: 'astrology', label: 'Astrology & Zodiac', icon: 'sun' },
    { id: 'ecology', label: 'Ecology & Biodiversity', icon: 'leaf' },
    { id: 'memorial', label: 'Memorials & Remembrance', icon: 'heart' },
    { id: 'celebration', label: 'Celebrations & Milestones', icon: 'gift' },
    { id: 'personal', label: 'Personal Meaning & Values', icon: 'sprout' },
  ];

  let answers = $state<FlowAnswers>(emptyAnswers());
  let step = $state<StepId>('intention');
  let hydrated = $state(false);
  let heading = $state<HTMLHeadingElement | null>(null);

  let traditions = $state<Tradition[]>([]);
  let nakshatras = $state<Nakshatra[]>([]);
  let regions = $state<string[]>([]);

  /**
   * Each optional lookup keeps its own state, because they feed different
   * steps. Loaded together under one shared flag, a nakshatra request that
   * failed would also blank out the traditions — so a single unavailable
   * lookup would take the whole step with it. Neither list is discovery
   * itself; they only decide which options are on offer.
   */
  let traditionsState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let traditionsError = $state('');
  let nakshatrasState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let nakshatrasError = $state('');

  /**
   * The naming systems on offer, and the names served in the one that was
   * chosen. Kept apart from the canonical list above on purpose: a naming
   * system is optional context, so when it cannot be loaded the step falls
   * back to the romanised names rather than losing the whole selection.
   */
  let namingSystems = $state<NamingSystem[]>([]);
  let namingSystemsState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let namingSystemsError = $state('');
  let namingNakshatras = $state<NamedNakshatra[]>([]);
  let namingState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');

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
      : step === 'clarification'
        ? 'Which astrological or cultural tradition would you like to explore?'
        : step === 'followup'
          ? follow?.question ?? 'Tell us a little more.'
          : step === 'tradition'
            ? 'Which tradition would you like to explore?'
            : 'Which Nakshatra should we look under?'
  );

  /**
   * The visitor asked for a naming system Everroot does not hold names in.
   * They are still shown the twenty-seven — the sequence is the same in every
   * naming system — but the step has to say that the spellings are not the
   * ones they asked for, rather than quietly presenting romanised names as a
   * match.
   */
  const namingUnsupported = $derived(
    answers.naming !== '' &&
      namingSystemsState === 'ready' &&
      !namingSystems.some((system) => system.code === answers.naming)
  );

  /** The names to offer, in the chosen naming system when there is one. */
  const activeNakshatras = $derived.by<NamedNakshatra[]>(() => {
    if (answers.naming && namingNakshatras.length) return namingNakshatras;
    return nakshatras.map((n) => ({ ...n }));
  });

  const filteredNakshatras = $derived.by(() => {
    const q = nakshatraQuery.trim().toLowerCase();
    if (!q) return activeNakshatras;
    // Matching on the canonical form as well means a visitor who knows the
    // romanised name can still find the entry while reading it in their own
    // naming system.
    return activeNakshatras.filter(
      (n) => n.name.toLowerCase().includes(q) || (n.canonical ?? '').toLowerCase().includes(q)
    );
  });

  /**
   * The tradition step asks for context, not for permission to continue. The
   * list it offers comes from local reference data, so the only way it can fail
   * is the request itself — no API at the configured address, or a transport
   * error — and an optional lookup must never hold the visitor at this
   * question. The same is true when the list comes back empty. While it is
   * still loading, the question stands.
   */
  const traditionSkippable = $derived(
    traditionsState === 'error' || (traditionsState === 'ready' && traditions.length === 0)
  );

  const canContinue = $derived.by(() => {
    if (step === 'intention') {
      return (
        answers.intention !== null ||
        answers.followUpNote.trim().length > 0 ||
        (answers.motivations && answers.motivations.length > 0)
      );
    }
    if (step === 'clarification') return true;
    if (step === 'followup') return follow?.choices ? answers.followUpChoice !== null : true;
    if (step === 'tradition') return answers.tradition !== null || traditionSkippable;
    // Both cultural steps are optional. Neither asks for anything the
    // recommendation needs, so a visitor who wants only general discovery can
    // pass straight through them.
    return true;
  });

  const continueLabel = $derived.by(() => {
    if (step === 'intention' && !answers.intention && answers.followUpNote.trim().length > 0) {
      return 'Discover trees';
    }
    if (step === 'clarification') {
      return answers.astrologySystem || answers.clarification?.trim()
        ? 'Continue'
        : 'Skip cultural context';
    }
    if (step === 'culture') {
      return answers.naming || answers.tradition ? 'Continue' : 'Skip cultural context';
    }
    if (step === 'nakshatra') {
      return answers.nakshatra
        ? 'See the associations on record'
        : 'Skip and see everything on record';
    }
    // Nothing was offered, so the button says what pressing it actually does
    // rather than implying a choice was made.
    if (step === 'tradition' && traditionSkippable && !answers.tradition) {
      return 'Skip and see my result';
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
    answers.naming = '';
    answers.nakshatra = null;
    answers.nakshatraName = '';
  }

  function chooseFollowUp(value: string) {
    answers.followUpChoice = value;
    if (value !== 'nakshatra') {
      answers.tradition = null;
      answers.traditionName = '';
      answers.region = '';
      answers.naming = '';
      answers.nakshatra = null;
      answers.nakshatraName = '';
      return;
    }
    // Wanting a nakshatra is not an answer about which tradition it should be
    // read in. It used to settle the tradition here, which is how the next
    // step ended up showing one tradition's names to everybody. The cultural
    // step now asks, and nothing is assumed from the visitor's other answers.
  }

  function chooseTradition(slug: string) {
    answers.tradition = slug;
    answers.traditionName = traditions.find((t) => t.slug === slug)?.name ?? slug;
    if (slug !== NAKSHATRA_TRADITION) {
      answers.region = '';
      answers.naming = '';
      answers.nakshatra = null;
      answers.nakshatraName = '';
    }
  }

  /**
   * `value` is the canonical name, so a selection made in any naming system
   * still travels as the name the API and the reviewed records know. The
   * spelling that was on the button is kept alongside it, so the result page
   * can repeat the choice back the way the visitor made it.
   */
  function chooseNakshatra(value: string) {
    const chosen = activeNakshatras.find((n) => (n.canonical ?? n.name) === value);
    answers.nakshatra = value;
    answers.nakshatraName = chosen?.name ?? value;
  }

  function chooseNaming(code: string) {
    answers.naming = code;
    // A nakshatra picked under one naming system is not the same choice
    // spelled another way, so the selection is cleared rather than silently
    // reinterpreted as a different name.
    answers.nakshatra = null;
    answers.nakshatraName = '';
    nakshatraQuery = '';
    void loadNamingNakshatras(code);
  }

  function togglePick(label: string) {
    const picks = answers.followUpPicks;
    answers.followUpPicks = picks.includes(label)
      ? picks.filter((p) => p !== label)
      : [...picks, label];
  }

  function toggleMotivation(id: string) {
    const current = answers.motivations ?? [];
    answers.motivations = current.includes(id)
      ? current.filter((m) => m !== id)
      : [...current, id];
  }

  function chooseAstrology(value: string) {
    answers.astrologySystem = value as FlowAnswers['astrologySystem'];
    if (value === 'western') {
      answers.tradition = 'Western zodiac (default)';
      answers.traditionName = 'Western zodiac (default)';
    } else if (value === 'vedic') {
      answers.tradition = 'Indian / Vedic astrology';
      answers.traditionName = 'Indian / Vedic astrology (Nakshatras)';
    } else if (value === 'tamil') {
      answers.tradition = 'Tamil astrological tradition';
      answers.traditionName = 'Tamil astrological tradition';
    } else if (value === 'explore' || value === 'none') {
      answers.tradition = null;
      answers.traditionName = '';
    }
  }

  function enter(target: StepId) {
    step = target;
    if (target === 'tradition' || target === 'culture' || target === 'nakshatra') {
      void ensureCatalog();
    }
    if (target === 'culture') void loadNamingSystems();
    if (target === 'nakshatra') {
      void loadRegions();
      // Re-entering with a naming system already chosen still needs its names.
      if (answers.naming) void loadNamingNakshatras(answers.naming);
    }
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
    if (!answers.intention) {
      answers.intention = 'explore';
    }
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

  /**
   * Load one optional list, keeping any failure to itself.
   *
   * The wording distinguishes a 404 (no API at the configured address), a 503
   * (the API answered but this feature is unavailable) and a transport or
   * configuration failure. It never says discovery is unavailable, because it
   * never is: a missing list changes which options are offered, not whether a
   * recommendation can be made.
   */
  async function loadTraditions() {
    if (traditionsState === 'ready' || traditionsState === 'loading') return;
    traditionsState = 'loading';
    traditionsError = '';
    try {
      traditions = await fetchTraditions();
      traditionsState = 'ready';
    } catch (error) {
      traditionsError = apiErrorMessage(error);
      traditionsState = 'error';
    }
  }

  async function loadNakshatras() {
    if (nakshatrasState === 'ready' || nakshatrasState === 'loading') return;
    nakshatrasState = 'loading';
    nakshatrasError = '';
    try {
      nakshatras = await fetchNakshatras();
      nakshatrasState = 'ready';
    } catch (error) {
      nakshatrasError = apiErrorMessage(error);
      nakshatrasState = 'error';
    }
  }

  /** Both lookups, started together and answered independently. */
  function ensureCatalog() {
    void loadTraditions();
    void loadNakshatras();
  }

  async function reloadTraditions() {
    traditionsState = 'idle';
    await loadTraditions();
  }

  async function reloadNakshatras() {
    nakshatrasState = 'idle';
    await loadNakshatras();
  }

  // The naming systems decide which spellings are offered, so like the
  // tradition list they are reference vocabulary rather than a gate: a failure
  // is reported on the step that shows them and never stops the journey.
  async function loadNamingSystems() {
    if (namingSystemsState === 'ready' || namingSystemsState === 'loading') return;
    namingSystemsState = 'loading';
    namingSystemsError = '';
    try {
      namingSystems = await fetchNamingSystems();
      namingSystemsState = 'ready';
    } catch (error) {
      namingSystemsError = apiErrorMessage(error);
      namingSystemsState = 'error';
    }
  }

  async function reloadNamingSystems() {
    namingSystemsState = 'idle';
    await loadNamingSystems();
  }

  /**
   * The names in one naming system. A failure leaves the romanised list in
   * place — the visitor can still pick a nakshatra, they just see it under the
   * spelling Everroot already had.
   */
  async function loadNamingNakshatras(code: string) {
    if (!code) {
      namingNakshatras = [];
      namingState = 'idle';
      return;
    }
    if (namingState === 'loading') return;
    namingState = 'loading';
    try {
      namingNakshatras = await fetchNakshatrasIn(code);
      namingState = 'ready';
    } catch {
      namingNakshatras = [];
      namingState = 'error';
    }
  }

  // The region list only narrows the query and is optional, so a failure is
  // shown as "any region" rather than as an error. `regionsFor` remembers which
  // tradition the list belongs to so re-entering this step (Back, forward, a
  // progress click) does not repeat the request. A failure is not remembered,
  // so returning to the step retries instead of hiding an outage all visit.
  let regionsFor = '';

  async function loadRegions() {
    const slug = answers.tradition;
    if (!slug || regionsFor === slug) return;
    try {
      regions = await fetchRegions(slug);
      regionsFor = slug;
    } catch {
      regions = [];
      regionsFor = '';
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
    if (step === 'tradition' || step === 'culture' || step === 'nakshatra') void ensureCatalog();
    if (step === 'culture' || step === 'nakshatra') void loadNamingSystems();
    if (step === 'nakshatra') void loadRegions();
    // A journey restored mid-way may already have chosen a naming system.
    if (answers.naming) void loadNamingNakshatras(answers.naming);
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
            Everroot discovers trees based on whatever is meaningful to you: culture, folklore,
            spirituality, loved ones, ecology, or personal values. You can describe your intention in
            your own words, explore along any path below, or combine them. These paths are examples,
            not the limits of what Everroot can research.
          </p>

          <div class="field">
            <label for="intent-free-text" class="picks__label">
              Describe in your own words what you are seeking (optional)
            </label>
            <textarea
              id="intent-free-text"
              class="textarea"
              maxlength={600}
              bind:value={answers.followUpNote}
              placeholder="e.g. A memorial tree for my grandmother who loved mountain rivers in Kyoto, something native and bird-friendly with autumn color..."
            ></textarea>
            <p class="field-hint">
              Feel free to mention traditions, loved ones, environmental goals, or personal milestones.
            </p>
          </div>

          <p class="picks__label" style="margin-top: 1rem;">
            Or explore by theme or motivation (examples to start from, not limits):
          </p>
          <div class="picks__row" role="group" aria-label="Motivations">
            {#each MOTIVATION_EXAMPLES as mot (mot.id)}
              <button
                type="button"
                class="pick"
                aria-pressed={answers.motivations.includes(mot.id)}
                onclick={() => toggleMotivation(mot.id)}
              >
                {#if answers.motivations.includes(mot.id)}
                  <Icon name="check" size={14} strokeWidth={2.8} />
                {/if}
                {mot.label}
              </button>
            {/each}
          </div>

          <p class="picks__label" style="margin-top: 1.5rem;">
            Or choose a discovery path (examples to start from):
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

          <div class="field" style="margin-top: 1.5rem;">
            <details class="context-box">
              <summary class="context-box__summary">
                <strong>Optional context:</strong> planting region, soil, or cultural tradition
              </summary>
              <div class="context-box__body">
                <div class="field">
                  <label for="planting-region">Region or planting environment (optional)</label>
                  <input
                    id="planting-region"
                    type="text"
                    class="input"
                    bind:value={answers.region}
                    placeholder="e.g. Pacific Northwest, Mediterranean dry summer, clay soil, small garden"
                  />
                </div>
                <div class="field">
                  <label for="cultural-tradition">Cultural, spiritual, or regional context (optional)</label>
                  <input
                    id="cultural-tradition"
                    type="text"
                    class="input"
                    bind:value={answers.traditionName}
                    oninput={(e) => {
                      const val = (e.target as HTMLInputElement).value;
                      answers.tradition = val.trim() ? val : null;
                      answers.traditionName = val;
                      answers.customCulture = val.trim();
                    }}
                    placeholder="e.g. Celtic folklore, Yoruba, Shinto, Vedic, local community tradition"
                  />
                </div>
              </div>
            </details>
          </div>
        {:else if step === 'clarification'}
          <p class="step__intro">
            Astrology and cultural tree associations exist across many world traditions. Choose the
            tradition that best matches your question, specify another, or continue with the Western
            zodiac default. Everroot will dynamically research the relevant lore and botany.
          </p>

          <div class="choices" role="radiogroup" aria-labelledby="step-q">
            <SelectableCard
              name="astrology-system"
              value="western"
              selected={answers.astrologySystem ?? null}
              label="Western zodiac (default)"
              description="Tropical zodiac signs (Aries through Pisces) and classical botanical correspondences."
              icon="sun"
              onselect={(val) => chooseAstrology(val)}
            />
            <SelectableCard
              name="astrology-system"
              value="vedic"
              selected={answers.astrologySystem ?? null}
              label="Indian / Vedic astrology"
              description="The 27 lunar Nakshatras and 12 Rāśis rooted in Vedic astronomy and cultural lore."
              icon="sun"
              onselect={(val) => chooseAstrology(val)}
            />
            <SelectableCard
              name="astrology-system"
              value="tamil"
              selected={answers.astrologySystem ?? null}
              label="Tamil astrological tradition"
              description="Traditional tree, star, and temple grove associations documented in Tamil regional heritage."
              icon="leaf"
              onselect={(val) => chooseAstrology(val)}
            />
            <SelectableCard
              name="astrology-system"
              value="other"
              selected={answers.astrologySystem ?? null}
              label="Another cultural tradition"
              description="Celtic tree astrology, Chinese lunar calendar, or any cultural tradition in your own words."
              icon="globe"
              onselect={(val) => chooseAstrology(val)}
            />
            <SelectableCard
              name="astrology-system"
              value="explore"
              selected={answers.astrologySystem ?? null}
              label="Help me explore"
              description="Explore trees with an open, general approach without committing to one specific system."
              icon="compass"
              onselect={(val) => chooseAstrology(val)}
            />
          </div>

          {#if answers.astrologySystem === 'other'}
            <div class="field" style="margin-top: 1rem;">
              <label for="custom-clarification">Specify your tradition or astrological system</label>
              <input
                id="custom-clarification"
                type="text"
                class="input"
                bind:value={answers.clarification}
                placeholder="e.g. Celtic tree astrology, Mayan calendar, or personal tradition"
              />
            </div>
          {/if}

          <div style="margin-top: 1rem;">
            <button
              class="btn btn--soft btn--sm"
              type="button"
              onclick={() => {
                answers.astrologySystem = 'none';
                next();
              }}
            >
              Skip cultural context / Continue with general approach
            </button>
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
            A tradition is cultural context for your answer, not a requirement. Choose one if it
            helps explain what the tree should mean — or skip it entirely.
          </p>

          {#if traditionsState === 'loading'}
            <p class="step__status" role="status">Loading the traditions…</p>
          {:else if traditionsState === 'error'}
            <div class="notice notice--warn" role="alert">
              <p><strong>The list of traditions could not be loaded.</strong> {traditionsError}</p>
              <p class="notice__hint">
                This is only the list of traditions — your recommendation does not depend on it.
                You can continue without choosing a tradition and Everroot will work from your
                answers.
              </p>
              <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadTraditions}>
                Try again
              </button>
            </div>
          {:else if traditions.length === 0}
            <div class="notice notice--info">
              <p>No traditions with reviewed records are published yet.</p>
              <p class="notice__hint">You can continue without choosing one.</p>
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
        {:else if step === 'culture'}
          <p class="step__intro">
            Nothing here is assumed about you — not from your name, not from where you are, and not
            from the answers you have given. Choose a tradition and a naming system if they help
            explain what the tree should mean, or skip this and Everroot will suggest from your
            answers alone.
          </p>

          {#if !answers.tradition}
            <!--
              No tradition has been established yet, so it is asked for here rather
              than settled behind the visitor's back. An earlier version of this flow
              jumped straight from "I want a nakshatra" to a list of names in one
              tradition's spelling, which made the question of whose tradition it was
              unanswerable by the time it was asked.
            -->
            <h3 class="group__title" id="culture-tradition">Tradition or community</h3>

            {#if traditionsState === 'loading'}
              <p class="step__status" role="status">Loading the traditions…</p>
            {:else if traditionsState === 'error'}
              <div class="notice notice--warn" role="alert">
                <p><strong>The list of traditions could not be loaded.</strong> {traditionsError}</p>
                <p class="notice__hint">
                  This is only the list of traditions — your recommendation does not depend on it.
                  You can continue without choosing a tradition and Everroot will work from your
                  answers.
                </p>
                <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadTraditions}>
                  Try again
                </button>
              </div>
            {:else}
              <div class="choices choices--tight" role="radiogroup" aria-labelledby="culture-tradition">
                {#each traditions as item (item.slug)}
                  <SelectableCard
                    name="culture-tradition"
                    value={item.slug}
                    selected={answers.tradition}
                    label={item.name}
                    description={item.slug === NAKSHATRA_TRADITION
                      ? 'The 27 nakshatras and the trees recorded against them.'
                      : 'Everroot holds no names in this tradition yet.'}
                    icon={item.slug === NAKSHATRA_TRADITION ? 'sun' : 'book'}
                    onselect={chooseTradition}
                  />
                {/each}
              </div>
            {/if}
          {/if}

          <h3 class="group__title" id="culture-naming">Naming system</h3>
          <p class="group__hint">
            Which names for the 27 nakshatras do you recognise? Leave this unset to see the
            romanised names.
          </p>

          {#if namingSystemsState === 'loading'}
            <p class="step__status" role="status">Loading the naming systems…</p>
          {:else if namingSystemsState === 'error'}
            <div class="notice notice--warn" role="alert">
              <p><strong>The naming systems could not be loaded.</strong> {namingSystemsError}</p>
              <p class="notice__hint">
                This only decides which spellings are shown. Continue and Everroot will use the
                romanised names.
              </p>
              <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadNamingSystems}>
                Try again
              </button>
            </div>
          {:else}
            <div class="choices choices--tight" role="radiogroup" aria-labelledby="culture-naming">
              {#each namingSystems as system (system.code)}
                <SelectableCard
                  name="culture-naming"
                  value={system.code}
                  selected={answers.naming}
                  label={system.name}
                  description={system.endonym}
                  onselect={chooseNaming}
                />
              {/each}
            </div>
          {/if}

          {#if namingUnsupported}
            <!--
              Everroot was asked for a naming system it holds no names in. The 27 are
              the same wherever you go, so the list is still shown — but as romanised
              names, and said to be that, rather than presented as the ones that were
              asked for.
            -->
            <div class="notice notice--info" role="status">
              <p>Everroot has no names in that naming system yet.</p>
              <p class="notice__hint">
                The 27 are the same in every naming system, so the list below is in romanised
                Sanskrit. Anything cultural the model says about them is written rather than
                sourced, and may need confirming.
              </p>
            </div>
          {/if}
        {:else if step === 'nakshatra'}
          <p class="step__intro">
            Choose one if you know it. If you don't, skip this step — Everroot will suggest trees
            from your answers alone. You never need a birth date or a birth time.
          </p>

          {#if answers.naming}
            <p class="group__hint">
              Showing the names in {namingSystems.find((s) => s.code === answers.naming)?.name ??
                'your chosen naming system'}.
            </p>
          {/if}

          {#if nakshatrasState === 'loading'}
            <p class="step__status" role="status">Loading the nakshatras…</p>
          {:else if nakshatrasState === 'error'}
            <div class="notice notice--warn" role="alert">
              <p><strong>The list of nakshatras could not be loaded.</strong> {nakshatrasError}</p>
              <p class="notice__hint">
                This is only the nakshatra list — it is optional, and your recommendation does not
                depend on it. Skip this step and Everroot will suggest a tree from your answers
                alone. You never need a birth date or a birth time.
              </p>
              <button class="btn btn--soft btn--sm retry" type="button" onclick={reloadNakshatras}>
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
                {#each filteredNakshatras as item ((item.canonical ?? item.name))}
                  <SelectableCard
                    dense
                    name="nakshatra"
                    value={item.canonical ?? item.name}
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

  /* The line under a catalogue warning that says the visitor can still move on. */
  .notice__hint {
    margin-top: 0.5rem;
  }

  .choices {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 310px), 1fr));
    gap: 0.7rem;
    margin-top: 1.5rem;
  }

  /* The cultural step asks two questions on one screen, so each needs a name
     of its own. Kept to the same scale and colour as the step's other text
     rather than introducing a heading level the flow has not used. */
  .group__title {
    margin: 1.4rem 0 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--color-charcoal, #2f3a30);
  }

  .group__hint {
    margin: 0.3rem 0 0;
    max-width: 62ch;
    color: var(--color-charcoal-soft);
    font-size: 0.92rem;
    line-height: 1.6;
  }

  .choices--tight {
    margin-top: 0.7rem;
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

  .context-box {
    border: 1px solid var(--color-line-soft);
    border-radius: var(--radius-md);
    background: #fbfcf9;
    padding: 0.85rem 1.15rem;
  }

  .context-box__summary {
    cursor: pointer;
    font-size: 0.92rem;
    color: var(--color-charcoal);
    user-select: none;
  }

  .context-box__body {
    margin-top: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
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
