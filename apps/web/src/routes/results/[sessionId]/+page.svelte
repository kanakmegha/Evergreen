<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import BirdScene from '../../../lib/components/BirdScene.svelte';
  import Icon from '../../../lib/components/Icon.svelte';
  import TreeOption from '../../../lib/components/TreeOption.svelte';
  import { referenceOption, reviewedOption, suggestionOption, SUGGESTION_RULE } from '../../../lib/treeOption';
  import { filterTrees } from '../../../lib/treeOption';
  import type { TreeOptionView } from '../../../lib/treeOption';
  import {
    askQuestion,
    aiDiscoveryRequest,
    candidateToSuggestion,
    clearFlow,
    discoveryRequest,
    distinctSources,
    emptyAnswers,
    explanationFailureText,
    fetchCatalogue,
    fetchSession,
    followUpOptionLabel,
    intention as intentionInfo,
    loadDedication,
    loadRecord,
    loadSaved,
    loadSessionToken,
    personalReflection,
    recommendationsRequest,
    removeDedication,
    resolveIntentionId,
    runAIDiscovery,
    runDiscovery,
    runExplain,
    runRecommendations,
    saveDedication,
    saveRecord,
    submitFeedback,
    toggleSaved,
    usesNakshatra,
  } from '../../../lib/discovery';
  import type {
    AICandidate,
    AIDiscoveryResponse,
    DiscoveryRecord,
    DiscoveryRequestInput,
    EvidenceStatus,
    ExplanationEnvelope,
    RecommendationSuggestion,
    ResearchSource,
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

  /**
   * Tree suggestions drawn from the visitor's own answers.
   *
   * A separate track from both reviewed records and the botanical reference:
   * it needs neither the database nor the catalogue, so an empty or unreachable
   * catalogue cannot stop a suggestion arriving. Everything it produces is
   * labelled as generated wherever it is shown.
   */
  let suggestions = $state<RecommendationSuggestion[]>([]);
  let suggestionsEvidence = $state<EvidenceStatus | null>(null);
  let suggestionsStatus = $state('');
  let suggestionsLoading = $state(false);
  let suggestionNotice = $state('');

  /**
   * The research-backed discovery, when it answered.
   *
   * It is fetched beside the suggestion call rather than instead of it, and it
   * is allowed to be absent: a page that could not research still has an
   * answer to show, and the provenance section simply does not render. What it
   * must never do is render a research panel from sources that were not
   * actually retrieved — which is why this stays null rather than defaulting
   * to an empty "researched" result.
   */
  let aiDiscovery = $state<AIDiscoveryResponse | null>(null);
  /**
   * The one question the model asked because a missing fact would have changed
   * its suggestions. It arrives beside the answer, never instead of it, and it
   * has no field of its own to be typed into: "Change my answers" is how a
   * visitor answers it, so nothing here sends a second model call.
   */
  let suggestionsQuestion = $state('');

  /**
   * Where the bird is, and therefore what the visitor is being asked to watch.
   *
   * ''         no suggestion call is running and none has landed
   * 'flying'   the model has not answered yet, so the bird is crossing the garden
   * 'landing'  an answer arrived and the bird is coming down onto that tree
   * 'landed'   the bird is resting, so the information beside it may be read
   *
   * The scene is decoration over a real state: it never slows the call, and it
   * is dropped entirely when there is no answer to rest on.
   */
  let birdPhase = $state<'flying' | 'landing' | 'landed' | ''>('');
  let birdTimer: ReturnType<typeof setTimeout> | undefined;

  /** Only run the landing sequence when the visitor has not asked for stillness. */
  function prefersStillness(): boolean {
    return (
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    );
  }

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

  /* --------------------------------------------- user AI questions & feedback */
  let userQuestion = $state('');
  let questionAnswer = $state<string | null>(null);
  let questionInteractionId = $state<string | null>(null);
  let questionLoading = $state(false);
  let questionError = $state('');
  let feedbackGiven = $state<number | null>(null);

  async function handleAskQuestion() {
    const q = userQuestion.trim();
    if (!q || questionLoading) return;
    questionLoading = true;
    questionError = '';
    feedbackGiven = null;
    try {
      const res = await askQuestion({
        question: q,
        sessionId: record?.id,
        intention: answers.intention ?? '',
        context: {
          Tradition: answers.tradition ?? '',
          Nakshatra: answers.nakshatra ?? '',
          Region: answers.region ?? '',
        },
      });
      questionAnswer = res.answer;
      questionInteractionId = res.interactionId ?? null;
    } catch (e) {
      questionError = e instanceof Error ? e.message : 'Could not generate answer. Please try again.';
    } finally {
      questionLoading = false;
    }
  }

  async function handleFeedback(rating: number) {
    if (feedbackGiven !== null) return;
    feedbackGiven = rating;
    if (questionInteractionId) {
      await submitFeedback({
        interactionId: questionInteractionId,
        rating,
        feedbackType: 'answer',
      });
    }
  }

  /* ------------------------------------------------------------- derived */

  const answers = $derived(record?.answers ?? emptyAnswers());
  const chosenIntention = $derived(intentionInfo(answers.intention));
  const recommendations = $derived(record?.discovery?.recommendations ?? []);
  const hasResults = $derived(
    record?.discovery?.status === 'verified_results' && recommendations.length > 0
  );
  const followUpLabel = $derived(followUpOptionLabel(answers.intention, answers.followUpChoice));
  const showTraditionFacts = $derived(Boolean(answers.tradition));
  const showNakshatraFact = $derived(Boolean(answers.nakshatra) || usesNakshatra(answers));

  /** The cultural-association state, as the server's own publication gate reported it. */
  const culturalAssociation = $derived(
    hasResults
      ? 'reviewed_match'
      : (suggestionsEvidence?.culturalAssociation ?? explorationEvidence?.culturalAssociation ?? '')
  );

  /**
   * The AI suggestions, shaped for the same card as every other option and
   * tagged as generated. The cultural verdict on each card still comes from the
   * publication gate — never from the model that wrote the suggestion.
   */
  const suggestionViews = $derived(
    suggestions
      .slice(0, SHORTLIST_SIZE)
      .map((suggestion, index) =>
        suggestionOption(suggestion, index, 'shortlist', culturalAssociation)
      )
  );

  /* ------------------------------------------------ research provenance */

  /**
   * What actually happened during research, as the pipeline recorded it.
   *
   * It is read from the response rather than inferred from whether sources are
   * present, because the two are not the same statement: an answer can carry
   * no sources because nothing was researched, or because research ran and
   * found nothing worth citing. Only the first of those is an outage, and only
   * the second could ever be shown without a caveat.
   */
  const researchStatus = $derived(aiDiscovery?.researchStatus ?? null);
  const researchSummary = $derived(aiDiscovery?.researchSummary ?? '');
  const unverifiedAreas = $derived(aiDiscovery?.unverifiedAreas ?? []);
  const researchDisclaimer = $derived(aiDiscovery?.disclaimer ?? '');

  /** Every source the answer drew on, in first-cited order. */
  const consultedSources = $derived<ResearchSource[]>(
    aiDiscovery ? distinctSources(aiDiscovery) : []
  );

  /**
   * The AI candidates, kept alongside the suggestion views rather than
   * replacing them, so the provenance panel can name the exact candidate a
   * claim belongs to even when the shortlist was drawn from another source.
   */
  const aiCandidates = $derived<AICandidate[]>(aiDiscovery?.candidates ?? []);

  /** Renders one candidate's support in the words the vocabulary defines. */
  function evidenceSummary(candidate: AICandidate): string {
    const parts: string[] = [];
    const cultural = candidate.culturalSignificance;
    const ecological = candidate.ecologicalFit;

    if (cultural?.evidence === 'sourced' && (cultural.sourceIds?.length ?? 0) > 0) {
      parts.push('cultural claim backed by a source');
    } else if (cultural?.evidence === 'interpretation') {
      parts.push('cultural reading, not a documented fact');
    } else if (cultural?.evidence === 'user_defined') {
      parts.push('meaning you supplied yourself');
    } else if (cultural?.evidence === 'uncertain') {
      parts.push('cultural claim only partly supported');
    } else {
      parts.push('no cultural evidence found');
    }

    if (ecological?.evidence === 'sourced' && (ecological.sourceIds?.length ?? 0) > 0) {
      parts.push('regional fit backed by a source');
    } else {
      parts.push('regional fit not evidenced');
    }

    const identity = candidate.identity?.status;
    if (identity === 'confirmed') parts.push('botanical name confirmed');
    else if (identity === 'not_found') parts.push('botanical name not found in the taxonomy record');
    else if (identity === 'unverified') parts.push('botanical name could not be checked');

    parts.push(`confidence ${candidate.confidence?.level ?? 'low'}`);
    return parts.join(' · ');
  }

  /** A retrieval date in the visitor's own locale, without the time. */
  function formatRetrieved(iso: string): string {
    const parsed = new Date(iso);
    if (Number.isNaN(parsed.getTime())) return 'date unknown';
    return parsed.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }

  /**
   * The botanical records the shortlist falls back to. The explanation endpoint
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
    if (suggestionViews.length) return suggestionViews;
    // When dynamic AI discovery ran, do not fall back to unpopulated catalogue
    if (aiDiscovery) return [];
    return referenceTrees
      .slice(0, SHORTLIST_SIZE)
      .map((tree, index) => referenceOption(tree, index, 'shortlist', true, culturalAssociation));
  });

  /** Both calls that can fill the shortlist have settled and neither produced anything. */
  const shortlistPending = $derived(exploratory && (suggestionsLoading || explorationLoading));
  const shortlistEmpty = $derived(
    exploratory && !shortlist.length && !suggestionsLoading && !explorationLoading
  );
  const shortlistEmptyReason = $derived.by(() => {
    if (aiDiscovery) {
      if (aiDiscovery.status === 'no_candidates' || aiDiscovery.researchStatus === 'no_sources') {
        return (
          aiDiscovery.researchSummary ||
          'The research request completed, but found no tree candidates with sufficient documented evidence for these specific terms. You can broaden or rephrase your intention below.'
        );
      }
      if (aiDiscovery.researchStatus === 'not_configured') {
        return (
          aiDiscovery.researchSummary ||
          'Research provider is not configured on this server. Set RESEARCH_PROVIDER=open-web to enable key-free web research.'
        );
      }
      if (aiDiscovery.researchStatus === 'unavailable') {
        return (
          aiDiscovery.researchSummary ||
          'Research is temporarily unavailable right now. This does not mean no suitable trees exist — please try again in a moment or revise your question.'
        );
      }
    }
    return (
      [suggestionNotice, explorationNotice].filter(Boolean).join(' ') ||
      (suggestionsStatus === 'no_suggestions'
        ? 'The suggestion model did not find a tree it would stand behind for these answers.'
        : 'The suggestion service returned nothing for these answers.')
    );
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
    if (suggestionViews.length) {
      // What these trees are, said once and plainly: they answer the visitor's
      // own answers rather than a record, so they say so instead of carrying a
      // warning about a dataset that was never consulted for them.
      return suggestionViews.length === 1
        ? 'One tree suggested from your answers — general botany written by the AI model, not a reviewed cultural record.'
        : `${suggestionViews.length} trees suggested from your answers — general botany written by the AI model, not a reviewed cultural record.`;
    }
    // Nothing came back from the model, so the shortlist is what the botanical
    // reference holds. That is a normal outcome rather than a failure, and it
    // is not one caused by which cultural answers the visitor gave or skipped
    // — nothing is being withheld from them, so there is nothing here to warn
    // them about. The note beneath the shortlist is where the difference
    // between a suggestion and a record gets explained.
    if (!shortlist.length) return '';
    return shortlist.length === 1
      ? 'One tree from the botanical reference that fits your answers, with its general botany.'
      : `${shortlist.length} trees from the botanical reference that fit your answers, with their general botany.`;
  });

  /**
   * The lede is a description, never a warning. Nothing about it depends on
   * which cultural answers were given or skipped, so it is never dressed as a
   * notice — a visitor whose cultural selection found nothing is not being
   * told they did something wrong.
   */

  /**
   * A discreet indicator of what backs the page, not a promise about the trees.
   * A missing reviewed record is described as scope ("botanical records only"),
   * never as a verdict about the visitor's selection — the single notice in the
   * lede already carries that.
   */
  const evidencePill = $derived.by<{ text: string; tone: 'ok' | 'none' } | null>(() => {
    if (hasResults) return { text: 'Reviewed association on record', tone: 'ok' };
    if (aiDiscovery) {
      if (researchStatus === 'researched') return { text: 'Source-grounded discovery', tone: 'ok' };
      if (researchStatus === 'partial') return { text: 'Partially researched discovery', tone: 'ok' };
      if (researchStatus === 'no_sources') return { text: 'Provisional discovery (no sources found)', tone: 'none' };
      if (researchStatus === 'not_configured') return { text: 'Provisional discovery (research unconfigured)', tone: 'none' };
      if (researchStatus === 'unavailable') return { text: 'Provisional discovery (research unavailable)', tone: 'none' };
    }
    if (!culturalAssociation) return null;
    if (culturalAssociation === 'reviewed_match') return { text: 'Reviewed record exists', tone: 'ok' };
    return { text: 'Botanical records only', tone: 'none' };
  });

  const shortlistNote = $derived.by(() => {
    if (hasResults || !exploratory) return null;
    if (suggestionViews.length) {
      // These were written from the visitor's answers, so the note explains
      // what a suggestion is instead of carrying a summary nobody wrote.
      return {
        label: 'About these suggestions',
        loading: false,
        loadingText: '',
        text: SUGGESTION_RULE,
        meta: 'There is no citation behind these, because they were written for you rather than quoted from a record. Open the tree reference to check a name against an authority record.',
        notice: '',
      };
    }
    const notice = [
      // A model that found nothing is telling the visitor something, and that
      // stays true even though the reference could fill the shortlist behind it.
      suggestionsStatus === 'no_suggestions'
        ? 'The suggestion model did not find a tree it would stand behind for these answers, so the botanical records below stand in — they are not suggestions.'
        : '',
      suggestionNotice,
      explorationNotice,
    ]
      .filter(Boolean)
      .join(' ');
    return {
      label: 'Exploration summary',
      loading: suggestionsLoading || (explorationLoading && !explorationNotice),
      loadingText: suggestionsLoading
        ? 'Asking the AI model for tree suggestions…'
        : 'Asking for a summary…',
      // A failure outranks a summary: the visitor has to know the call broke
      // rather than be handed prose as if nothing had gone wrong.
      text: notice ? '' : (exploration?.status === 'ok' ? exploration.text : ''),
      meta: 'Written by an AI from the botanical facts above, for your answers rather than quoted from a record.',
      notice,
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
    // The cultural lookup and the suggestion call are independent. A failed
    // lookup must not cost the visitor their suggestions, and neither may
    // depend on the botanical catalogue being readable.
    if (!hasResults) void loadSuggestions();
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
    // A failed suggestion call is worth another attempt when the visitor asks
    // again; one that already arrived is not re-bought for an unrelated retry.
    if (!hasResults && !record.error && !suggestions.length) void loadSuggestions();
  }

  /* --------------------------------------------- exploratory discovery */

  /**
   * Tree suggestions from the visitor's answers: the first call this page makes
   * and the only one that needs neither the database nor the botanical
   * catalogue.
   *
   * The research-backed discovery is tried first, because it is the only call
   * that can attach a source to a claim. It must be tried first rather than in
   * parallel with the suggestion call: a shortlist drawn from one endpoint and
   * provenance drawn from another would let a sourced claim sit underneath a
   * card it never supported. So the two are alternatives — exactly one model
   * call is made per view — and whichever produced the cards is the one whose
   * provenance is shown.
   *
   * When it is unavailable the older suggestion call runs and the provenance
   * panel simply does not render. That absence is deliberate: the honest empty
   * state for "we have no research record" is not to claim any provenance, not
   * to display a panel of caveats beside an answer nobody looked anything up
   * for.
   */
  async function loadSuggestions(force = false) {
    // One request per result, and never a second while one is in flight: these
    // are paid calls, so a re-entry must not silently buy another answer.
    if (!record || suggestionsLoading || (suggestions.length && !force)) return;
    suggestionsLoading = true;
    suggestionNotice = '';
    suggestionsQuestion = '';
    suggestionsEvidence = null;
    aiDiscovery = null;
    birdPhase = 'flying';
    clearTimeout(birdTimer);

    let produced = false;
    try {
      const researched = await runAIDiscovery(aiDiscoveryRequest(answers), loadSessionToken() ?? undefined);
      aiDiscovery = researched;
      suggestionsStatus = researched.status;
      suggestionsQuestion = researched.followUpQuestion ?? '';
      if (researched.candidates.length) {
        suggestions = researched.candidates.map((c) => candidateToSuggestion(c, researched.sources));
        // No gate verdict travels with a research pass. The publication gate
        // only ever reports on a REVIEWED association, and this answer has
        // none — leaving the evidence null means the cards claim no reviewed
        // match, which is exactly what is true of them.
        suggestionsEvidence = null;
        produced = true;
      } else {
        // An honest empty answer from the model is still an answer, and it
        // must not be retried by the older endpoint as though it were a
        // failure. The provenance panel has something to say about it: no
        // candidates, and why.
        suggestions = [];
        produced = true;
      }
    } catch (err) {
      // The research pass is optional. Falling through to the older endpoint
      // keeps the shortlist working while the new one is unconfigured, and the
      // panel stays hidden so nothing implies a research record exists.
      aiDiscovery = null;
      suggestionNotice =
        err instanceof Error ? err.message : 'Research service is temporarily unavailable.';
    }

    if (!produced) {
      try {
        const response = await runRecommendations(recommendationsRequest(answers));
        suggestions = response.suggestions;
        suggestionsEvidence = response.evidence;
        suggestionsStatus = response.status;
        suggestionsQuestion = response.followUpQuestion ?? '';
      } catch (error) {
        suggestions = [];
        suggestionsEvidence = null;
        suggestionsStatus = '';
        suggestionsQuestion = '';
        suggestionNotice =
          error instanceof Error ? error.message : 'Tree suggestions are unavailable right now.';
      }
    }

    if (suggestions.length) {
      // There is a tree to rest on, so the bird comes down onto it. A visitor
      // who asked for no motion goes straight to the finished picture.
      birdPhase = prefersStillness() ? 'landed' : 'landing';
      birdTimer = setTimeout(() => (birdPhase = 'landed'), 1600);
    } else {
      // Nothing landed, so the scene goes away with it: a picture of success
      // over a failed call would be the same lie as an empty list.
      birdPhase = '';
    }
    suggestionsLoading = false;
    if (!suggestions.length) void loadExploration();
  }

  /**
   * The grounding call behind the reference shortlist. It returns the shortlist
   * records and the cultural-association state whatever the provider does, so a
   * provider failure never empties the shortlist.
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
  onDestroy(() => clearTimeout(birdTimer));

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
            <p class="summary__lede">
              <span>{lede}</span>
            </p>
          {/if}

          <!--
            A question the model asked because one missing fact would have
            changed what it suggested. It is shown as a question, beside the
            answer it did not wait for, and it has no field of its own: the
            visitor answers it the way they answered everything else here.
          -->
          {#if suggestionsQuestion}
            <p class="summary__ask">
              <Icon name="info" size={17} />
              <span><strong>One thing that would help:</strong> {suggestionsQuestion}</span>
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
                <!-- Repeated back in the naming system it was chosen in, not in a
                     transliteration of it. The canonical name is what was sent on;
                     this is what the visitor read when they picked it. -->
                <dd>{answers.nakshatraName || answers.nakshatra || 'Skipped'}</dd>
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
        {#if shortlist.length || shortlistPending || shortlistEmpty}
          <section class="shortlist" aria-labelledby="shortlist-title">
            <!--
              The wait, and the arrival. While the model works the bird crosses
              the garden; when the answer comes it comes down onto the chosen
              tree, and the information below is what it found. Shown only while
              a suggestion call is genuinely running or has genuinely landed —
              a failure or an empty answer drops the scene rather than pose as
              a success.
            -->
            {#if birdPhase !== ''}
              <div
                class="shortlist__flight"
                class:shortlist__flight--settled={birdPhase === 'landed'}
              >
                <BirdScene
                  phase={birdPhase}
                  treeName={suggestionViews[0]?.title ?? suggestions[0]?.name ?? ''}
                >
                  {#if birdPhase === 'flying'}
                    <p class="flight__caption" role="status">
                      Asking the model which tree fits your answers…
                    </p>
                  {:else if birdPhase === 'landing'}
                    <p class="flight__caption" role="status">
                      Found it — bringing you the tree it chose.
                    </p>
                  {:else if suggestionViews[0]?.title || suggestions[0]?.name}
                    <p class="flight__caption">
                      Resting on <strong>{suggestionViews[0]?.title ?? suggestions[0]?.name}</strong>
                      — here is why it was chosen for you.
                    </p>
                  {/if}
                </BirdScene>
              </div>
            {/if}

            <div class="shortlist__head">
              <h2 id="shortlist-title" class="shortlist__title">
                {hasResults
                  ? recommendations.length === 1
                    ? 'The association on record'
                    : 'The associations on record'
                  : 'Start with these'}
              </h2>
              {#if !hasResults && !suggestionViews.length && shortlist.length < SHORTLIST_SIZE && !aiDiscovery}
                <p class="shortlist__count">
                  {catalogue.length}
                  {catalogue.length === 1 ? 'tree is' : 'trees are'} in Everroot’s botanical
                  reference, so these are all of them.
                </p>
              {/if}
            </div>

            {#if shortlistEmpty}
              <!-- Actionable discovery empty / outage state -->
              <div class="card-surface panel" style="padding: 1.5rem; margin-top: 1rem; border-radius: 8px;" role="status">
                <p style="font-size: 1.05rem; font-weight: 600; color: #1e332a; margin-bottom: 0.5rem;">
                  {#if aiDiscovery?.researchStatus === 'unavailable' || suggestionNotice}
                    Research temporarily unavailable
                  {:else if aiDiscovery?.researchStatus === 'not_configured'}
                    Research provider unconfigured
                  {:else if aiDiscovery?.status === 'no_candidates' || aiDiscovery?.researchStatus === 'no_sources'}
                    No candidates found with sufficient evidence
                  {:else}
                    Nothing to show for these answers yet
                  {/if}
                </p>
                <p style="font-size: 0.92rem; color: #53645c; line-height: 1.5; margin-bottom: 1.25rem;">
                  {shortlistEmptyReason}
                </p>
                <div style="display: flex; gap: 0.75rem; flex-wrap: wrap; align-items: center;">
                  {#if aiDiscovery?.researchStatus === 'unavailable' || aiDiscovery?.researchStatus === 'not_configured' || suggestionNotice}
                    <button
                      class="btn btn--primary btn--sm"
                      type="button"
                      onclick={() => void loadSuggestions(true)}
                      disabled={suggestionsLoading}
                    >
                      {suggestionsLoading ? 'Retrying…' : 'Retry discovery'}
                    </button>
                  {/if}
                  <a class="btn btn--soft btn--sm" href="/discover">
                    Revise your intention
                  </a>
                </div>
              </div>
            {:else if shortlistNote && (shortlistNote.loading || shortlistNote.text || shortlistNote.notice)}
              <div class="note card-surface" aria-live="polite">
                <p class="note__label">
                  <Icon name="sparkle" size={16} /> {shortlistNote.label}
                </p>
                {#if shortlistNote.loading}
                  <p class="note__body">
                    <span class="spinner spinner--sm" aria-hidden="true"></span>
                    {shortlistNote.loadingText}
                  </p>
                {:else if shortlistNote.text}
                  <p class="note__body">{shortlistNote.text}</p>
                  <p class="note__meta">{shortlistNote.meta}</p>
                {:else if shortlistNote.notice}
                  <p class="note__body">{shortlistNote.notice}</p>
                {/if}
              </div>
            {/if}

            <!-- The cards exist from the moment the answer arrives, so nothing
                 is waiting on an animation to become readable: the settle only
                 moves them into place as the bird folds its wings. -->
            <div
              class="shortlist__list"
              class:shortlist__list--arriving={birdPhase === 'landing'}
            >
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

        <!-- ------------------------------------- B2. what this rests on -->
        <!-- The honest account of how the answer above was produced: what was
             researched, which claims a source backs, and which do not. It sits
             with the shortlist rather than in a footer because a visitor should
             be able to see the provenance of a claim at the same moment they
             read the claim.
             It renders only when a discovery actually came back: an absent
             panel and a panel full of caveats would mean different things, and
             the honest empty state is simply not to claim any provenance. -->
        {#if aiDiscovery}
        <section class="card-surface panel" style="margin: 1.5rem 0; padding: 1.25rem;" aria-labelledby="provenance-title">
          <h2 id="provenance-title" style="font-size: 1.15rem; font-weight: 650; margin-bottom: 0.5rem;">
            What this answer rests on
          </h2>

          {#if researchStatus === 'unavailable'}
            <!-- A discovery that could not be researched is still an answer, but
                 it must never read as a sourced one. Saying so plainly is the
                 whole point of the field. -->
            <p class="notice notice--info banner" role="status" style="margin-bottom: 0.75rem;">
              Nothing was researched for this answer, so it comes from general
              botanical knowledge rather than from sources. Treat the tree ideas
              as a starting point to check, not as documented findings.
            </p>
          {:else if researchStatus === 'not_configured'}
            <p class="notice notice--info banner" role="status" style="margin-bottom: 0.75rem;">
              Research provider is not configured on this server. To enable key-free web and biodiversity research, set <code>RESEARCH_PROVIDER=open-web</code> in your environment.
            </p>
          {:else if researchStatus === 'no_sources'}
            <p class="notice notice--info banner" role="status" style="margin-bottom: 0.75rem;">
              Research ran, but no reliable external sources were found for these specific criteria.
            </p>
          {:else if researchStatus === 'partial'}
            <p class="notice notice--info banner" role="status" style="margin-bottom: 0.75rem;">
              Partly researched: some sources were retrieved, but at least one
              source could not be reached. The findings below are real; the
              search was simply incomplete.
            </p>
          {/if}

          {#if researchSummary}
            <p style="font-size: 0.9rem; color: #53645c; margin-bottom: 0.75rem;">{researchSummary}</p>
          {/if}

          {#if unverifiedAreas.length}
            <div style="margin-bottom: 0.75rem;">
              <h3 style="font-size: 0.95rem; font-weight: 620; margin-bottom: 0.35rem;">Not settled by the sources</h3>
              <ul style="margin: 0; padding-left: 1.1rem; font-size: 0.9rem; color: #53645c;">
                {#each unverifiedAreas as area (area)}
                  <li style="margin-bottom: 0.2rem;">{area}</li>
                {/each}
              </ul>
            </div>
          {/if}

          {#if consultedSources.length}
            <div style="margin-bottom: 0.75rem;">
              <h3 style="font-size: 0.95rem; font-weight: 620; margin-bottom: 0.35rem;">
                Sources consulted ({consultedSources.length})
              </h3>
              <ul style="list-style: none; margin: 0; padding: 0;">
                {#each consultedSources as source (source.id)}
                  <li style="margin-bottom: 0.6rem; font-size: 0.88rem;">
                    <a href={source.url} target="_blank" rel="noopener noreferrer nofollow"
                       style="color: #27664e; font-weight: 560;">
                      {source.title}{source.publisher ? ` — ${source.publisher}` : ''}
                    </a>
                    <span style="color: #68756c;">
                      · {source.kind.replace(/_/g, ' ')}
                      · retrieved {formatRetrieved(source.retrievedAt)}
                    </span>
                    {#if source.snippet}
                      <p style="margin: 0.15rem 0 0; color: #53645c;">{source.snippet}</p>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {:else if researchStatus !== 'unavailable'}
            <p style="font-size: 0.88rem; color: #68756c; margin-bottom: 0.75rem;">
              No sources are attached to this answer, so no claim below should be
              read as documented.
            </p>
          {/if}

          {#if aiCandidates.length}
            <div>
              <h3 style="font-size: 0.95rem; font-weight: 620; margin-bottom: 0.35rem;">How well each claim is supported</h3>
              <ul style="list-style: none; margin: 0; padding: 0;">
                {#each aiCandidates as candidate (candidate.name + candidate.scientificName)}
                  <li style="margin-bottom: 0.6rem; font-size: 0.88rem; color: #53645c;">
                    <strong style="color: #1e332a;">{candidate.name}</strong>
                    — {evidenceSummary(candidate)}
                  </li>
                {/each}
              </ul>
              <p style="font-size: 0.8rem; color: #68756c; margin-top: 0.35rem;">
                Confidence is the model's own read of the retrieved sources. It is
                not a measured score of evidence quality, and it is never a
                review of the cultural claim.
              </p>
            </div>
          {/if}

          {#if researchDisclaimer}
            <p style="font-size: 0.8rem; color: #68756c; margin-top: 0.75rem;">{researchDisclaimer}</p>
          {/if}
        </section>
        {/if}

        <!-- ----------------------------- Ask Everroot AI Question Section -->
        <section class="card-surface panel" style="margin: 1.5rem 0; padding: 1.25rem;">
          <h2 style="font-size: 1.15rem; font-weight: 650; margin-bottom: 0.5rem; display: flex; align-items: center; gap: 0.5rem;">
            <Icon name="sparkle" size={18} /> Ask about a tree
          </h2>
          <p style="font-size: 0.9rem; color: #53645c; margin-bottom: 0.75rem;">
            Ask a botanical question and a language model will answer from general knowledge. Cultural associations never come from here — those appear only with Everroot's reviewed sources attached.
          </p>
          <div style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
            <input
              type="text"
              class="input"
              style="flex: 1; min-width: 240px; padding: 0.6rem 0.8rem; border-radius: 8px; border: 1px solid #c8d0c8; font-size: 0.95rem;"
              placeholder="e.g. What family does Jamun belong to, or how tall does Sal grow?"
              bind:value={userQuestion}
              onkeydown={(e) => { if (e.key === 'Enter') void handleAskQuestion(); }}
              disabled={questionLoading}
            />
            <button
              class="btn btn--primary btn--sm"
              type="button"
              onclick={handleAskQuestion}
              disabled={questionLoading || !userQuestion.trim()}
            >
              {questionLoading ? 'Asking…' : 'Ask'}
            </button>
          </div>

          {#if questionError}
            <p style="color: #a22d26; font-size: 0.88rem; margin-top: 0.5rem;" role="alert">{questionError}</p>
          {/if}

          {#if questionAnswer}
            <div style="margin-top: 1rem; padding: 1rem; background: #f6f8f6; border-radius: 8px; border-left: 3px solid #27664e;">
              <p style="font-size: 0.95rem; line-height: 1.5; color: #1e332a; white-space: pre-wrap;">{questionAnswer}</p>
              <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 0.75rem; flex-wrap: wrap; gap: 0.5rem;">
                <span style="font-size: 0.78rem; color: #68756c;">
                  Machine-written from general botanical knowledge · not a reviewed record
                </span>
                <div style="display: flex; align-items: center; gap: 0.5rem;">
                  <span style="font-size: 0.82rem; color: #53645c;">Was this helpful?</span>
                  <button
                    type="button"
                    class="btn btn--soft btn--sm"
                    style="padding: 0.2rem 0.5rem; font-size: 0.8rem;"
                    onclick={() => void handleFeedback(1)}
                    disabled={feedbackGiven !== null}
                  >
                    {feedbackGiven === 1 ? '✓ Helpful' : '👍 Helpful'}
                  </button>
                  <button
                    type="button"
                    class="btn btn--soft btn--sm"
                    style="padding: 0.2rem 0.5rem; font-size: 0.8rem;"
                    onclick={() => void handleFeedback(-1)}
                    disabled={feedbackGiven !== null}
                  >
                    {feedbackGiven === -1 ? '✓ Noted' : '👎 Not helpful'}
                  </button>
                </div>
              </div>
            </div>
          {/if}
        </section>

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
              {:else if catalogueNotice}
                <!-- An unreadable reference is not an empty one: saying "0 available" here
                     would report a failure as a fact about Everroot's records. -->
                unavailable
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

  /* A question beside the answer: quieter than the notice, because it is an
     offer rather than a limitation. */
  .summary__ask {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin: 0.7rem 0 0;
    max-width: 64ch;
    color: var(--color-charcoal-soft);
    line-height: 1.6;
    font-size: 0.94rem;
  }

  .summary__ask :global(svg) {
    flex: none;
    margin-top: 0.2rem;
    color: var(--color-earth);
  }

  .summary__ask strong {
    color: var(--color-charcoal, #2c3025);
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

  /* ------------------------------------------------ the bird and its landing */
  .shortlist__flight {
    margin: 0 0 0.35rem;
    animation: flight-in 0.45s ease-out both;
  }

  .flight__caption {
    margin: 0;
  }

  .shortlist__list {
    display: grid;
    gap: 0.75rem;
    /* The cards are already in the document while the bird is still coming
       down — they are only moved into place, never withheld from a reader. */
    transition: opacity 0.6s ease, transform 0.6s ease;
  }

  .shortlist__list--arriving {
    opacity: 0;
    transform: translateY(10px);
  }

  @keyframes flight-in {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .shortlist__flight {
      animation: none;
    }

    .shortlist__list {
      transition: none;
    }

    .shortlist__list--arriving {
      opacity: 1;
      transform: none;
    }
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
