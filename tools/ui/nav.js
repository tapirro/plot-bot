/**
 * Mantissa Design System — Shared Navigation
 * Injects header nav + footer + rate widget into every page.
 *
 * Usage: <script src="/nav.js" defer></script>
 * Expects: <body> has <main> element. Nav is inserted before <main>, footer after.
 * Expects: Tailwind + theme CSS variables already loaded.
 */
(function () {

  // -- Pages (clean URLs) --
  const pages = [
    { href: '/navigator',   label: 'Navigator' },
    { href: '/strategy2',   label: 'Strategy' },
    { href: '/tracker',     label: 'Tracker' },
    { href: '/quality',     label: 'Quality' },
    { href: '/digest',      label: 'Digest' },
    { href: '/graph',       label: 'Graph' },
    { href: '/observatory', label: 'Observatory' },
    { href: '/intake',      label: 'Intake' },
    { href: '/xray',        label: 'X-Ray' },
    { href: '/domains',     label: 'Domains' },
    { href: '/queue',       label: 'Queue' },
    { href: '/chains',      label: 'Chains' },
    { href: '/chains2',     label: 'Chains v2' },
  ];

  const currentPath = '/' + location.pathname.replace(/^\//, '').replace(/\.html$/, '');

  // -- Theme --
  const html = document.documentElement;
  if (localStorage.getItem('mantissa-theme') === 'dark' ||
      (!localStorage.getItem('mantissa-theme') && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    html.classList.add('dark');
  }

  // -- Header --
  const header = document.createElement('header');
  header.className = 'border-b border-subtle';
  header.innerHTML = `
    <div class="max-w-page mx-auto px-8 h-14 flex items-center justify-between">
      <div class="flex items-center gap-8">
        <a href="/navigator" class="font-serif font-bold text-lg tracking-tighter text-ink hover:text-ink no-underline">Mantissa</a>
        <nav class="flex items-center gap-6">
          ${pages.map(p => {
            const active = currentPath === p.href;
            return `<a href="${p.href}" class="text-sm no-underline transition-colors ${active ? 'text-ink font-medium' : 'text-muted hover:text-ink'}">${p.label}</a>`;
          }).join('')}
        </nav>
      </div>
      <div class="flex items-center gap-3">
        <div id="rate-pill" class="hidden items-center gap-1.5 cursor-pointer group" title="Claude budget">
          <span id="rate-dot" class="inline-block w-2 h-2 rounded-full"></span>
          <span id="rate-label" class="text-xs font-mono text-muted group-hover:text-ink transition-colors"></span>
        </div>
        <span id="nav-status-dot" class="inline-block w-1.5 h-1.5 rounded-full bg-faint"></span>
        <button id="theme-toggle" class="w-8 h-8 flex items-center justify-center rounded-full hover:bg-highlight transition-colors" aria-label="Toggle theme">
          <svg class="w-4 h-4 text-muted dark:hidden" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z"/>
          </svg>
          <svg class="w-4 h-4 text-muted hidden dark:block" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z"/>
          </svg>
        </button>
      </div>
    </div>`;

  // -- Footer --
  const footer = document.createElement('footer');
  footer.className = 'bg-footer mt-16';
  footer.innerHTML = `
    <div class="max-w-page mx-auto px-8 py-5 flex items-center justify-between">
      <span class="text-sm" style="color:#7C7C87">Mantissa Lab</span>
      <div class="flex items-center gap-4">
        ${pages.map(p => `<a href="${p.href}" class="text-xs no-underline transition-colors" style="color:#5E5E5E">${p.label}</a>`).join('')}
      </div>
    </div>`;

  // -- Inject --
  const main = document.querySelector('main');
  if (main) {
    main.parentNode.insertBefore(header, main);
    main.parentNode.insertBefore(footer, main.nextSibling);
  } else {
    document.body.prepend(header);
    document.body.append(footer);
  }

  // -- Toggle handler --
  document.getElementById('theme-toggle').addEventListener('click', () => {
    html.classList.add('transitioning');
    html.classList.toggle('dark');
    localStorage.setItem('mantissa-theme', html.classList.contains('dark') ? 'dark' : 'light');
    setTimeout(() => html.classList.remove('transitioning'), 350);
  });

  // -- Status dot (exposed for pages to mark as connected) --
  window.__navReady = () => {
    const dot = document.getElementById('nav-status-dot');
    if (dot) { dot.classList.remove('bg-faint'); dot.classList.add('bg-teal-500'); }
  };

  // -- Rate widget (prefers Anthropic usage API, falls back to calculated) --
  Promise.all([
    fetch('/api/usage').then(r => r.ok ? r.json() : null).catch(() => null),
    fetch('/api/ask/rate').then(r => r.ok ? r.json() : null).catch(() => null),
  ]).then(([usage, rate]) => {
    const hasUsage = usage && !usage.error && usage.seven_day;
    const hasRate = rate && rate.windows;
    if (!hasUsage && !hasRate) return;

    const pct = hasUsage ? (usage.seven_day.utilization || 0) : (hasRate ? (rate.windows['7d']?.pct || 0) : 0);
    const mode = hasRate ? (rate.mode || '?') : '?';
    const daily = hasRate ? (rate.windows['7d']?.daily_budget || 0) : 0;
    const src = hasUsage ? 'Anthropic' : 'calc';

    const pill = document.getElementById('rate-pill');
    const dot = document.getElementById('rate-dot');
    const label = document.getElementById('rate-label');
    pill.classList.remove('hidden');
    pill.classList.add('flex');
    dot.className = 'inline-block w-2 h-2 rounded-full ' +
      (pct > 85 ? 'bg-terracotta' : pct > 60 ? 'bg-amber' : 'bg-teal-500');
    label.textContent = pct.toFixed(0) + '%';
    pill.title = `${mode} mode · ${pct.toFixed(1)}% weekly (${src}) · $${daily.toFixed(0)}/day`;
  });

  // -- Deep link handler: style and intercept entity links --
  function initDeepLinks() {
    document.addEventListener('click', function(e) {
      const a = e.target.closest('a[href]');
      if (!a) return;
      const href = a.getAttribute('href');
      if (!href) return;

      // /j/ → Jira (open in new tab)
      if (href.startsWith('/j/')) {
        e.preventDefault();
        window.open(href, '_blank');
        return;
      }
      // /f/ → Fireflies (open in new tab)
      if (href.startsWith('/f/')) {
        e.preventDefault();
        window.open(href, '_blank');
        return;
      }
      // /a/ → artifact (navigate internally)
      if (href.startsWith('/a/')) {
        // let the redirect happen naturally
        return;
      }
      // /p/ → person (navigate internally)
      if (href.startsWith('/p/')) {
        return;
      }
    });

    // Auto-style deep links in rendered content
    styleDeepLinks(document.body);

    // Observe for dynamically added content
    const observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        for (const node of m.addedNodes) {
          if (node.nodeType === 1) styleDeepLinks(node);
        }
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  function styleDeepLinks(root) {
    const links = root.querySelectorAll ? root.querySelectorAll('a[href]') : [];
    for (const a of links) {
      const href = a.getAttribute('href');
      if (!href || a.dataset.dlStyled) continue;
      a.dataset.dlStyled = '1';

      let icon = '', cls = '';
      if (href.startsWith('/j/')) {
        icon = '<svg class="inline w-3 h-3 mr-0.5 opacity-60" viewBox="0 0 24 24" fill="currentColor"><path d="M11.53 2C6.63 2 2.57 5.65 2.08 10.44a.5.5 0 00.5.56h3.04a.5.5 0 00.49-.42A5.5 5.5 0 0111.53 6a5.5 5.5 0 015.47 5.01.5.5 0 00.5.49h3.04a.5.5 0 00.5-.56C20.49 5.65 16.43 2 11.53 2z"/><path d="M11.53 22c4.9 0 8.96-3.65 9.45-8.44a.5.5 0 00-.5-.56h-3.04a.5.5 0 00-.49.42A5.5 5.5 0 0111.53 18a5.5 5.5 0 01-5.47-5.01.5.5 0 00-.5-.49H2.52a.5.5 0 00-.5.56C2.57 18.35 6.63 22 11.53 22z"/></svg>';
        cls = 'dl-jira';
      } else if (href.startsWith('/f/')) {
        icon = '<span class="text-xs opacity-60 mr-0.5">&#9654;</span>';
        cls = 'dl-fireflies';
      } else if (href.startsWith('/a/')) {
        icon = '<span class="text-xs opacity-60 mr-0.5">&#8594;</span>';
        cls = 'dl-artifact';
      } else if (href.startsWith('/p/')) {
        icon = '<span class="text-xs opacity-60 mr-0.5">@</span>';
        cls = 'dl-person';
      } else {
        continue;
      }
      a.classList.add('dl-link', cls);
      a.innerHTML = icon + a.innerHTML;
      a.title = href;
    }
  }

  // Add deep link styles
  const dlStyle = document.createElement('style');
  dlStyle.textContent = `
    .dl-link { text-decoration: none; border-bottom: 1px dashed currentColor; transition: opacity .15s; }
    .dl-link:hover { opacity: .7; }
    .dl-jira { color: var(--c-link, #2563eb); }
    .dl-fireflies { color: var(--c-teal, #0d9488); }
    .dl-artifact { color: var(--c-ink, #191918); }
    .dl-person { color: var(--c-muted, #5E5E66); }
  `;
  document.head.appendChild(dlStyle);

  // Init after DOM ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initDeepLinks);
  } else {
    initDeepLinks();
  }

})();
