<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Icon from '../../../lib/components/Icon.svelte';
  import TreeOption from '../../../lib/components/TreeOption.svelte';
  import { referenceOption, reviewedOption } from '../../../lib/treeOption';
  import { filterTrees } from '../../../lib/treeOption';
  import type { TreeOptionView } from '../../../lib/treeOption';
  import {
    clearFlow,
    discoveryRequest,
    emptyAnswers,
    explanationFailureText,
    fetchCatalogue,
    fetchSession,
    followUpOptionLabel,
    intention as intentionInfo,
    loadDedication,
    loadRecord,
    loadSaved,
    personalReflection,
    removeDedication,
    resolveIntentionId,
    runDiscovery,
    runExplain,
    saveDedication,
    saveRecord,
    toggleSaved,
    usesNakshatra,
  } from '../../../lib/discovery';
  import type {
    DiscoveryRecord,
    DiscoveryRequestInput,
    EvidenceStatus,
    ExplanationEnvelope,
    TreeFact,
  } from '../../../lib/discovery';

  /** The shortlist never grows beyond three options. */
  const SHORTLIST_SIZE = 3;

  let status = $state<'loading' | 'ready' | 'not-found'>('loading');
  let record = $state<DiscoveryRecord | null>(null);
  /** True when the summary was rebuilt from an earlier conversation session. */
  let reconstructed = $state(false);
  let retrying = $state(false);

  let saved = $state(loadSaved());
  let saveStatus = $state('');

  let dedicationText = $state('');
  let dedicationSavedAt = $state<string | null>(null);
  let dedicationStatus = $state('');
  /* Secondary sections start collapsed so the shortlist is what loads first. */
  let meaningOpen = $state(false);
  let dedicationOpen = $state(false);

  /* ------------------------------------------- exploratory tree discovery */

  /**
   * The provider state for the one selection-level explanation on the page.
   * Kept separate from the cultural result on purpose: a provider failure is
   * never reported as missing cultural data.
   */
  let exploration = $state<ExplanationEnvelope | null>(null);
  let explorationEvidence = $state<EvidenceStatus | null>(null);
  let explorationTrees = $state<TreeFact[]>([]);
  let explorationLoading = $state(false);
  let explorationNotice = $state('');

  let catalogue = $state<TreeFact[]>([]);
  let catalogueOrdering = $state('');
  let catalogueLoading = $state(false);
  let catalogueNotice = $state('');
  let treeSearch = $state('');
  /** The catalogue is closed by default; it must never dominate the results. */
  let catalogueOpen = $state(false);
  /** Which catalogue row is expanded. */
  let openRowId = $state('');
  /** Which shortlist option has its "Why this option?" detail open. */
  let openOptionKey = $state('');

  /* ------------------------------------------------------------- derived */

  const answers = $derived(record?.answers ?? emptyAnswers());
  const chosenIntention = $derived(intentionInfo(answers.intention));
  const queryable = $derived(discoveryRequest(answers) !== null);
  const needsNakshatra = $derived(usesNakshatra(answers) && !answers.nakshatra);
  const recommendations = $derived(record?.discovery?.recommendations ?? []);
  const hasResults = $derived(
    record?.discovery?.status === 'verified_results' && recommendations.length > 0
  );
  const followUpLabel = $derived(followUpOptionLabel(answers.intention, answers.followUpChoice));
  const showTraditionFacts = $derived(Boolean(answers.tradition));
  const showNakshatraFact = $derived(Boolean(answers.nakshatra) || usesNakshatra(answers));

  /** The cultural-association state, as the server's own publication gate reported it. */
  const culturalAssociation = $derived(
    hasResults ? 'reviewed_match' : (explorationEvidence?.culturalAssociation ?? '')
  );

  /**
   * The botanical records the shortlist is drawn from. The explanation endpoint
   * returns the same three records it was given, so the shortlist still renders
   * when the provider is switched off or unreachable.
   */
  const referenceTrees = $derived(
    explorationTrees.length ? explorationTrees : catalogue.slice(0, SHORTLIST_SIZE)
  );

  const primaryTreeName = $derived(
    recommendations[0]?.botanicalCandidates?.[0]?.scientificName ??
      referenceTrees[0]?.acceptedName ??
      ''
  );

  /** No verified cultural match: the botanical reference is the answer, not a fallback. */
  const exploratory = $derived(
    status === 'ready' && Boolean(record) && !record?.error && !hasResults
  );

  const shortlist = $derived.by<TreeOptionView[]>(() => {
    if (hasResults) {
      return recommendations.slice(0, SHORTLIST_SIZE).map((rec, index) => reviewedOption(rec, index));
    }
    return referenceTrees
      .slice(0, SHORTLIST_SIZE)
      .map((tree, index) => referenceOption(tree, index, 'shortlist', true, culturalAssociation));
  });

  const headline = $derived(
    hasResults
      ? recommendations.length === 1
        ? 'Your reviewed association'
        : 'Your reviewed associations'
      : 'Your trees to explore'
  );

  /**
   * One sentence describing what this page actually contains. When there is no
   * reviewed cultural match it is also the single notice saying so — the same
   * warning is not repeated anywhere else on the page.
   */
  const lede = $derived.by(() => {
    if (hasResults) {
      return recommendations.length === 1
        ? 'One association is on record for this selection, with the sources it rests on.'
        : `${recommendations.length} associations are on record for this selection, each with the sources it rests on.`;
    }
    if (!record || record.error) return '';
    if (needsNakshatra) {
      return 'You skipped the Nakshatra step, so there is no cultural record to check. These are botanical explorations—not Nakshatra recommendations.';
    }
    if (!queryable) {
      return 'Your answers are not part of Everroot’s reviewed cultural dataset, so there is nothing cultural to verify. These are botanical explorations—not Nakshatra recommendations.';
    }
    return 'We couldn’t verify a cultural association for this exact selection, so these are botanical explorations—not Nakshatra recommendations.';
  });

  const ledeIsNotice = $derived(!hasResults);

  /** A discreet indicator of what backs the page, not a promise about the trees. */
  const evidencePill = $derived.by<{ text: string; tone: 'ok' | 'none' } | null>(() => {
    if (hasResults) return { text: 'Reviewed association on record', tone: 'ok' };
    if (!culturalAssociation) return null;
    if (culturalAssociation === 'reviewed_match') return { text: 'Reviewed record exists', tone: 'ok' };
    if (culturalAssociation === 'no_reviewed_match') {
      return { text: 'No verified cultural match', tone: 'none' };
    }
    return { text: 'Botanical records only', tone: 'none' };
  });

  const shortlistNote = $derived.by(() => {
    if (hasResults || !exploratory) return null;
    return {
      label: 'Exploration summary',
      loading: explorationLoading && !explorationNotice,
      text: exploration?.status === 'ok' ? exploration.text : '',
      notice: explorationNotice,
    };
  });

  /** Client-side narrowing of the already-loaded reference. */
  const filteredCatalogue = $derived(filterTrees(catalogue, treeSearch));

  const shortlistSpecies = $derived(
    new Set(referenceTrees.slice(0, SHORTLIST_SIZE).map((tree) => tree.speciesId))
  );

  const catalogueAvailable = $derived(
    catalogueLoading ? null : catalogueNotice ? 0 : catalogue.length
  );

  /* --------------------------------------------------------------- load */

  async function load() {
    status = 'loading';
    const id = page.params.sessionId ?? '';
    if (!id) {
      status = 'not-found';
      return;
    }

    let loaded = loadRecord(id);

    if (!loaded) {
      // Read-only fallback for result links created by the earlier conversation flow.
      const session = await fetchSession(id);
      if (!session) {
        status = 'not-found';
        return;
      }
      loaded = {
        id,
        createdAt: session.expires_at ?? new Date().toISOString(),
        answers: {
          ...emptyAnswers(),
          intention: resolveIntentionId(session.intent ?? null),
          tradition: session.tradition_slug ?? null,
          traditionName: '',
          region: session.region_name ?? '',
          nakshatra: session.nakshatra ?? null,
        },
        discovery: null,
        error: null,
      };
      reconstructed = true;
    }

    record = loaded;
    applyDedication(loaded.id);
    status = 'ready';
    if (page.url.hash === '#dedication') dedicationOpen = true;

    const request = discoveryRequest(loaded.answers);
    if (request && !loaded.discovery && !loaded.error) {
      await query(request);
    }
    // The reference is always loaded, so its count is honest in both paths.
    void loadCatalogue();
    if (!hasResults && !record?.error) {
      void loadExploration();
    }
  }

  async function query(request: DiscoveryRequestInput) {
    if (!record) return;
    try {
      record.discovery = await runDiscovery(request);
      record.error = null;
    } catch (error) {
      record.discovery = null;
      record.error =
        error instanceof Error ? error.message : 'The discovery service could not be reached.';
    }
    saveRecord($state.snapshot(record));
  }

  async function retry() {
    if (!record) return;
    const request = discoveryRequest(record.answers);
    if (!request) return;
    retrying = true;
    record.error = null;
    await query(request);
    retrying = false;
    void loadCatalogue();
    if (!hasResults && !record.error) void loadExploration();
  }

  /* --------------------------------------------- exploratory discovery */

  /**
   * The single provider call on this page. It returns the shortlist records and
   * the cultural-association state whatever the provider does, so a provider
   * failure never empties the shortlist.
   */
  async function loadExploration() {
    if (!record || explorationLoading) return;
    explorationLoading = true;
    explorationNotice = '';
    try {
      const response = await runExplain({
        intention: answers.intention ?? 'explore',
        tradition: answers.tradition ?? '',
        nakshatra: answers.nakshatra ?? '',
        region: answers.region ?? '',
        speciesId: '',
      });
      explorationEvidence = response.evidence;
      explorationTrees = response.trees;
      exploration = response.explanation;
      if (response.explanation.status !== 'ok') {
        explorationNotice = explanationFailureText(response.explanation.reason);
      }
    } catch (error) {
      exploration = null;
      explorationTrees = [];
      explorationNotice =
        error instanceof Error
          ? `${error.message} The botanical records below are unaffected.`
          : 'The explanation service could not be reached. The botanical records below are unaffected.';
    } finally {
      explorationLoading = false;
    }
  }

  async function loadCatalogue() {
    if (catalogueLoading) return;
    catalogueLoading = true;
    catalogueNotice = '';
    try {
      const response = await fetchCatalogue();
      catalogue = response.trees;
      catalogueOrdering = response.ordering;
    } catch (error) {
      catalogue = [];
      catalogueNotice =
        error instanceof Error ? error.message : 'The botanical reference could not be reached.';
    } finally {
      catalogueLoading = false;
    }
  }

  function applyDedication(id: string) {
    const existing = loadDedication(id);
    dedicationText = existing?.text ?? '';
    dedicationSavedAt = existing?.updatedAt ?? null;
    dedicationStatus = '';
  }

  onMount(load);

  /* ------------------------------------------------------------ actions */

  function toggleOption(key: string) {
    openOptionKey = openOptionKey === key ? '' : key;
  }

  function isSpeciesSaved(tree: TreeFact): boolean {
    return saved.some((entry) => entry.key === `species:${tree.speciesId}`);
  }

  function saveOption(option: TreeOptionView) {
    if (!record) return;
    const tree = option.tree;
    if (!tree) return;
    const key = `species:${tree.speciesId}`;
    const wasSaved = saved.some((entry) => entry.key === key);
    toggleSaved({ key, label: tree.acceptedName, scientificName: tree.acceptedName, recordId: record.id });
    // Re-read so the UI reflects what actually persisted.
    saved = loadSaved();
    if (isSpeciesSaved(tree)) {
      saveStatus = `“${tree.acceptedName}” is saved in this browser only. Everroot does not publish or plant anything.`;
    } else if (wasSaved) {
      saveStatus = `“${tree.acceptedName}” was removed from this browser.`;
    } else {
      saveStatus =
        'This browser blocked local storage, so nothing could be saved. Private browsing can cause this.';
    }
  }

  function commitDedication() {
    if (!record) return;
    const text = dedicationText.trim();
    if (!text) return;
    if (!saveDedication(record.id, text)) {
      dedicationStatus = 'This browser blocked local storage, so the dedication could not be saved.';
      return;
    }
    dedicationSavedAt = new Date().toISOString();
    dedicationStatus = 'Saved in this browser only. It has not been published or shared.';
  }

  function clearDedication() {
    if (!record) return;
    removeDedication(record.id);
    dedicationText = '';
    dedicationSavedAt = null;
    dedicationStatus = 'Removed from this browser.';
  }

  async function startOver() {
    clearFlow();
    await goto('/discover');
  }

  /* ------------------------------------------------- option construction */

  /** Catalogue rows reuse the same card, built from the same record. */
  function catalogueOption(tree: TreeFact, index: number): TreeOptionView {
    return referenceOption(
      tree,
      index,
      'catalogue',
      shortlistSpecies.has(tree.speciesId),
      culturalAssociation
    );
  }
