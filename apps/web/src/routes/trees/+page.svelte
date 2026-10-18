<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import Icon from '../../lib/components/Icon.svelte';
  import TreeOption from '../../lib/components/TreeOption.svelte';
  import { referenceOption, filterTrees } from '../../lib/treeOption';
  import type { TreeOptionView } from '../../lib/treeOption';
  import { absoluteUrl } from '../../lib/site';
  import {
    explanationFailureText,
    fetchCatalogue,
    loadSaved,
    runExplain,
    toggleSaved,
  } from '../../lib/discovery';
  import type { ExplanationEnvelope, TreeFact } from '../../lib/discovery';

  let catalogue = $state<TreeFact[]>([]);
  let ordering = $state('');
  let loading = $state(true);
  let notice = $state('');
  let search = $state('');
  let openId = $state('');

  let saved = $state(loadSaved());
  let saveStatus = $state('');

  let explanations = $state<Record<string, ExplanationEnvelope | null>>({});
  let treeNotices = $state<Record<string, string>>({});
  let treeLoading = $state<Record<string, boolean>>({});

  const filtered = $derived(filterTrees(catalogue, search));

  async function load() {
    loading = true;
    notice = '';
    try {
      const response = await fetchCatalogue();
      catalogue = response.trees;
      ordering = response.ordering;
    } catch (error) {
      catalogue = [];
      notice = error instanceof Error ? error.message : 'The botanical reference could not be reached.';
    } finally {
      loading = false;
    }
  }

  /**
   * The visitor arrived here without answering any discovery questions, so no
   * intention is sent and no personal detail is implied. The explanation is
   * grounded on the one record they chose.
   */
  async function explainTree(tree: TreeFact) {
    const id = tree.speciesId;
    explanations = { ...explanations, [id]: null };
    treeNotices = { ...treeNotices, [id]: '' };
    treeLoading = { ...treeLoading, [id]: true };
    try {
      const response = await runExplain({
        intention: 'explore',
        tradition: '',
        nakshatra: '',
        region: '',
        speciesId: id,
      });
      explanations = { ...explanations, [id]: response.explanation };
      if (response.explanation.status !== 'ok') {
        treeNotices = { ...treeNotices, [id]: explanationFailureText(response.explanation.reason, 'tree') };
      }
    } catch (error) {
      treeNotices = {
        ...treeNotices,
        [id]:
          error instanceof Error
            ? `${error.message} No generated explanation is shown.`
            : 'The explanation service could not be reached. No generated explanation is shown.',
      };
    } finally {
      treeLoading = { ...treeLoading, [id]: false };
    }
  }

  function savedKey(tree: TreeFact): string {
    return `species:${tree.speciesId}`;
  }

  function isSaved(tree: TreeFact): boolean {
    return saved.some((entry) => entry.key === savedKey(tree));
  }

  function toggleSave(tree: TreeFact) {
    const key = savedKey(tree);
    const wasSaved = isSaved(tree);
    toggleSaved({ key, label: tree.acceptedName, scientificName: tree.acceptedName, recordId: '' });
    saved = loadSaved();
    if (isSaved(tree)) {
      saveStatus = `“${tree.acceptedName}” is saved in this browser only. Everroot does not publish or plant anything.`;
    } else if (wasSaved) {
      saveStatus = `“${tree.acceptedName}” was removed from this browser.`;
    } else {
      saveStatus =
        'This browser blocked local storage, so nothing could be saved. Private browsing can cause this.';
    }
  }

  onMount(load);
</script>

<svelte:head>
  <title>Tree reference — Everroot</title>
  <meta
    name="description"
    content="Everroot's botanical reference: accepted tree names, plant classification and the authority record behind each fact. Not a cultural association."
  />
  <link rel="canonical" href={absoluteUrl('/trees', page.url.origin)} />
</svelte:head>

