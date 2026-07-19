---
layout: home
title: Honest readiness for your agent stack
description: Minerva operates the shared agent library, verifies stack readiness, packages portable bundles, and turns evidence into exact next actions.
pageClass: minerva-home
hero:
  name: Minerva · Agent library operator
  text: Know when your agent stack is actually ready.
  tagline: One control plane for skills, profiles, retrieval health, portable libraries, and evidence-backed next actions. Minerva reads the signals your tools already own—then gives operators an honest answer.
  image:
    src: /hero-system.svg
    alt: Minerva connecting library, stack, signal, MCP, proof, and profile systems
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Explore the architecture
      link: /guide/architecture
---

<div class="signal-strip landing-shell" aria-label="Minerva at a glance">
  <div class="signal-lead">A small operator for a stack that is becoming anything but small.</div>
  <div><span class="signal-number">01</span><span class="signal-label">Shared library</span></div>
  <div><span class="signal-number">02</span><span class="signal-label">Readiness layers</span></div>
  <div><span class="signal-number">00</span><span class="signal-label">False greens</span></div>
</div>

<section class="landing-section landing-shell">
  <div class="thesis-grid">
    <div>
      <div class="section-kicker">The operating problem</div>
      <h2 class="section-heading">Installed is not the same as ready.</h2>
    </div>
    <p class="section-copy">Your binaries can exist while indexes are stale, profiles drift, retrieval is hollow, and failures leave no trail. Minerva separates presence from readiness—then points to the tool that actually owns the fix.</p>
  </div>
</section>

<section class="landing-section workflow-section">
  <div class="landing-shell">
    <div class="workflow-topline">
      <div>
        <div class="section-kicker">One honest loop</div>
        <h2 class="section-heading">From disk state to a defensible next action.</h2>
      </div>
      <p class="section-copy">Minerva does not replace your runtime, gateway, graph, or evidence vault. It composes their public contracts into one operator view.</p>
    </div>
    <div class="workflow-grid">
      <div class="workflow-step">
        <span class="step-number">01 / DISCOVER</span>
        <h3>Read the shared library</h3>
        <p>Skills and agent profiles stay on the same <code>~/.agents</code> tree your harness already consumes.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">02 / PROBE</span>
        <h3>Interrogate the stack</h3>
        <p>Check real binary names, core tiers, deep health, index freshness, and gateway drift.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">03 / PROVE</span>
        <h3>Keep outcome evidence</h3>
        <p>Save eval and readiness artifacts to fcheap with consistent, searchable attribution.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">04 / ACT</span>
        <h3>Rank the next move</h3>
        <p>Turn owned signals into exact commands—without silently mutating another system.</p>
      </div>
    </div>
  </div>
</section>

<section class="landing-section landing-shell">
  <div class="section-kicker">Operator surface</div>
  <h2 class="section-heading">A control plane that knows its place.</h2>
  <div class="capability-grid">
    <div class="capability-panel terminal-panel">
      <div class="terminal-toolbar"><i></i><i></i><i></i><span>minerva / stack deep</span></div>
      <div class="terminal-code" aria-label="Example Minerva stack report">
        <div><span class="prompt">$</span> minerva stack deep</div>
        <div class="muted">probing core operators...</div>
        <div><span class="good">●</span> bob <span class="muted">contract healthy</span></div>
        <div><span class="good">●</span> cortex <span class="muted">12 verified / 1 stale</span></div>
        <div><span class="good">●</span> mcphub <span class="muted">routes synchronized</span></div>
        <div><span class="prompt">●</span> codemap <span class="muted">index stale</span></div>
        <div><span class="good">●</span> vecgrep <span class="muted">profile current</span></div>
        <div class="muted">────────────────────────────</div>
        <div>retrieval_ready: <span class="prompt">false</span></div>
      </div>
      <div class="terminal-verdict">
        <span class="verdict-dot"></span>
        <div><strong>Truth before confidence</strong><small>Codemap owns the next action. Minerva names it.</small></div>
      </div>
    </div>
    <div class="capability-panel mini-panel">
      <span class="panel-index">LIBRARY / SSOT</span>
      <h3>Skills and profiles, without a shadow state.</h3>
      <p>Create, compare, and organize the files your harness already loads. Activation semantics stay explicit.</p>
    </div>
    <div class="capability-panel mini-panel mcp-panel">
      <span class="panel-index">MCP / STDIO</span>
      <h3>Operator intelligence for any harness.</h3>
      <p>Expose read-only diagnostics and approval-gated mutations through exact MCP routes.</p>
    </div>
  </div>
</section>

<section class="landing-section authority-section">
  <div class="landing-shell authority-layout">
    <div>
      <div class="section-kicker">Clear authority</div>
      <h2 class="section-heading">Orchestrate. Never impersonate.</h2>
    </div>
    <div class="authority-list">
      <div class="authority-row"><strong>local-agent</strong><span>Prompt assembly, session skills, MCP trust</span></div>
      <div class="authority-row"><strong>Cortex</strong><span>Task evidence lifecycle and verification</span></div>
      <div class="authority-row"><strong>MCPHub</strong><span>Gateway, sync, routes, and call intelligence</span></div>
      <div class="authority-row"><strong>Codemap + Vecgrep</strong><span>Graph and semantic retrieval readiness</span></div>
      <div class="authority-row"><strong>fcheap</strong><span>Durable artifacts and outcome evidence</span></div>
      <div class="authority-row"><strong>Minerva</strong><span>Library state, readiness composition, ranked suggestions</span></div>
    </div>
  </div>
</section>

<section class="install-section">
  <div class="landing-shell install-grid">
    <div>
      <div class="section-kicker">Open source · Go CLI + MCP</div>
      <h2 class="section-heading">Give your stack a source of truth.</h2>
      <p class="section-copy">Install Minerva, initialize the shared library, and get your first honest readiness report in four commands.</p>
    </div>
    <div>
      <div class="install-command">
        <pre><span class="prompt">$</span> go install github.com/abdul-hamid-achik/minerva/cmd/minerva@latest
<span class="prompt">$</span> minerva init
<span class="prompt">$</span> minerva stack deep
<span class="prompt">$</span> minerva suggest</pre>
      </div>
      <div class="install-links">
        <a class="install-link" href="/guide/getting-started">Read the getting started guide</a>
        <a class="install-link" href="https://github.com/abdul-hamid-achik/minerva">View source on GitHub</a>
      </div>
    </div>
  </div>
</section>
