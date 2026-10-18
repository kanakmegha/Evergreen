<script lang="ts">
  import Icon from './Icon.svelte';

  type Props = {
    /** Radio group name — must be shared across the group. */
    name: string;
    value: string;
    /** Currently selected value in the group, or null. */
    selected: string | null;
    label: string;
    description?: string;
    icon?: string;
    badge?: string;
    /** Compact layout for long lists such as the 27 nakshatras. */
    dense?: boolean;
    onselect: (value: string) => void;
  };

  let { name, value, selected, label, description, icon, badge, dense = false, onselect }: Props =
    $props();

  const isSelected = $derived(selected === value);
</script>

<label class="choice" class:choice--selected={isSelected} class:choice--dense={dense}>
  <input
    class="choice__input"
    type="radio"
    {name}
    {value}
    checked={isSelected}
    onchange={() => onselect(value)}
  />
  {#if icon}
    <span class="choice__icon"><Icon name={icon} size={22} /></span>
  {/if}
  <span class="choice__body">
    <span class="choice__label">
      {label}
      {#if badge}<span class="choice__badge">{badge}</span>{/if}
    </span>
    {#if description}<span class="choice__desc">{description}</span>{/if}
  </span>
  <span class="choice__tick" aria-hidden="true"><Icon name="check" size={15} strokeWidth={2.4} /></span>
</label>

<style>
  .choice {
    position: relative;
    display: flex;
    align-items: flex-start;
    gap: 0.9rem;
    width: 100%;
    min-height: 76px;
    padding: 1.05rem 1.15rem;
    background: var(--surface);
    border: 1.5px solid var(--color-line);
    border-radius: var(--radius-card);
    cursor: pointer;
    transition: border-color 0.16s ease, background-color 0.16s ease,
      box-shadow 0.16s ease, transform 0.16s ease;
  }

  .choice:hover {
    border-color: #bcc8b6;
    background: #fbfcf8;
  }

  .choice:focus-within {
    outline: 3px solid rgba(31, 111, 82, 0.45);
    outline-offset: 2px;
  }

  .choice--selected {
    border-color: var(--color-forest);
    background: #f4f8f3;
    box-shadow: 0 1px 0 rgba(36, 79, 61, 0.05),
      0 16px 32px -26px rgba(36, 79, 61, 0.55);
  }

  .choice__input {
    position: absolute;
    opacity: 0;
    width: 1px;
    height: 1px;
    margin: 0;
    pointer-events: none;
  }

  .choice__icon {
    display: grid;
    place-items: center;
    flex: none;
    width: 44px;
    height: 44px;
    border-radius: 12px;
    background: #eef2e8;
    color: var(--color-forest);
    border: 1px solid #dde4d6;
  }

  .choice--selected .choice__icon {
    background: var(--color-forest);
    color: #fff;
    border-color: var(--color-forest);
  }

  .choice__body {
    display: flex;
    flex-direction: column;
    gap: 0.28rem;
    min-width: 0;
    flex: 1;
    padding-top: 0.05rem;
  }

  .choice__label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    font-weight: 700;
    font-size: 1.02rem;
    color: var(--color-ink);
    line-height: 1.35;
  }

  .choice__badge {
    font-size: 0.68rem;
    font-weight: 650;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-charcoal-soft);
    background: #f0efe4;
    border: 1px solid var(--color-line);
    border-radius: var(--radius-pill);
    padding: 0.1rem 0.5rem;
  }

  .choice__desc {
    font-size: 0.94rem;
    line-height: 1.6;
    color: var(--color-charcoal-soft);
  }

  .choice__tick {
    flex: none;
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    margin-top: 0.15rem;
    border-radius: 50%;
    border: 1.5px solid #cdd5c7;
    color: transparent;
    background: transparent;
    transition: color 0.16s ease, background-color 0.16s ease,
      border-color 0.16s ease;
  }

  .choice--selected .choice__tick {
    background: var(--color-forest);
    border-color: var(--color-forest);
    color: #fff;
  }

  .choice--dense {
    min-height: 56px;
    padding: 0.7rem 0.85rem;
    gap: 0.7rem;
    border-radius: var(--radius-control);
  }

  .choice--dense .choice__label {
    font-size: 0.96rem;
  }

  .choice--dense .choice__tick {
    width: 22px;
    height: 22px;
  }

  @media (max-width: 560px) {
    .choice {
      padding: 0.9rem 0.95rem;
      gap: 0.75rem;
      min-height: 68px;
    }

    .choice--dense {
      min-height: 52px;
      padding: 0.65rem 0.75rem;
    }

    .choice__icon {
      width: 38px;
      height: 38px;
      border-radius: 10px;
    }

    .choice__label {
      font-size: 0.98rem;
    }
  }
</style>
