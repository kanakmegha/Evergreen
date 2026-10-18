<script lang="ts">
  import Icon from './Icon.svelte';
  import type { ExplanationEnvelope } from '../discovery';
  import type { TreeOptionView } from '../treeOption';

  type Props = {
    option: TreeOptionView;
    open?: boolean;
    saved?: boolean;
    /** Flat styling for a card that already sits inside a bordered row. */
    nested?: boolean;
    onToggleOpen?: (key: string) => void;
    onToggleSave?: (option: TreeOptionView) => void;
    summary?: ExplanationEnvelope | null;
    summaryLoading?: boolean;
    /** Plain-language provider state. Never a claim about missing records. */
    summaryNotice?: string;
    onRequestSummary?: (option: TreeOptionView) => void;
  };

  let {
    option,
    open = false,
    saved = false,
    nested = false,
    onToggleOpen,
    onToggleSave,
    summary = null,
    summaryLoading = false,
    summaryNotice = '',
    onRequestSummary,
  }: Props = $props();

  const panelId = $derived(`option-panel-${option.key}`);
  const titleId = $derived(`option-title-${option.key}`);
</script>

<article class="opt" class:card-surface={!nested} class:opt--nested={nested} aria-labelledby={titleId}>
  <div class="opt__main">
    {#if !nested}
      <span class="opt__mark" aria-hidden="true">{option.mark}</span>
    {/if}

    <div class="opt__identity">
      {#if option.tags.length}
        <p class="opt__tags">
          {#each option.tags as tag}
            <span
              class="tag"
              class:tag--neutral={tag.tone === 'neutral'}
              class:tag--pending={tag.tone === 'pending'}
              class:tag--unknown={tag.tone === 'unknown'}>{tag.text}</span
            >
          {/each}
        </p>
      {/if}

      <h3 id={titleId} class="opt__title" class:opt__title--botanical={option.titleIsScientific}>
        {option.title}
      </h3>
      {#if !option.titleIsScientific && option.scientificName}
        <p class="opt__sci">{option.scientificName}{#if option.authorship}<span class="opt__authorship">{` ${option.authorship}`}</span>{/if}</p>
      {/if}

      <p class="opt__why">{option.whyIncluded}</p>
    </div>
  </div>

  <div class="opt__actions">
    {#if onToggleOpen}
      <button
        class="btn btn--primary btn--md"
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onclick={() => onToggleOpen(option.key)}
      >
        {open ? 'Hide details' : 'Explore this tree'}
      </button>
    {/if}
    {#if onToggleSave}
      <button
        class="btn btn--ghost btn--md"
        type="button"
        aria-pressed={saved}
        onclick={() => onToggleSave(option)}
      >
        <Icon name={saved ? 'check' : 'leaf'} size={18} />
        {saved ? 'Saved on this device' : 'Save this tree'}
      </button>
    {/if}
  </div>

  {#if open}
    <div class="opt__more" id={panelId}>
      <h4 class="opt__more-title">Why this option?</h4>

      <dl class="opt__more-list">
        <dt>Why it was included</dt>
        <dd>{option.whyIncluded}</dd>

        <dt>What is actually known</dt>
        <dd>
          {#if option.facts.length}
            <ul class="opt__facts">
              {#each option.facts as fact}
                <li><span class="opt__fact-label">{fact.label}</span> {fact.value}</li>
              {/each}
            </ul>
          {:else}
            No botanical facts are on record for this option.
          {/if}
          {#if option.otherNames.length}
            <p class="opt__fact-note">Also recorded as {option.otherNames.join(' · ')}</p>
          {/if}
        </dd>

        <dt>Cultural association</dt>
        <dd>
          <span class="opt__verdict" class:opt__verdict--verified={option.culturalVerified}>
            <Icon name={option.culturalVerified ? 'check' : 'info'} size={15} />
            {option.culturalVerified ? 'Verified cultural association' : 'Not a cultural association'}
          </span>
          <p class="opt__fact-note">{option.culturalNote}</p>
        </dd>

        <dt>Source</dt>
        <dd>
          {#if option.sources.length}
            <ul class="opt__sources">
              {#each option.sources as source}
                <li>
                  <p class="opt__source-title">
                    {#if source.url}
                      <a href={source.url} target="_blank" rel="noopener noreferrer nofollow"
                        >{source.title}</a
                      >
                    {:else}
                      {source.title}
                    {/if}
                  </p>
                  {#if source.meta}<p class="opt__fact-note">{source.meta}</p>{/if}
                  {#if source.tags.length}
                    <p class="opt__source-tags">
                      {#each source.tags as tag}
                        <span class="tag tag--neutral">{tag}</span>
                      {/each}
                    </p>
                  {/if}
                  {#if source.attribution}
                    <p class="opt__fact-note">{source.attribution}</p>
                  {/if}
                </li>
              {/each}
            </ul>
          {:else}
            <p class="opt__fact-note">
              No source is attached to this option, so it should not be treated as verified.
            </p>
          {/if}
        </dd>

        {#if option.gaps.length}
          <dt>Not on record yet</dt>
          <dd>
            <ul class="opt__gaps">
              {#each option.gaps as gap}
                <li>{gap}</li>
              {/each}
            </ul>
          </dd>
        {/if}
      </dl>

      {#if onRequestSummary}
        <div class="opt__summary">
          {#if summaryLoading}
            <p class="opt__fact-note" aria-live="polite">
              <span class="spinner spinner--xs" aria-hidden="true"></span> Asking for a plain-language
              summary…
            </p>
          {:else if summary?.status === 'ok'}
            <p class="opt__summary-label">
              <Icon name="sparkle" size={15} /> AI-written from the facts above
            </p>
            <p class="opt__summary-text">{summary.text}</p>
            <p class="opt__fact-note">
              Machine-written and not a reviewed record. It adds no facts beyond the ones above.
            </p>
          {:else if summaryNotice}
            <p class="opt__fact-note" aria-live="polite">{summaryNotice}</p>
          {:else}
            <button
              class="btn btn--soft btn--sm"
              type="button"
              onclick={() => onRequestSummary(option)}
            >
              <Icon name="sparkle" size={16} /> Ask for a plain-language summary
            </button>
          {/if}
        </div>
      {/if}

      {#if option.referenceHref}
        <p class="opt__reference">
          <a href={option.referenceHref}>{option.referenceLabel}</a>
        </p>
      {/if}
    </div>
  {/if}
</article>

<style>
  .opt {
    display: grid;
    gap: 0.9rem;
    padding: clamp(1rem, 2.4vw, 1.35rem);
  }

  /* Inside the catalogue a row already provides the border and the padding. */
  .opt--nested {
    padding: 0.75rem 0.35rem 0.35rem;
    gap: 0.7rem;
  }

  .opt__main {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.9rem;
    align-items: start;
  }

  .opt--nested .opt__main {
    grid-template-columns: minmax(0, 1fr);
  }

  .opt__mark {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: #eef2e8;
    border: 1px solid #dbe3d5;
    color: var(--color-forest-soft);
    font-family: var(--font-display);
    font-size: 1.05rem;
    line-height: 1;
    user-select: none;
  }

  .opt__identity {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }

  .opt__tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0 0 0.1rem;
  }

  .opt__title {
    margin: 0;
    font-size: clamp(1.1rem, 2.2vw, 1.3rem);
    line-height: 1.3;
    color: var(--color-ink);
    overflow-wrap: anywhere;
  }

  .opt__title--botanical {
    font-family: var(--font-display);
    font-style: italic;
    font-weight: 500;
  }

  .opt__sci {
    margin: 0;
    font-family: var(--font-display);
    font-style: italic;
    font-size: 0.98rem;
    color: var(--color-forest-soft);
    overflow-wrap: anywhere;
  }

  .opt__authorship {
    font-style: normal;
    font-size: 0.88em;
    color: var(--color-charcoal-soft);
  }

  /*
   * The card copy is a summary. The full claim or rule is repeated verbatim in
   * the detail panel below, so clamping here hides nothing from a reader who
   * opens the card.
   */
  .opt__why {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    overflow: hidden;
    margin: 0.3rem 0 0;
    font-size: 0.94rem;
    line-height: 1.6;
    color: var(--color-charcoal-soft);
    max-width: 62ch;
  }

  .opt__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.55rem;
  }

  /* -------------------------------------------------- expandable detail */
  .opt__more {
    display: grid;
    gap: 0.75rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-line-soft);
  }

  .opt__more-title {
    margin: 0;
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-sage);
  }

  .opt__more-list {
    margin: 0;
    display: grid;
    gap: 0.2rem;
  }

  .opt__more-list dt {
    font-size: 0.9rem;
    font-weight: 650;
    color: var(--color-forest);
    margin-top: 0.7rem;
  }

  .opt__more-list dt:first-child {
    margin-top: 0;
  }

  .opt__more-list dd {
    margin: 0.25rem 0 0;
    font-size: 0.92rem;
    line-height: 1.65;
    color: var(--color-charcoal-soft);
    overflow-wrap: anywhere;
  }

  .opt__facts,
  .opt__gaps {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.2rem;
  }

  .opt__gaps li::before {
    content: '— ';
    color: var(--color-moss);
  }

  .opt__fact-label {
    color: var(--color-ink);
    font-weight: 600;
  }

  .opt__fact-note {
    margin: 0.35rem 0 0;
    font-size: 0.88rem;
    line-height: 1.6;
    color: var(--color-charcoal-soft);
  }

  .opt__verdict {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.15rem 0.6rem;
    border-radius: var(--radius-pill);
    border: 1px solid var(--color-line);
    background: var(--surface-soft);
    font-size: 0.78rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    color: var(--color-charcoal-soft);
  }

  .opt__verdict--verified {
    color: var(--color-forest);
    border-color: #cfdcc7;
    background: #f1f6ee;
  }

  .opt__sources {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.6rem;
  }

  .opt__source-title {
    margin: 0;
    font-weight: 600;
    color: var(--color-ink);
    overflow-wrap: anywhere;
  }

  .opt__source-title a {
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .opt__source-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    margin: 0.35rem 0 0;
  }

  .opt__summary {
    display: grid;
    gap: 0.4rem;
    padding: 0.85rem 0.95rem;
    border-radius: var(--radius-control);
    background: #f7faf5;
    border: 1px solid #dde7d8;
  }

  .opt__summary-label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0;
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--color-forest);
  }

  .opt__summary-text {
    margin: 0;
    font-size: 0.98rem;
    line-height: 1.7;
    color: var(--color-ink);
  }

  .opt__reference {
    margin: 0;
    font-size: 0.88rem;
  }

  .opt__reference a {
    color: var(--color-forest);
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .spinner--xs {
    display: inline-block;
    width: 13px;
    height: 13px;
    border-width: 2px;
    vertical-align: -2px;
    margin-right: 0.4rem;
  }

  @media (max-width: 420px) {
    .opt__main {
      grid-template-columns: minmax(0, 1fr);
    }

    .opt__mark {
      display: none;
    }
  }
</style>
