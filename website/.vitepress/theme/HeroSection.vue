<script setup>
import { onMounted, ref } from 'vue'

const visible = ref(false)
const terminalVisible = ref(false)
const activeTab = ref('npm')

const installCommands = {
  npm: { cmd: 'npm install -g', arg: 'jira-jr' },
  brew: { cmd: 'brew install', arg: 'sofq/tap/jr' },
  pip: { cmd: 'pip install', arg: 'jira-jr' },
  go: { cmd: 'go install', arg: 'github.com/sofq/jira-cli@latest' },
}

onMounted(() => {
  requestAnimationFrame(() => {
    visible.value = true
    setTimeout(() => {
      terminalVisible.value = true
    }, 600)
  })
})
</script>

<template>
  <section class="hero-section">
    <div class="hero-bg">
      <div class="hero-grid"></div>
      <div class="hero-glow"></div>
      <div class="hero-glow-2"></div>
    </div>

    <div class="hero-container" :class="{ visible }">
      <div class="hero-badge">
        <span class="badge-dot"></span>
        Open Source CLI Tool
      </div>

      <h1 class="hero-title">
        <span class="hero-brand">jr</span>
        <span class="hero-divider"></span>
        <span class="hero-headline">Jira, but for<br><span class="hero-accent">AI agents</span></span>
      </h1>

      <p class="hero-description">
        The CLI that gives AI agents full control over Jira.
        <span class="hero-dim">600+ commands auto-generated from OpenAPI. JSON in, JSON out. Drop-in skill for Claude Code, Cursor, Codex, and more.</span>
      </p>

      <div class="hero-stats">
        <div class="stat">
          <span class="stat-value">600+</span>
          <span class="stat-label">Commands</span>
        </div>
        <div class="stat-divider"></div>
        <div class="stat">
          <span class="stat-value">0</span>
          <span class="stat-label">Prompt eng. needed</span>
        </div>
        <div class="stat-divider"></div>
        <div class="stat">
          <span class="stat-value">100%</span>
          <span class="stat-label">JSON output</span>
        </div>
      </div>

      <div class="hero-actions">
        <a href="/jira-cli/guide/getting-started" class="btn-primary">
          Get Started
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
        </a>
        <a href="https://github.com/sofq/jira-cli" class="btn-secondary" target="_blank">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
          View on GitHub
        </a>
      </div>
    </div>

    <div class="hero-terminal" :class="{ visible: terminalVisible }">
      <div class="term-chrome">
        <div class="term-dots">
          <span></span><span></span><span></span>
        </div>
        <div class="term-tabs">
          <button
            v-for="(val, key) in installCommands"
            :key="key"
            class="term-tab-btn"
            :class="{ active: activeTab === key }"
            @click="activeTab = key"
          >{{ key }}</button>
        </div>
      </div>
      <div class="term-body">
        <div class="term-line">
          <span class="term-prompt">$</span>
          <span class="term-cmd">{{ installCommands[activeTab].cmd }}</span>
          <span class="term-arg">{{ installCommands[activeTab].arg }}</span>
        </div>
        <div class="term-line term-blank"></div>
        <div class="term-line">
          <span class="term-prompt">$</span>
          <span class="term-cmd">jr workflow move</span>
          <span class="term-flag">--issue</span>
          <span class="term-val">PROJ-123</span>
          <span class="term-flag">--to</span>
          <span class="term-str">"In Progress"</span>
          <span class="term-flag">--assign</span>
          <span class="term-val">me</span>
        </div>
        <div class="term-line term-blank"></div>
        <div class="term-line term-output">
          <span class="term-json">{</span>
          <span class="term-key">"key"</span><span class="term-json">:</span>
          <span class="term-str">"PROJ-123"</span><span class="term-json">,</span>
          <span class="term-key">"status"</span><span class="term-json">:</span>
          <span class="term-str">"In Progress"</span><span class="term-json">,</span>
          <span class="term-key">"assignee"</span><span class="term-json">:</span>
          <span class="term-str">"me@company.com"</span>
          <span class="term-json">}</span>
        </div>
        <div class="term-line term-blank"></div>
        <div class="term-line">
          <span class="term-prompt">$</span>
          <span class="term-cmd">jr diff</span>
          <span class="term-flag">--issue</span>
          <span class="term-val">PROJ-123</span>
          <span class="term-flag">--since</span>
          <span class="term-val">2h</span>
        </div>
        <div class="term-line term-blank"></div>
        <div class="term-line term-output">
          <span class="term-json">[{</span>
          <span class="term-key">"field"</span><span class="term-json">:</span>
          <span class="term-str">"status"</span><span class="term-json">,</span>
          <span class="term-key">"from"</span><span class="term-json">:</span>
          <span class="term-str">"Open"</span><span class="term-json">,</span>
          <span class="term-key">"to"</span><span class="term-json">:</span>
          <span class="term-str">"In Progress"</span>
          <span class="term-json">}]</span>
        </div>
        <div class="term-line">
          <span class="term-cursor"></span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hero-section {
  position: relative;
  overflow: hidden;
  padding: 80px 24px 60px;
  min-height: 90vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

/* ---- Background effects ---- */

.hero-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 0;
}

