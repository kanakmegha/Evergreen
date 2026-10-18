<script lang="ts">
  /**
   * Inline botanical/UI glyph set.
   *
   * The project intentionally has no icon dependency, so these are hand-authored
   * 24x24 stroke paths that inherit `currentColor`. Rendered through `{@html}`
   * with a fully static, internal string so nothing user-supplied reaches the DOM.
   */
  type Props = {
    name: keyof typeof ICONS | string;
    size?: number;
    class?: string;
    strokeWidth?: number;
  };

  let { name, size = 22, class: klass = '', strokeWidth = 1.6 }: Props = $props();

  const ICONS: Record<string, string> = {
    sprout:
      '<path d="M12 21v-6"/><path d="M12 15C12 11 9 8 4 8c0 4 3 7 8 7Z"/><path d="M12 15c0-3.4 2.7-6 7-6 0 3.4-2.7 6-7 6Z"/>',
    heart:
      '<path d="M12 20.5S3.8 15.6 3.8 10.2A4.2 4.2 0 0 1 12 8.3a4.2 4.2 0 0 1 8.2 1.9c0 5.4-8.2 10.3-8.2 10.3Z"/>',
    globe:
      '<circle cx="12" cy="12" r="8.6"/><path d="M3.4 12h17.2"/><path d="M12 3.4c2.3 2.4 3.5 5.4 3.5 8.6s-1.2 6.2-3.5 8.6c-2.3-2.4-3.5-5.4-3.5-8.6S9.7 5.8 12 3.4Z"/>',
    gift:
      '<path d="M4 11.5h16V20H4z"/><path d="M12 11.5V20"/><path d="M3 7.6h18v3.9H3z"/><path d="M12 7.5C10.8 4.7 9.6 3.4 8.2 3.6 6.9 3.8 6.6 5.7 8 6.7c1.2.9 2.6 1 4 .8Z"/><path d="M12 7.5c1.2-2.8 2.4-4.1 3.8-3.9 1.3.2 1.6 2.1.2 3.1-1.2.9-2.6 1-4 .8Z"/>',
    compass:
      '<circle cx="12" cy="12" r="8.6"/><path d="m15.6 8.4-2.1 5.1-5.1 2.1 2.1-5.1z"/>',
    leaf:
      '<path d="M4.5 19.5C4.5 11 11 4.5 19.5 4.5c0 8.5-6.5 15-15 15Z"/><path d="M4.5 19.5C7.5 14 12 10 17 8"/>',
    tree:
      '<path d="M12 21v-4.5"/><path d="M12 16.5c-3.6 0-6-2.3-6-5.2 0-1.3.6-2.5 1.5-3.3-.1-3 2-5.5 4.5-5.5s4.6 2.5 4.5 5.5c.9.8 1.5 2 1.5 3.3 0 2.9-2.4 5.2-6 5.2Z"/>',
    seed: '<path d="M12 3.5c3.2 3.4 5 6.1 5 8.6a5 5 0 0 1-10 0c0-2.5 1.8-5.2 5-8.6Z"/><path d="M12 21v-4"/>',
    book:
      '<path d="M5 4.5h9.5A3.5 3.5 0 0 1 18 8v11.5H8.5A3.5 3.5 0 0 1 5 16V4.5Z"/><path d="M8.5 19.5A3.5 3.5 0 0 1 12 16h6"/><path d="M8.5 8.5h6"/>',
    search:
      '<circle cx="11" cy="11" r="6.6"/><path d="m16 16 4.5 4.5"/>',
    check: '<path d="M20 6.5 9.5 17.5 4 12"/>',
    'arrow-right': '<path d="M4.5 12h15"/><path d="m13 5.8 6.2 6.2-6.2 6.2"/>',
    'arrow-left': '<path d="M19.5 12h-15"/><path d="M11 5.8 4.8 12 11 18.2"/>',
    menu: '<path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h16"/>',
    close: '<path d="M6.2 6.2 17.8 17.8"/><path d="M17.8 6.2 6.2 17.8"/>',
    info: '<circle cx="12" cy="12" r="8.6"/><path d="M12 11v5.2"/><path d="M12 8.1h.01"/>',
    alert: '<path d="M12 3.6 21 19.8H3Z"/><path d="M12 10v4"/><path d="M12 17h.01"/>',
    sun: '<circle cx="12" cy="12" r="4.2"/><path d="M12 2.6v2.2M12 19.2v2.2M2.6 12h2.2M19.2 12h2.2M5.4 5.4l1.6 1.6M17 17l1.6 1.6M18.6 5.4 17 7M7 17l-1.6 1.6"/>',
    shield:
      '<path d="M12 3.5 19 6v5.6c0 4-3 7.1-7 8.9-4-1.8-7-4.9-7-8.9V6Z"/><path d="m9.2 12 2 2 3.6-3.8"/>',
    scale:
      '<path d="M12 4v16"/><path d="M6 8h12"/><path d="M7.5 8 4.5 14h6Z"/><path d="M16.5 8l-3 6h6Z"/>',
    sparkle:
      '<path d="M10.5 3.5 12 8l4.5 1.5L12 11l-1.5 4.5L9 11 4.5 9.5 9 8Z"/><path d="M17.5 15.5l.7 2.1 2.1.7-2.1.7-.7 2.1-.7-2.1-2.1-.7 2.1-.7Z"/>',
    layers:
      '<path d="M12 3.5 21 8l-9 4.5L3 8Z"/><path d="m3 12.5 9 4.5 9-4.5"/><path d="m3 16.5 9 4.5 9-4.5"/>',
  };

  function markup(n: string, s: number, extra: string, sw: number): string {
    const cls = extra.replace(/[^a-zA-Z0-9 _-]/g, '');
    const body = ICONS[n] ?? '';
    return `<svg viewBox="0 0 24 24" width="${s}" height="${s}" fill="none" stroke="currentColor" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false" class="${cls}">${body}</svg>`;
  }
</script>

{@html markup(String(name), size, klass, strokeWidth)}
