/*
 * Mantissa Design System — Shared Tailwind Config
 * v1.0 | Source of truth for Tailwind theme configuration
 *
 * Usage: <script src="theme.js"></script> (AFTER Tailwind CDN, BEFORE nav.js)
 * CSS variables: <link rel="stylesheet" href="base.css">
 *
 * Spec:  work/topics/2026-03-08_design-system/DESIGN_SYSTEM.md
 */
tailwind.config = {
  darkMode: 'class',
  theme: { extend: {
    colors: {
      page:'var(--c-page)',card:'var(--c-card)',ink:'var(--c-ink)',muted:'var(--c-muted)',
      faint:'var(--c-faint)',subtle:'var(--c-subtle)',highlight:'var(--c-highlight)',
      footer:'var(--c-footer)',coal:'#191918','warm-gold':'#C4B590',
      teal:{50:'#EDF8F5',100:'#D9F0EC',200:'#B8E0D9',300:'#8ECDC2',400:'#66B5A8',500:'#3D9E8F',600:'#2D8B7D',700:'#1A7A6C',800:'#14655A',900:'#0D5C52'},
      terracotta:'#C75D4A',amber:'#D4956A',
    },
    fontFamily: { serif:['Newsreader','Georgia','serif'], sans:['Inter','system-ui','-apple-system','sans-serif'], mono:['"JetBrains Mono"','"SF Mono"','Consolas','monospace'] },
    fontSize: { 'xs':['0.75rem',{lineHeight:'1.3'}],'sm':['0.875rem',{lineHeight:'1.5'}],'base':['1.0625rem',{lineHeight:'1.6'}],'lg':['1.25rem',{lineHeight:'1.5'}],'xl':['1.5rem',{lineHeight:'1.3'}],'2xl':['2rem',{lineHeight:'1.2',letterSpacing:'-0.01em'}],'3xl':['2.5rem',{lineHeight:'1.15',letterSpacing:'-0.015em'}],'4xl':['3.5rem',{lineHeight:'1.05',letterSpacing:'-0.02em'}] },
    borderRadius: { 'card':'5px','pill':'999px' },
    maxWidth: { 'page':'1200px','text':'720px' },
    letterSpacing: { 'tighter':'-0.02em','wide':'0.08em' },
  }}
};