.hero-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 136, 0, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 136, 0, 0.03) 1px, transparent 1px);
  background-size: 60px 60px;
  mask-image: radial-gradient(ellipse 80% 60% at 50% 40%, black 20%, transparent 70%);
  -webkit-mask-image: radial-gradient(ellipse 80% 60% at 50% 40%, black 20%, transparent 70%);
}

.hero-glow {
  position: absolute;
  top: -20%;
  left: 50%;
  transform: translateX(-50%);
  width: 800px;
  height: 600px;
  background: radial-gradient(ellipse, rgba(255, 136, 0, 0.08) 0%, transparent 70%);
  filter: blur(40px);
}

.hero-glow-2 {
  position: absolute;
  bottom: -10%;
  left: 50%;
  transform: translateX(-50%);
  width: 600px;
  height: 400px;
  background: radial-gradient(ellipse, rgba(255, 169, 64, 0.04) 0%, transparent 70%);
  filter: blur(60px);
}

/* ---- Content ---- */

.hero-container {
  position: relative;
  z-index: 1;
  max-width: 720px;
  text-align: center;
  opacity: 0;
  transform: translateY(20px);
  transition: opacity 0.8s cubic-bezier(0.16, 1, 0.3, 1), transform 0.8s cubic-bezier(0.16, 1, 0.3, 1);
}

.hero-container.visible {
  opacity: 1;
  transform: translateY(0);
}

/* Badge */

.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.75rem;
  font-weight: 500;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--vp-c-brand-1);
  border: 1px solid rgba(255, 136, 0, 0.25);
  background: rgba(255, 136, 0, 0.06);
  padding: 6px 16px;
  border-radius: 100px;
  margin-bottom: 32px;
}

.badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--vp-c-brand-1);
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* Title */

.hero-title {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 24px;
  margin-bottom: 24px;
  flex-wrap: wrap;
}

.hero-brand {
  font-family: 'JetBrains Mono', monospace;
  font-size: 5.5rem;
  font-weight: 700;
  line-height: 1;
  background: linear-gradient(135deg, #FF8800 0%, #FFB347 40%, #FF8800 80%);
  background-size: 200% 200%;
  animation: gradient-shift 6s ease infinite;
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  filter: drop-shadow(0 0 40px rgba(255, 136, 0, 0.2));
}

@keyframes gradient-shift {
  0%, 100% { background-position: 0% 50%; }
  50% { background-position: 100% 50%; }
}

.hero-divider {
  width: 1px;
  height: 72px;
  background: linear-gradient(to bottom, transparent, var(--vp-c-border), transparent);
}

.hero-headline {
  font-family: 'Inter', -apple-system, sans-serif;
  font-size: 2.4rem;
  font-weight: 700;
  line-height: 1.15;
  letter-spacing: -0.03em;
  color: var(--vp-c-text-1);
  text-align: left;
}

.hero-accent {
  background: linear-gradient(135deg, #FF8800, #FFA940);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

/* Description */

.hero-description {
  font-size: 1.1rem;
  line-height: 1.7;
  color: var(--vp-c-text-1);
  max-width: 540px;
  margin: 0 auto 32px;
}

.hero-dim {
  color: var(--vp-c-text-3);
}

/* Stats */

.hero-stats {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 24px;
  margin-bottom: 36px;
}

.stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.stat-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--vp-c-text-1);
}

.stat-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--vp-c-text-3);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.stat-divider {
  width: 1px;
  height: 32px;
  background: var(--vp-c-border);
}

/* Actions */

.hero-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  flex-wrap: wrap;
}

