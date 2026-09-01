---
layout: home
title: Skill intelligence for agent harnesses
description: Minerva reads agent conversations and tool calls, proposes SKILL.md files, and keeps skills in sync across Claude Code, Codex, Cursor, and other harnesses.
pageClass: minerva-home
hero:
  name: Minerva · Skill intelligence
  text: Turn agent sessions into skills.
  tagline: Read the conversation. See the tool calls. Propose the skill the harness should have loaded — then keep that library in sync.
  image:
    src: /hero-system.svg
    alt: Minerva connecting sessions, signals, skills, and harnesses
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Explore the architecture
      link: /guide/architecture
---

<div class="signal-strip landing-shell" aria-label="Minerva at a glance">
  <div class="signal-lead">Named for wisdom — Minerva watches how your agents work, then writes it down as skills.</div>
  <div><span class="signal-number">01</span><span class="signal-label">Sessions</span></div>
  <div><span class="signal-number">02</span><span class="signal-label">Signals</span></div>
  <div><span class="signal-number">03</span><span class="signal-label">Skills</span></div>
</div>

<section class="landing-section landing-shell">
  <div class="thesis-grid">
    <div>
      <div class="section-kicker">The operating problem</div>
      <h2 class="section-heading">Agents repeat themselves. Skills should capture that.</h2>
    </div>
    <p class="section-copy">A harness session already contains the workflow: the prompt, the tool calls, the retries, the corrections. Minerva reads those traces and proposes <code>SKILL.md</code> files so the next session does not rediscover the same path.</p>
  </div>
</section>

<section class="landing-section workflow-section">
  <div class="landing-shell">
    <div class="workflow-topline">
      <div>
        <div class="section-kicker">One honest loop</div>
        <h2 class="section-heading">From a transcript to a skill you can load.</h2>
      </div>
      <p class="section-copy">Minerva does not run your agent. It reads what the harness already wrote to disk, extracts deterministic signals, and drafts skills with evidence.</p>
    </div>
    <div class="workflow-grid">
      <div class="workflow-step">
        <span class="step-number">01 / READ</span>
        <h3>Parse harness sessions</h3>
        <p>Claude Code, Codex, Cursor, and others — normalized into one Trace model.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">02 / SIGNAL</span>
        <h3>Find the pattern</h3>
        <p>Retries, corrections, load-gaps, repeated tool sequences, shell families.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">03 / PROPOSE</span>
        <h3>Draft a skill</h3>
        <p>New, update, or “load this existing skill” — each with session evidence.</p>
      </div>
      <div class="workflow-step">
        <span class="step-number">04 / SYNC</span>
        <h3>Keep harnesses aligned</h3>
        <p>Canonical <code>~/.agents/skills</code>, linked or copied into writable harness dirs.</p>
      </div>
    </div>
  </div>
</section>

<section class="install-section">
  <div class="landing-shell install-grid">
    <div>
      <div class="section-kicker">Open source · Go CLI + MCP</div>
      <h2 class="section-heading">Install and read your first sessions.</h2>
      <p class="section-copy">Four commands get you from a binary to ranked skill proposals.</p>
    </div>
    <div>
      <div class="install-command">
        <pre><span class="prompt">$</span> go install github.com/abdul-hamid-achik/minerva/cmd/minerva@latest
<span class="prompt">$</span> minerva init
<span class="prompt">$</span> minerva sessions --since 7d
<span class="prompt">$</span> minerva propose --since 30d</pre>
      </div>
      <div class="install-links">
        <a class="install-link" href="/guide/getting-started">Read the getting started guide</a>
        <a class="install-link" href="https://github.com/abdul-hamid-achik/minerva">View source on GitHub</a>
      </div>
    </div>
  </div>
</section>