</script>

<svelte:head>
  <title>Your discovery — Everroot</title>
  <meta name="robots" content="noindex, nofollow" />
  <meta
    name="description"
    content="The trees, sources and review statuses Everroot found for your discovery."
  />
</svelte:head>

<div class="result">
  <div class="container">
    <div class="result__inner">
      {#if status === 'loading'}
        <div class="state card-surface" role="status">
          <span class="spinner" aria-hidden="true"></span>
          <p>Checking the reviewed records…</p>
        </div>
      {:else if status === 'not-found'}
        <div class="state card-surface">
          <span class="state__icon" aria-hidden="true"><Icon name="info" size={26} /></span>
          <h1 class="state__title">This result link is not available</h1>
          <p class="state__body">
            Everroot keeps discovery sessions for a short time, and new results are stored in your
            own browser rather than on the server. This link no longer has anything behind it.
          </p>
          <a class="btn btn--primary btn--md" href="/discover">Find my tree again</a>
        </div>
      {:else if record}
        {#if reconstructed}
          <p class="notice notice--info banner" role="status">
            <Icon name="info" size={18} />
            <span>
              This link came from an earlier Everroot session. The summary below was rebuilt from
              that saved session.
            </span>
          </p>
        {/if}

        <!-- ----------------------------------------------- A. summary -->
        <section class="summary card-surface" aria-labelledby="summary-title">
          <div class="summary__top">
            <div>
              <p class="eyebrow">Your discovery</p>
              <h1 id="summary-title" class="summary__title">{headline}</h1>
            </div>
            {#if evidencePill}
              <p class="pill" class:pill--ok={evidencePill.tone === 'ok'}>
                <span class="pill__dot" aria-hidden="true"></span>{evidencePill.text}
              </p>
            {/if}
          </div>

          {#if lede}
            <p class="summary__lede" class:summary__lede--notice={ledeIsNotice}>
              {#if ledeIsNotice}<Icon name="info" size={17} />{/if}
              <span>{lede}</span>
            </p>
          {/if}

          <dl class="facts">
            <div class="fact">
              <dt>What matters to you</dt>
              <dd>{chosenIntention?.label ?? 'Not specified'}</dd>
            </div>
            {#if followUpLabel}
              <div class="fact">
                <dt>A little more</dt>
                <dd>{followUpLabel}</dd>
              </div>
            {/if}
            {#if showTraditionFacts}
              <div class="fact">
                <dt>Tradition</dt>
                <dd>{answers.traditionName || answers.tradition}</dd>
              </div>
            {/if}
            {#if showNakshatraFact}
              <div class="fact">
                <dt>Nakshatra</dt>
                <dd>{answers.nakshatra ?? 'Skipped'}</dd>
              </div>
            {/if}
            {#if showTraditionFacts}
              <div class="fact">
                <dt>Region</dt>
                <dd>{answers.region || 'Any region'}</dd>
              </div>
            {/if}
          </dl>
        </section>

        {#if record.error}
          <div class="notice notice--error banner" role="alert">
            <Icon name="alert" size={18} />
            <span>
              <strong>The reviewed records could not be checked.</strong>
              {record.error} Nothing has been guessed in the meantime.
            </span>
            <button
              class="btn btn--soft btn--sm"
              type="button"
              onclick={retry}
              disabled={retrying}
            >
              {retrying ? 'Trying again…' : 'Try again'}
            </button>
          </div>
        {/if}

        <!-- ----------------------------------------- B. the shortlist -->
        {#if shortlist.length}
          <section class="shortlist" aria-labelledby="shortlist-title">
            <div class="shortlist__head">
              <h2 id="shortlist-title" class="shortlist__title">
                {hasResults
                  ? recommendations.length === 1
                    ? 'The association on record'
                    : 'The associations on record'
                  : 'Start with these'}
              </h2>
              {#if !hasResults && shortlist.length < SHORTLIST_SIZE}
                <p class="shortlist__count">
                  {catalogue.length}
                  {catalogue.length === 1 ? 'tree is' : 'trees are'} in Everroot’s botanical
                  reference, so these are all of them.
                </p>
              {/if}
            </div>

            {#if shortlistNote && (shortlistNote.loading || shortlistNote.text || shortlistNote.notice)}
              <div class="note card-surface" aria-live="polite">
                <p class="note__label">
                  <Icon name="sparkle" size={16} /> {shortlistNote.label}
                </p>
                {#if shortlistNote.loading}
                  <p class="note__body">
                    <span class="spinner spinner--sm" aria-hidden="true"></span>
                    Asking for a summary…
                  </p>
                {:else if shortlistNote.text}
                  <p class="note__body">{shortlistNote.text}</p>
                  <p class="note__meta">
                    Written by an AI from the botanical facts above. Machine-written, not a reviewed
                    record, and never a cultural claim.
                  </p>
                {:else if shortlistNote.notice}
                  <p class="note__body">{shortlistNote.notice}</p>
                {/if}
              </div>
            {/if}

            <div class="shortlist__list">
              {#each shortlist as option (option.key)}
                <TreeOption
                  {option}
                  open={openOptionKey === option.key}
                  saved={option.tree ? isSpeciesSaved(option.tree) : false}
                  onToggleOpen={toggleOption}
                  onToggleSave={option.tree ? saveOption : undefined}
                />
              {/each}
            </div>

            {#if saveStatus}
              <p class="save-status" role="status">{saveStatus}</p>
            {/if}
          </section>
        {/if}

        <!-- ----------------------------- C. the rest, progressively disclosed -->
        <section class="cat card-surface" aria-labelledby="catalogue-title">
          <button
            class="cat__toggle"
            type="button"
            aria-expanded={catalogueOpen}
            aria-controls="catalogue-panel"
            onclick={() => (catalogueOpen = !catalogueOpen)}
          >
            <span class="cat__toggle-label" id="catalogue-title">
              <Icon name="tree" size={18} /> Browse all trees
            </span>
            <span class="cat__toggle-meta">
              {#if catalogueAvailable === null}
                loading…
              {:else}
                {catalogueAvailable} available
              {/if}
            </span>
            <span class="cat__caret" aria-hidden="true">{catalogueOpen ? '–' : '+'}</span>
          </button>

          {#if catalogueOpen}
            <div class="cat__panel" id="catalogue-panel">
              <p class="cat__note">
                {#if catalogueNotice}
                  {catalogueNotice}
                {:else if catalogueOrdering}
                  Ordered by {catalogueOrdering}. There is no ranking here, and no suggestion that
                  one of these trees is meant for you.
                {:else}
                  Every tree identity Everroot has a verified authority record for.
                {/if}
              </p>

              <div class="field">
                <label for="tree-search">Search by common name, scientific name or family</label>
                <input
                  id="tree-search"
                  class="input"
                  type="search"
                  bind:value={treeSearch}
                  placeholder="e.g. neem, Ficus, Fabaceae"
                  autocomplete="off"
                />
                <p class="field-hint">
                  Filtered in this browser. Nothing you type is sent anywhere.
                </p>
              </div>

              {#if catalogueNotice}
                <p class="cat__note" role="status">
                  <button class="btn btn--soft btn--sm" type="button" onclick={loadCatalogue}>
                    Try again
                  </button>
                </p>
              {:else if catalogueLoading}
                <p class="cat__note" role="status">
                  <span class="spinner spinner--sm" aria-hidden="true"></span>
                  Loading the botanical reference…
                </p>
              {:else if filteredCatalogue.length === 0}
                <p class="cat__note" role="status">
                  {catalogue.length === 0
                    ? 'The botanical reference has no records yet. Everroot will not invent one to fill the space.'
                    : `No record matches “${treeSearch.trim()}”. Try a scientific name, a common name, or a family such as Fabaceae.`}
                </p>
              {:else}
                <ul class="cat__list">
                  {#each filteredCatalogue as tree, index (tree.speciesId)}
                    {@const option = catalogueOption(tree, index)}
                    {@const open = openRowId === tree.speciesId}
                    <li class="cat__item">
                      <button
                        class="cat__row"
                        type="button"
                        aria-expanded={open}
                        aria-controls={`row-panel-${tree.speciesId}`}
                        onclick={() => (openRowId = open ? '' : tree.speciesId)}
                      >
                        <span class="cat__row-name">
                          {option.title}
                          {#if !option.titleIsScientific}
                            <span class="cat__row-sci">{tree.acceptedName}</span>
                          {/if}
                        </span>
                        <span class="cat__row-meta">
                          {tree.family}{#if shortlistSpecies.has(tree.speciesId)} · in your shortlist{/if}
                        </span>
                        <span class="cat__caret" aria-hidden="true">{open ? '–' : '+'}</span>
                      </button>
                      {#if open}
                        <div class="cat__row-panel" id={`row-panel-${tree.speciesId}`}>
                          <TreeOption
                            {option}
                            nested
                            open
                            saved={isSpeciesSaved(tree)}
                            onToggleSave={saveOption}
                          />
                        </div>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/if}
        </section>

        <!-- -------------------------------- D. personal meaning, secondary -->
        <details class="panel card-surface" bind:open={meaningOpen}>
          <summary class="panel__summary">
            <span class="panel__title">
              <Icon name="sparkle" size={18} /> Your own meaning
            </span>
            <span class="panel__hint">Chosen by you. Kept in this browser.</span>
            <span class="panel__caret" aria-hidden="true">{meaningOpen ? '–' : '+'}</span>
          </summary>
          <div class="panel__content">
            <p class="panel__body">{personalReflection(answers.intention)}</p>

            {#if answers.followUpPicks.length}
              <ul class="chips">
                {#each answers.followUpPicks as pick}
                  <li class="chip">{pick}</li>
                {/each}
              </ul>
            {/if}

            {#if answers.followUpNote.trim()}
              <blockquote class="quote">{answers.followUpNote.trim()}</blockquote>
            {:else}
              <p class="panel__note">
                You did not write anything in your own words — that is entirely fine. A personal
                note is never required, and never sent to Everroot’s servers.
              </p>
            {/if}
          </div>
        </details>

        <!-- --------------------------------------- E. dedication, secondary -->
        <details class="panel card-surface" id="dedication" bind:open={dedicationOpen}>
          <summary class="panel__summary">
            <span class="panel__title"><Icon name="heart" size={18} /> A dedication (optional)</span>
            <span class="panel__hint">Kept in this browser. Not published, and it plants nothing.</span>
            <span class="panel__caret" aria-hidden="true">{dedicationOpen ? '–' : '+'}</span>
          </summary>
          <div class="panel__content">
            <div class="dedication__grid">
              <div class="field">
                <label for="dedication-text">Your dedication</label>
                <textarea
                  id="dedication-text"
                  class="textarea"
                  maxlength={500}
                  bind:value={dedicationText}
                  placeholder="e.g. For the ones who kept the garden going."
                  aria-describedby="dedication-hint"
                ></textarea>
                <p class="field-hint" id="dedication-hint">
                  {dedicationText.length}/500 characters. Saved only in this browser.
                </p>
              </div>

              <div class="dedication__preview">
                <p class="dedication__preview-label">Preview</p>
                <div class="dedication__card">
                  <span class="dedication__card-mark" aria-hidden="true">
                    <Icon name="leaf" size={18} />
                  </span>
                  <p class="dedication__card-text">
                    {dedicationText.trim() || 'Your words will appear here.'}
                  </p>
                  <p class="dedication__card-meta">
                    {primaryTreeName
                      ? `For ${primaryTreeName}`
                      : `For ${chosenIntention?.label ?? 'your intention'}`}
                  </p>
                  <p class="dedication__card-privacy">Private · not published</p>
                </div>
              </div>
            </div>

            <div class="dedication__actions">
              <button
                class="btn btn--primary btn--md"
                type="button"
                onclick={commitDedication}
                disabled={!dedicationText.trim()}
              >
                Save in this browser
              </button>
              {#if dedicationSavedAt}
                <button class="btn btn--soft btn--md" type="button" onclick={clearDedication}>
                  Remove
                </button>
              {/if}
            </div>

            {#if dedicationStatus}
              <p class="save-status" role="status">{dedicationStatus}</p>
            {/if}

            <p class="dedication__caveat">
              Nothing here creates, reserves or plants a tree. Everroot has no verified planting
              partner yet, so it will not pretend otherwise.
            </p>
          </div>
        </details>

        <!-- ------------------------------------------------ next steps -->
        <nav class="result__actions" aria-label="Next steps">
          <a class="btn btn--soft btn--md" href="/discover">Change my answers</a>
          <a class="btn btn--soft btn--md" href="/trees">Explore another tree</a>
          <button class="btn btn--ghost btn--md" type="button" onclick={startOver}>
            <Icon name="arrow-left" size={18} /> Start over
          </button>
        </nav>

        {#if saved.length}
          <p class="result__saved" role="status">
            {saved.length === 1
              ? '1 tree is saved in this browser.'
              : `${saved.length} trees are saved in this browser.`}
          </p>
        {/if}
      {/if}
    </div>
  </div>
</div>

<style>
  .result {
    padding-block: clamp(1.5rem, 3.5vw, 2.5rem) var(--space-section);
    background: var(--color-ivory);
    min-height: 66vh;
  }

  .result__inner {
    max-width: 860px;
    margin-inline: auto;
    display: grid;
    gap: 1.25rem;
  }

  /* ---------------------------------------------------------- states */
  .state {
    display: grid;
    place-items: center;
    gap: 0.85rem;
    text-align: center;
    padding: clamp(2rem, 6vw, 4rem) 1.5rem;
  }

  .state__icon {
    display: grid;
    place-items: center;
    width: 54px;
    height: 54px;
    border-radius: 50%;
    background: #eef2e8;
    color: var(--color-forest);
  }

  .state__title {
    font-size: clamp(1.4rem, 3vw, 1.9rem);
  }

  .state__body {
    max-width: 48ch;
    color: var(--color-charcoal-soft);
    line-height: 1.7;
  }

  .spinner {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: 2.5px solid #d7ddcf;
    border-top-color: var(--color-forest);
    animation: spin 0.8s linear infinite;
  }

  .spinner--sm {
    display: inline-block;
    width: 15px;
    height: 15px;
    border-width: 2px;
    vertical-align: -2px;
    margin-right: 0.45rem;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .spinner {
      animation-duration: 2.4s;
    }
  }

  /* --------------------------------------------------------- banners */
  .banner {
    display: flex;
    align-items: flex-start;
    gap: 0.7rem;
    flex-wrap: wrap;
    font-size: 0.94rem;
  }

  .banner :global(svg) {
    flex: none;
    margin-top: 0.15rem;
  }

  /* --------------------------------------------------------- summary */
  .summary {
    padding: clamp(1.15rem, 3vw, 1.6rem);
  }

  .summary__top {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .summary__title {
    margin-top: 0.35rem;
    font-size: clamp(1.45rem, 3.2vw, 1.95rem);
    line-height: 1.2;
    letter-spacing: -0.02em;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.3rem 0.75rem;
    border-radius: var(--radius-pill);
    border: 1px solid var(--color-line);
    background: var(--surface-soft);
    font-size: 0.78rem;
    font-weight: 700;
    letter-spacing: 0.02em;
    color: var(--color-charcoal-soft);
    white-space: nowrap;
  }

  .pill__dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--color-sage);
  }

  .pill--ok {
    color: var(--color-forest);
    border-color: #cfdcc7;
    background: #f1f6ee;
  }

  .pill--ok .pill__dot {
    background: var(--color-forest-soft);
  }

  .summary__lede {
    margin: 0.8rem 0 0;
    max-width: 64ch;
    color: var(--color-charcoal-soft);
    line-height: 1.65;
    font-size: 0.98rem;
  }

  .summary__lede--notice {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.7rem 0.85rem;
    border-radius: var(--radius-control);
    background: #f6f4ea;
    border: 1px solid var(--color-line);
    color: #4d5240;
  }

  .summary__lede--notice :global(svg) {
    flex: none;
    margin-top: 0.2rem;
    color: var(--color-earth);
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin: 1.1rem 0 0;
  }

  .fact {
    padding: 0.45rem 0.7rem;
    background: var(--surface-soft);
    border: 1px solid var(--color-line-soft);
    border-radius: var(--radius-control);
    min-width: 0;
  }

  .fact dt {
    font-size: 0.65rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--color-sage);
  }

  .fact dd {
    margin: 0.15rem 0 0;
    font-weight: 600;
    font-size: 0.9rem;
    color: var(--color-ink);
    overflow-wrap: anywhere;
  }

  /* ------------------------------------------------------- shortlist */
  .shortlist {
    display: grid;
    gap: 0.85rem;
  }

  .shortlist__head {
    display: grid;
    gap: 0.3rem;
  }

  .shortlist__title {
    font-size: clamp(1.15rem, 2.4vw, 1.4rem);
    margin: 0;
  }

  .shortlist__count {
    margin: 0;
    font-size: 0.88rem;
    color: var(--color-charcoal-soft);
  }

  .shortlist__list {
    display: grid;
    gap: 0.75rem;
  }

  .note {
    display: grid;
    gap: 0.4rem;
    padding: 0.9rem 1rem;
    background: #f7faf5;
    border-color: #dde7d8;
  }

  .note__label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-forest);
  }

  .note__body {
    margin: 0;
    font-size: 0.98rem;
    line-height: 1.7;
    color: var(--color-ink);
  }

  .note__meta {
    margin: 0;
    font-size: 0.84rem;
    line-height: 1.6;
    color: var(--color-charcoal-soft);
  }

  .save-status {
    font-size: 0.88rem;
    color: #4c5a48;
    background: #f2f5ee;
    border: 1px solid #dbe3d5;
    border-radius: var(--radius-control);
    padding: 0.6rem 0.85rem;
  }

  /* ------------------------------------------------------- catalogue */
  .cat {
    overflow: hidden;
  }

  .cat__toggle {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    width: 100%;
    padding: 0.95rem clamp(1rem, 2.6vw, 1.35rem);
    background: transparent;
    border: 0;
    cursor: pointer;
    font: inherit;
    text-align: left;
    color: var(--color-ink);
  }

  .cat__toggle:hover {
    background: #fafbf6;
  }

  .cat__toggle:focus-visible {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: -3px;
  }

  .cat__toggle-label {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-family: var(--font-display);
    font-size: 1.08rem;
    font-weight: 600;
  }

  .cat__toggle-label :global(svg) {
    color: var(--color-forest-soft);
  }

  .cat__toggle-meta {
    margin-left: auto;
    font-size: 0.84rem;
    color: var(--color-charcoal-soft);
  }

  .cat__caret {
    font-weight: 700;
    font-size: 1.15rem;
    line-height: 1;
    color: var(--color-forest-soft);
  }

  .cat__panel {
    display: grid;
    gap: 0.85rem;
    padding: 0 clamp(1rem, 2.6vw, 1.35rem) clamp(1rem, 2.6vw, 1.35rem);
    border-top: 1px solid var(--color-line-soft);
    padding-top: 1rem;
  }

  .cat__note {
    margin: 0;
    font-size: 0.88rem;
    line-height: 1.6;
    color: var(--color-charcoal-soft);
  }

  .cat__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.4rem;
  }

  .cat__item {
    border: 1px solid var(--color-line);
    border-radius: var(--radius-control);
    background: var(--surface);
    overflow: hidden;
  }

  .cat__row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas: 'name caret' 'meta caret';
    align-items: center;
    gap: 0.15rem 0.75rem;
    width: 100%;
    padding: 0.65rem 0.85rem;
    background: transparent;
    border: 0;
    text-align: left;
    cursor: pointer;
    font: inherit;
  }

  .cat__row:hover {
    background: #fafbf6;
  }

  .cat__row:focus-visible {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: -2px;
  }

  .cat__row-name {
    grid-area: name;
    font-size: 0.98rem;
    color: var(--color-ink);
    overflow-wrap: anywhere;
  }

  .cat__row-sci {
    font-family: var(--font-display);
    font-style: italic;
    font-size: 0.86rem;
    color: var(--color-forest-soft);
    margin-left: 0.4rem;
  }

  .cat__row-meta {
    grid-area: meta;
    font-size: 0.82rem;
    color: var(--color-charcoal-soft);
    overflow-wrap: anywhere;
  }

  .cat__row-panel {
    padding: 0 0.4rem 0.4rem;
  }

  /* ------------------------------------------- secondary details panels */
  .panel {
    padding: 0;
    overflow: hidden;
  }

  .panel__summary {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    padding: 0.95rem clamp(1rem, 2.6vw, 1.35rem);
    cursor: pointer;
    list-style: none;
  }

  .panel__summary::-webkit-details-marker {
    display: none;
  }

  .panel__summary:hover {
    background: #fafbf6;
  }

  .panel__summary:focus-visible {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: -3px;
  }

  .panel__title {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-family: var(--font-display);
    font-size: 1.08rem;
    font-weight: 600;
    color: var(--color-ink);
  }

  .panel__title :global(svg) {
    color: var(--color-forest-soft);
  }

  .panel__hint {
    font-size: 0.84rem;
    color: var(--color-charcoal-soft);
  }

  .panel__caret {
    margin-left: auto;
    font-weight: 700;
    font-size: 1.15rem;
    line-height: 1;
    color: var(--color-forest-soft);
  }

  .panel__content {
    padding: 0 clamp(1rem, 2.6vw, 1.35rem) clamp(1rem, 2.6vw, 1.35rem);
    border-top: 1px solid var(--color-line-soft);
    padding-top: 1rem;
    display: grid;
    gap: 0.85rem;
  }

  .panel__note {
    margin: 0;
    color: var(--color-charcoal-soft);
    font-size: 0.9rem;
    line-height: 1.65;
    max-width: 66ch;
  }

  .panel__body {
    margin: 0;
    line-height: 1.7;
    color: #3c463c;
    max-width: 66ch;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .chip {
    background: #eef2e8;
    border: 1px solid #dbe3d5;
    color: #3f5a44;
    border-radius: var(--radius-pill);
    padding: 0.25rem 0.75rem;
    font-size: 0.84rem;
    font-weight: 600;
  }

  .quote {
    margin: 0;
    padding: 0.85rem 1.1rem;
    border-left: 3px solid var(--color-moss);
    background: var(--surface-soft);
    border-radius: 0 var(--radius-control) var(--radius-control) 0;
    font-family: var(--font-display);
    font-size: 1.02rem;
    line-height: 1.6;
    color: #354336;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  /* ------------------------------------------------- dedication */
  .dedication__grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 0.85fr);
    gap: 1.2rem;
    align-items: start;
  }

  .dedication__grid .field {
    margin-top: 0;
  }

  .dedication__preview-label {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-sage);
    margin-bottom: 0.5rem;
  }

  .dedication__card {
    background: linear-gradient(160deg, #fbfaf3 0%, #f3f4e9 100%);
    border: 1px solid var(--color-line);
    border-radius: var(--radius-card);
    padding: 1.1rem 1rem;
    text-align: center;
    display: grid;
    gap: 0.5rem;
    justify-items: center;
  }

  .dedication__card-mark {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: #e6ecdd;
    color: var(--color-forest);
  }

  .dedication__card-text {
    font-family: var(--font-display);
    font-size: 1rem;
    line-height: 1.55;
    color: #354336;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .dedication__card-meta {
    font-size: 0.8rem;
    color: #6f7a68;
  }

  .dedication__card-privacy {
    font-size: 0.66rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #8a8168;
    border: 1px solid #e2ddc8;
    border-radius: var(--radius-pill);
    padding: 0.15rem 0.6rem;
  }

  .dedication__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }

  .dedication__caveat {
    margin: 0;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-line-soft);
    font-size: 0.82rem;
    color: #6f7a68;
  }

  /* ------------------------------------------------- next steps */
  .result__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    padding-top: 0.25rem;
  }

  .result__saved {
    font-size: 0.84rem;
    color: var(--color-charcoal-soft);
  }

  /* ------------------------------------------------- responsive */
  @media (max-width: 760px) {
    .dedication__grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 560px) {
    .result {
      padding-block: 1.25rem var(--space-section-mobile);
    }

    .summary__top {
      display: grid;
    }

    .pill {
      justify-self: start;
    }

    .cat__toggle-meta {
      margin-left: 0;
    }

    .result__actions .btn,
    .panel__content .dedication__actions .btn {
      width: 100%;
    }
  }
</style>