.btn-primary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 28px;
  font-size: 0.95rem;
  font-weight: 600;
  color: #0a0a0a;
  background: linear-gradient(135deg, #FF8800, #FFA940);
  border-radius: 10px;
  text-decoration: none;
  transition: all 0.25s ease;
  box-shadow: 0 2px 12px rgba(255, 136, 0, 0.25);
}

.btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 20px rgba(255, 136, 0, 0.35);
}

.btn-secondary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 28px;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--vp-c-text-1);
  background: transparent;
  border: 1px solid var(--vp-c-border);
  border-radius: 10px;
  text-decoration: none;
  transition: all 0.25s ease;
}

.btn-secondary:hover {
  border-color: var(--vp-c-text-3);
  background: rgba(255, 255, 255, 0.04);
}

/* ---- Terminal ---- */

.hero-terminal {
  position: relative;
  z-index: 1;
  max-width: 660px;
  width: 100%;
  margin-top: 48px;
  border-radius: 12px;
  border: 1px solid var(--vp-c-border);
  background: #0d0d0d;
  overflow: hidden;
  opacity: 0;
  transform: translateY(24px);
  transition: opacity 0.8s cubic-bezier(0.16, 1, 0.3, 1), transform 0.8s cubic-bezier(0.16, 1, 0.3, 1);
  box-shadow:
    0 4px 24px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 136, 0, 0.06);
}

.hero-terminal.visible {
  opacity: 1;
  transform: translateY(0);
}

.term-chrome {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: #161616;
  border-bottom: 1px solid #222;
}

.term-dots {
  display: flex;
  gap: 6px;
}

.term-dots span {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}

.term-dots span:nth-child(1) { background: #ff5f57; }
.term-dots span:nth-child(2) { background: #febc2e; }
.term-dots span:nth-child(3) { background: #28c840; }

.term-tabs {
  display: flex;
  gap: 2px;
  margin-left: auto;
  margin-right: auto;
}

.term-tab-btn {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.72rem;
  color: #555;
  background: transparent;
  border: none;
  padding: 4px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.term-tab-btn:hover {
  color: #999;
  background: rgba(255, 255, 255, 0.04);
}

.term-tab-btn.active {
  color: var(--vp-c-brand-1);
  background: rgba(255, 136, 0, 0.1);
}

.term-body {
  padding: 20px 20px 24px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.82rem;
  line-height: 1.8;
}

.term-line {
  white-space: nowrap;
  overflow: hidden;
}

.term-blank {
  height: 8px;
}

.term-prompt {
  color: #28c840;
  margin-right: 8px;
}

.term-cmd {
  color: #e5e5e5;
  font-weight: 500;
  margin-right: 6px;
}

.term-arg {
  color: #FFA940;
}

.term-flag {
  color: #888;
  margin-left: 4px;
  margin-right: 4px;
}

.term-val {
  color: #60a5fa;
}

.term-str {
  color: #4ade80;
}

.term-output {
  padding-left: 16px;
}

.term-json {
  color: #888;
}

.term-key {
  color: #c084fc;
}

.term-cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: var(--vp-c-brand-1);
  opacity: 0.7;
  animation: cursor-blink 1.2s step-end infinite;
  vertical-align: middle;
  margin-left: 2px;
}

@keyframes cursor-blink {
  0%, 100% { opacity: 0.7; }
  50% { opacity: 0; }
}

/* ---- Responsive ---- */

@media (max-width: 768px) {
  .hero-section {
    padding: 60px 20px 40px;
    min-height: auto;
  }

  .hero-title {
    flex-direction: column;
    gap: 12px;
  }

  .hero-brand {
    font-size: 4rem;
  }

  .hero-divider {
    width: 48px;
    height: 1px;
    background: linear-gradient(to right, transparent, var(--vp-c-border), transparent);
  }

  .hero-headline {
    font-size: 1.75rem;
    text-align: center;
  }

  .hero-stats {
    gap: 16px;
  }

  .stat-value {
    font-size: 1.25rem;
  }

  .term-body {
    font-size: 0.72rem;
    padding: 16px;
    overflow-x: auto;
  }
}

@media (max-width: 480px) {
  .hero-brand {
    font-size: 3rem;
  }

  .hero-headline {
    font-size: 1.4rem;
  }

  .hero-description {
    font-size: 0.95rem;
  }

  .hero-actions {
    flex-direction: column;
  }

  .btn-primary, .btn-secondary {
    width: 100%;
    justify-content: center;
  }
}
</style>