<div class="page">
  <div class="container">
    <div class="page__inner">
      <header class="intro">
        <p class="eyebrow">Botanical reference</p>
        <h1 class="intro__title">Trees Everroot can name accurately</h1>
        <p class="intro__lede">
          Every entry here is a tree identity verified against an authority dataset, with the taxon
          key recorded so you can check it yourself. These are botanical records, not cultural
          associations: nothing here is matched to a nakshatra, tradition, religion or region, and
          Everroot does not claim otherwise.
        </p>
        <p class="intro__lede intro__lede--muted">
          Where Everroot has no reviewed record — habitat, native range, ecological role, or
          documented cultural and historical stories — it says so rather than filling the gap.
        </p>
      </header>

      <section class="browse card-surface" aria-labelledby="browse-title">
        <h2 id="browse-title" class="browse__title">Browse the reference</h2>
        <p class="browse__note">
          {#if ordering}
            Ordered by {ordering}. There is no ranking here, and nothing is suggested as being
            meant for you.
          {:else}
            Ordered, not ranked.
          {/if}
        </p>

        <div class="field browse__search">
          <label for="reference-search">Search by common name, scientific name or family</label>
          <input
            id="reference-search"
            class="input"
            type="search"
            bind:value={search}
            placeholder="e.g. neem, Ficus, Fabaceae"
            autocomplete="off"
          />
        </div>

        {#if notice}
          <p class="browse__note browse__note--notice" role="status">
            {notice}
            <button class="btn btn--soft btn--sm" type="button" onclick={load}>Try again</button>
          </p>
        {:else if loading}
          <p class="browse__note" role="status">Loading the botanical reference…</p>
        {:else if filtered.length === 0}
          <p class="browse__note" role="status">
            {catalogue.length === 0
              ? 'The botanical reference has no records yet. Everroot will not invent one to fill the space.'
              : `No record matches “${search.trim()}”. Try a scientific name, a common name, or a family such as Fabaceae.`}
          </p>
        {:else}
          <ul class="list">
            {#each filtered as tree, index (tree.speciesId)}
              {@const option = referenceOption(tree, index, 'browse', false)}
              {@const open = openId === tree.speciesId}
              <li class="item">
                <button
                  class="row"
                  type="button"
                  aria-expanded={open}
                  aria-controls={`reference-panel-${tree.speciesId}`}
                  onclick={() => (openId = open ? '' : tree.speciesId)}
                >
                  <span class="row__name">
                    {option.title}
                    {#if !option.titleIsScientific}
                      <span class="row__sci">{tree.acceptedName}</span>
                    {/if}
                  </span>
                  <span class="row__meta">
                    {tree.family}
                    {#if tree.commonNames?.length} · {tree.commonNames
                        .slice(0, 2)
                        .map((entry) => entry.name)
                        .join(', ')}{/if}
                  </span>
                  <span class="row__caret" aria-hidden="true">{open ? '–' : '+'}</span>
                </button>
                {#if open}
                  <div class="panel" id={`reference-panel-${tree.speciesId}`}>
                    <TreeOption
                      {option}
                      nested
                      open
                      saved={isSaved(tree)}
                      summary={explanations[tree.speciesId] ?? null}
                      summaryLoading={treeLoading[tree.speciesId] ?? false}
                      summaryNotice={treeNotices[tree.speciesId] ?? ''}
                      onRequestSummary={(view: TreeOptionView) =>
                        view.tree && explainTree(view.tree)}
                      onToggleSave={(view: TreeOptionView) => view.tree && toggleSave(view.tree)}
                    />
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}

        {#if saveStatus}
          <p class="browse__note" role="status">{saveStatus}</p>
        {/if}
      </section>

      <div class="actions">
        <a class="btn btn--primary btn--md" href="/discover">
          <Icon name="compass" size={18} /> Find my tree
        </a>
        <a class="btn btn--soft btn--md" href="/">Back to home</a>
      </div>
    </div>
  </div>
</div>

<style>
  .page {
    padding-block: clamp(1.75rem, 4vw, 3rem) var(--space-section);
    background: var(--color-ivory);
    min-height: 66vh;
  }

  .page__inner {
    max-width: 900px;
    margin-inline: auto;
    display: grid;
    gap: 1.5rem;
  }

  .intro__title {
    margin-top: 0.5rem;
    font-size: clamp(1.6rem, 3.4vw, 2.2rem);
    line-height: 1.2;
    letter-spacing: -0.02em;
  }

  .intro__lede {
    margin-top: 0.85rem;
    max-width: 68ch;
    color: var(--color-charcoal-soft);
    line-height: 1.7;
  }

  .intro__lede--muted {
    font-size: 0.95rem;
  }

  .browse {
    padding: clamp(1.15rem, 3vw, 1.75rem);
    display: grid;
    gap: 0.75rem;
  }

  .browse__title {
    font-size: clamp(1.15rem, 2.4vw, 1.4rem);
    margin: 0;
  }

  .browse__note {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.65;
    color: var(--color-charcoal-soft);
  }

  .browse__note--notice {
    padding: 0.7rem 0.85rem;
    border-radius: var(--radius-control);
    background: #f6f4ea;
    border: 1px solid var(--color-line);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .browse__search {
    margin-top: 0.35rem;
  }

  .list {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.4rem;
  }

  .item {
    border: 1px solid var(--color-line);
    border-radius: var(--radius-control);
    background: var(--surface);
    overflow: hidden;
  }

  .row {
    display: grid;
    grid-template-columns: 1fr auto;
    grid-template-areas: 'name caret' 'meta caret';
    align-items: center;
    gap: 0.15rem 0.75rem;
    width: 100%;
    padding: 0.7rem 0.85rem;
    background: transparent;
    border: 0;
    text-align: left;
    cursor: pointer;
    font: inherit;
  }

  .row:hover {
    background: #fafbf6;
  }

  .row:focus-visible {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: -2px;
  }

  .row__name {
    grid-area: name;
    font-size: 1rem;
    color: var(--color-ink);
    overflow-wrap: anywhere;
  }

  .row__sci {
    font-family: var(--font-display);
    font-style: italic;
    font-size: 0.88rem;
    color: var(--color-forest-soft);
    margin-left: 0.35rem;
  }

  .row__meta {
    grid-area: meta;
    font-size: 0.84rem;
    color: var(--color-charcoal-soft);
    overflow-wrap: anywhere;
  }

  .row__caret {
    grid-area: caret;
    font-weight: 700;
    font-size: 1.1rem;
    color: var(--color-forest-soft);
  }

  .panel {
    padding: 0 0.5rem 0.5rem;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.65rem;
  }
</style>
