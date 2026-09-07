<template>
  <component
    :is="as"
    class="glass-card"
    :class="{ 'glass-card--interactive': interactive }"
    @pointermove="moveGlow"
    @pointerleave="resetGlow"
    @pointercancel="resetGlow"
  >
    <slot />
  </component>
</template>

<script setup lang="ts">
  const props = withDefaults(
    defineProps<{
      as?: 'article' | 'section' | 'div' | 'li' | 'form'
      interactive?: boolean
    }>(),
    {
      as: 'article',
      interactive: true
    }
  )

  const motionAllowed =
    typeof window === 'undefined'
      ? null
      : window.matchMedia(
          '(hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference)'
        )

  function moveGlow(event: PointerEvent) {
    if (!props.interactive || event.pointerType !== 'mouse' || !motionAllowed?.matches) return

    const card = event.currentTarget as HTMLElement
    const { left, top, width, height } = card.getBoundingClientRect()
    // Update only the painted surface, without triggering a Vue render on each pointer move.
    card.style.setProperty('--glow-x', `${((event.clientX - left) / width) * 100}%`)
    card.style.setProperty('--glow-y', `${((event.clientY - top) / height) * 100}%`)
  }

  function resetGlow(event: PointerEvent) {
    const card = event.currentTarget as HTMLElement
    card.style.removeProperty('--glow-x')
    card.style.removeProperty('--glow-y')
  }
</script>

<style scoped>
  .glass-card {
    position: relative;
    isolation: isolate;
    min-width: 0;
    border: 1px solid var(--glass-border);
    border-radius: var(--card-radius);
    background: var(--surface-solid);
    box-shadow: var(--card-shadow);
    transform: var(--card-rest-transform, none);
    transition:
      transform var(--motion-duration) var(--motion-ease),
      border-color var(--motion-duration) var(--motion-ease),
      box-shadow var(--motion-duration) var(--motion-ease);
  }

  /* Negative layers stay above the isolated background and below all slotted content. */
  .glass-card::before,
  .glass-card::after {
    position: absolute;
    z-index: -1;
    inset: 0;
    border-radius: inherit;
    content: '';
    pointer-events: none;
  }

  .glass-card::before {
    background: var(--glass-overlay);
  }

  .glass-card::after {
    background: radial-gradient(
      circle var(--glow-radius) at var(--glow-x, 50%) var(--glow-y, 0%),
      var(--glow-color),
      transparent 75%
    );
    opacity: 0;
    transition: opacity var(--motion-duration) var(--motion-ease);
  }

  .glass-card:focus-within {
    border-color: var(--glass-border-hover);
    box-shadow: var(--card-shadow-hover);
  }

  @supports ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
    .glass-card {
      background: var(--surface-glass);
      -webkit-backdrop-filter: blur(var(--glass-blur)) saturate(135%);
      backdrop-filter: blur(var(--glass-blur)) saturate(135%);
    }
  }

  @media (hover: hover) and (pointer: fine) {
    .glass-card--interactive:hover {
      border-color: var(--glass-border-hover);
      box-shadow: var(--card-shadow-hover);
    }
  }

  @media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
    .glass-card--interactive:hover {
      transform: var(--card-rest-transform, translateY(0)) translateY(var(--card-hover-lift))
        scale(var(--card-hover-scale));
    }

    .glass-card--interactive:hover::after {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .glass-card,
    .glass-card::after {
      transition: none;
    }

    .glass-card::after {
      display: none;
    }
  }

  @media (prefers-reduced-transparency: reduce) {
    .glass-card {
      background: var(--surface-solid);
      -webkit-backdrop-filter: none;
      backdrop-filter: none;
    }
  }

  @media (forced-colors: active) {
    .glass-card {
      border-color: CanvasText;
      box-shadow: none;
    }

    .glass-card::before,
    .glass-card::after {
      display: none;
    }
  }
</style>
