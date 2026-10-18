<script lang="ts">
  type Step = { id: string; label: string };

  type Props = {
    steps: Step[];
    /** Zero-based index of the active step. */
    current: number;
    /** Called when the user returns to an already-completed step. */
    ongo?: (index: number) => void;
  };

  let { steps, current, ongo }: Props = $props();

  const total = $derived(steps.length);
</script>

<nav class="steps" aria-label="Discovery progress">
  <ol class="steps__list">
    {#each steps as step, i (step.id)}
      <li
        class="steps__item"
        class:steps__item--done={i < current}
        class:steps__item--active={i === current}
      >
        {#if i < current && ongo}
          <button
            type="button"
            class="steps__control"
            onclick={() => ongo(i)}
            aria-label={`Go back to step ${i + 1} of ${total}: ${step.label}`}
          >
            <span class="steps__dot" aria-hidden="true">
              <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6.5 9.5 17.5 4 12"/></svg>
            </span>
            <span class="steps__label">{step.label}</span>
          </button>
        {:else}
          <span class="steps__control steps__control--static">
            <span class="steps__dot" aria-hidden="true">{i + 1}</span>
            <span class="steps__label" aria-current={i === current ? 'step' : undefined}>{step.label}</span>
          </span>
        {/if}
      </li>
    {/each}
  </ol>
</nav>

<style>
  .steps__list {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    list-style: none;
    margin: 0;
    padding: 0;
    flex-wrap: wrap;
  }

  .steps__item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .steps__item + .steps__item::before {
    content: '';
    display: block;
    width: clamp(14px, 3vw, 34px);
    height: 1px;
    background: var(--color-line);
  }

  .steps__control {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    background: none;
    border: 0;
    padding: 0.25rem 0.2rem;
    border-radius: 8px;
    color: var(--color-charcoal-soft);
    font-size: 0.85rem;
    font-weight: 650;
    cursor: pointer;
  }

  .steps__control--static {
    cursor: default;
  }

  button.steps__control:hover {
    color: var(--color-forest);
    background: rgba(36, 79, 61, 0.07);
  }

  .steps__dot {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: 1.5px solid var(--color-line);
    background: var(--surface);
    font-size: 0.78rem;
    font-weight: 700;
    color: var(--color-charcoal-soft);
    flex: none;
  }

  .steps__item--done .steps__dot {
    background: var(--color-forest);
    border-color: var(--color-forest);
    color: #fff;
  }

  .steps__item--active .steps__dot {
    border-color: var(--color-forest);
    color: var(--color-forest);
    box-shadow: 0 0 0 3px rgba(36, 79, 61, 0.14);
  }

  .steps__item--active .steps__label {
    color: var(--color-ink);
  }

  @media (max-width: 620px) {
    .steps__label {
      display: none;
    }

    .steps__item--active .steps__label {
      display: inline;
    }

    .steps__list {
      gap: 0.35rem;
    }
  }
</style>
