package api

const webTemplateHTML = `
{{define "header"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} · MnemosyneOS</title>
  <style>
    html {
      min-height: 100%;
    }
    :root {
      --bg: #f3efe6;
      --panel: #fffaf0;
      --ink: #182028;
      --muted: #5e6a73;
      --line: #d9cfbf;
      --accent: #0f766e;
      --accent-2: #d97706;
      --danger: #b91c1c;
      --shadow: 0 10px 30px rgba(24,32,40,0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "IBM Plex Sans", "Helvetica Neue", sans-serif;
      background:
        radial-gradient(circle at top left, rgba(217,119,6,0.12), transparent 28%),
        radial-gradient(circle at top right, rgba(15,118,110,0.14), transparent 35%),
        var(--bg);
      color: var(--ink);
    }
    body.chat-body {
      height: 100%;
      overflow: hidden;
    }
    body.chat-body .shell {
      height: 100vh;
      min-height: 100vh;
      overflow: hidden;
    }
    a { color: inherit; text-decoration: none; }
    .shell {
      display: grid;
      grid-template-columns: 216px 1fr;
      min-height: 100vh;
      transition: grid-template-columns 180ms ease;
    }
    .nav {
      padding: 32px 20px;
      border-right: 1px solid var(--line);
      background: rgba(255,250,240,0.78);
      backdrop-filter: blur(8px);
      overflow: auto;
      transition: width 180ms ease, padding 180ms ease, border-color 180ms ease, opacity 180ms ease;
    }
    .nav-top {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      margin-bottom: 20px;
    }
    .brand {
      font-family: "IBM Plex Mono", monospace;
      font-size: 14px;
      letter-spacing: 0.14em;
      text-transform: uppercase;
      color: var(--muted);
      margin: 0;
      flex: 1 1 auto;
    }
    .nav-toggle {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      border: 1px solid rgba(217,207,191,0.95);
      border-radius: 12px;
      background: rgba(255,255,255,0.82);
      color: var(--ink);
      font-size: 14px;
      font-weight: 700;
      box-shadow: 0 8px 22px rgba(24,32,40,0.08);
    }
    .nav-toggle:hover {
      transform: translateY(-1px);
      box-shadow: 0 10px 26px rgba(24,32,40,0.12);
    }
    .nav a {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 12px 14px;
      border-radius: 12px;
      margin-bottom: 8px;
      color: var(--muted);
    }
    .nav-glyph {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 32px;
      height: 32px;
      border-radius: 10px;
      background: rgba(24,32,40,0.06);
      color: var(--muted);
      font-family: "IBM Plex Mono", monospace;
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .nav-label {
      white-space: nowrap;
    }
    .nav a.active {
      background: var(--ink);
      color: #fff;
    }
    .nav a.active .nav-glyph {
      background: rgba(255,255,255,0.14);
      color: #fff;
    }
    main {
      padding: 18px 20px;
    }
    main.chat-main-shell {
      height: 100vh;
      overflow: hidden;
      min-height: 0;
    }
    main.chat-main-shell {
      padding: 0;
      position: relative;
    }
    h1 {
      margin: 0 0 18px;
      font-size: 32px;
      font-family: "IBM Plex Serif", Georgia, serif;
    }
    .sub {
      color: var(--muted);
      margin-bottom: 24px;
    }
    .grid {
      display: grid;
      gap: 18px;
    }
    .grid.two { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .grid.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .grid.four { grid-template-columns: repeat(4, minmax(0, 1fr)); }
    .card {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 18px;
      box-shadow: var(--shadow);
    }
    .card h2, .card h3 {
      margin: 0 0 12px;
      font-size: 18px;
    }
    .metric {
      font-size: 28px;
      font-weight: 700;
    }
    .muted { color: var(--muted); }
    .pill {
      display: inline-block;
      border-radius: 999px;
      padding: 4px 10px;
      font-size: 12px;
      font-weight: 700;
      background: rgba(15,118,110,0.12);
      color: var(--accent);
      margin-right: 6px;
    }
    .pill.warn { background: rgba(217,119,6,0.14); color: var(--accent-2); }
    .pill.danger { background: rgba(185,28,28,0.12); color: var(--danger); }
    table {
      width: 100%;
      border-collapse: collapse;
    }
    th, td {
      text-align: left;
      padding: 10px 8px;
      border-bottom: 1px solid var(--line);
      vertical-align: top;
      font-size: 14px;
    }
    th { color: var(--muted); font-weight: 700; }
    form.inline { display: inline; }
    input[type="text"], textarea, select {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 10px 12px;
      background: #fff;
      font: inherit;
    }
    textarea { min-height: 88px; resize: vertical; }
    button {
      border: 0;
      border-radius: 12px;
      padding: 10px 14px;
      background: var(--ink);
      color: #fff;
      font: inherit;
      cursor: pointer;
    }
    button.secondary { background: var(--accent); }
    button.warn { background: var(--accent-2); }
    button.danger { background: var(--danger); }
    .stack > * + * { margin-top: 12px; }
    .split {
      display: grid;
      gap: 18px;
      grid-template-columns: 1.1fr 0.9fr;
    }
    .chat-shell {
      display: grid;
      gap: 18px;
      grid-template-columns: 240px minmax(0, 1fr) 320px;
      align-items: start;
    }
    .chat-app {
      display: grid;
      grid-template-columns: 236px minmax(0, 1fr);
      gap: 16px;
      height: 100%;
      min-height: 0;
      overflow: hidden;
    }
    .chat-app {
      position: fixed;
      top: 18px;
      right: 20px;
      bottom: 18px;
      left: 236px;
      height: auto;
      align-self: auto;
    }
    #app-shell.nav-collapsed .chat-app {
      left: 20px;
    }
    .chat-sidebar {
      display: grid;
      gap: 16px;
      align-content: start;
      min-height: 0;
      overflow: auto;
      transition: opacity 180ms ease, transform 180ms ease;
    }
    .chat-sidebar,
    .chat-panel.chat-main {
      height: 100%;
    }
    .chat-backdrop {
      position: fixed;
      inset: 0;
      background: rgba(24,32,40,0.28);
      opacity: 0;
      pointer-events: none;
      transition: opacity 180ms ease;
      z-index: 18;
    }
    .chat-panel {
      background: linear-gradient(180deg, rgba(255,250,240,0.95), rgba(248,242,231,0.92));
      border: 1px solid rgba(217,207,191,0.85);
      border-radius: 24px;
      box-shadow: var(--shadow);
      min-height: 0;
    }
    .chat-main {
      display: flex;
      flex-direction: column;
      height: 100%;
      min-height: 0;
      overflow: hidden;
    }
    .chat-topbar {
      padding: 12px 16px 10px;
      border-bottom: 1px solid rgba(217,207,191,0.9);
      background:
        radial-gradient(circle at top right, rgba(15,118,110,0.16), transparent 28%),
        linear-gradient(180deg, rgba(255,250,240,0.96), rgba(250,246,237,0.9));
    }
    .chat-kicker {
      font-family: "IBM Plex Mono", monospace;
      font-size: 12px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 10px;
    }
    .chat-topbar h1 {
      margin: 0;
      font-size: 24px;
    }
    .chat-topbar p {
      margin: 6px 0 0;
      color: var(--muted);
      max-width: 72ch;
      font-size: 12px;
      line-height: 1.45;
    }
    .session-state-card {
      margin-top: 6px;
      border: 1px solid rgba(217,207,191,0.82);
      border-radius: 999px;
      background: rgba(255,250,240,0.76);
      padding: 5px 9px;
    }
    .session-state-inline {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
      align-items: center;
    }
    .session-state-summary {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-top: 6px;
    }
    .session-state-summary .session-chip {
      color: var(--ink);
      background: rgba(255,255,255,0.86);
    }
    .session-state-label {
      font-size: 10px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      margin-right: 2px;
    }
    .session-state-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px 14px;
      margin-top: 10px;
      font-size: 13px;
      color: var(--muted);
    }
    .session-state-grid strong {
      display: block;
      color: var(--ink);
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      margin-bottom: 4px;
    }
    .session-state-list {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-top: 6px;
    }
    .session-chip {
      display: inline-flex;
      align-items: center;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 2px 8px;
      font-size: 10px;
      color: var(--muted);
      background: rgba(255,255,255,0.72);
    }
    .session-chip.active {
      color: var(--ink);
      background: rgba(255,255,255,0.92);
    }
    .topbar-row {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
    }
    .topbar-actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      justify-content: flex-end;
      flex-shrink: 0;
    }
    .toggle-button {
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 7px 11px;
      background: rgba(255,250,240,0.9);
      color: var(--ink);
      font-size: 12px;
      font-weight: 700;
    }
    .shell.nav-collapsed {
      grid-template-columns: 84px 1fr;
    }
    .shell.nav-collapsed .nav {
      padding-left: 12px;
      padding-right: 12px;
      overflow: hidden;
    }
    .shell.nav-collapsed .nav-top {
      justify-content: center;
    }
    .shell.nav-collapsed .brand {
      display: none;
    }
    .shell.nav-collapsed .nav a {
      justify-content: center;
      padding-left: 10px;
      padding-right: 10px;
    }
    .shell.nav-collapsed .nav-label {
      display: none;
    }
    .shell.nav-collapsed .nav-glyph {
      min-width: 36px;
      height: 36px;
    }
    .chat-app.sidebar-collapsed {
      grid-template-columns: 1fr;
    }
    .chat-app.sidebar-collapsed .chat-sidebar {
      opacity: 0;
      transform: translateX(-12px);
      pointer-events: none;
      width: 0;
    }
    .chat-stream {
      flex: 1 1 auto;
      overflow: auto;
      min-height: 260px;
      padding: 22px 22px 92px;
      scroll-behavior: smooth;
      background:
        linear-gradient(180deg, rgba(255,251,243,0.9), rgba(247,241,229,0.72)),
        repeating-linear-gradient(
          180deg,
          rgba(24,32,40,0.02) 0,
          rgba(24,32,40,0.02) 1px,
          transparent 1px,
          transparent 36px
        );
    }
    .composer-shell {
      flex: 0 0 auto;
      border-top: 1px solid rgba(217,207,191,0.9);
      padding: 10px 14px 12px;
      background: rgba(255,250,240,0.94);
      backdrop-filter: blur(8px);
    }
    .composer-form {
      border: 1px solid var(--line);
      border-radius: 22px;
      background: #fffdf8;
      padding: 12px 14px 12px;
      box-shadow: 0 8px 24px rgba(24,32,40,0.06);
    }
    .composer-input {
      width: 100%;
      border: 0;
      background: transparent;
      resize: none;
      min-height: 92px;
      max-height: 180px;
      padding: 0;
      font: inherit;
      color: var(--ink);
      line-height: 1.55;
    }
    .composer-input:focus {
      outline: none;
    }
    .composer-toolbar {
      display: flex;
      gap: 10px;
      justify-content: space-between;
      align-items: center;
      margin-top: 8px;
    }
    .composer-toolbar-right {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .composer-meta {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      color: var(--muted);
      font-size: 13px;
    }
    .composer-meta span {
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 6px 10px;
      background: rgba(255,250,240,0.78);
    }
    .composer-status {
      min-height: 18px;
      color: var(--muted);
      font-size: 12px;
      text-align: right;
    }
    .composer-status.error {
      color: var(--danger);
    }
    .setup-banner {
      margin: 16px 24px 0;
      padding: 16px 18px;
      border: 1px solid rgba(180,160,80,0.35);
      border-radius: 14px;
      background: rgba(180,160,80,0.08);
      font-size: 14px;
      line-height: 1.6;
    }
    .setup-banner strong { font-weight: 700; }
    .setup-banner a {
      color: var(--accent);
      text-decoration: underline;
      font-weight: 600;
    }
    .chat-error {
      margin: 16px 24px 0;
      padding: 12px 14px;
      border: 1px solid rgba(185,28,28,0.22);
      border-radius: 14px;
      background: rgba(185,28,28,0.08);
      color: var(--danger);
      font-size: 14px;
      font-weight: 700;
    }
    .sidebar-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      flex-wrap: wrap;
      padding: 14px 14px 10px;
      border-bottom: 1px solid rgba(217,207,191,0.9);
    }
    .sidebar-header h2 {
      margin: 0;
      font-size: 16px;
      flex: 1 1 auto;
    }
    .sidebar-section {
      padding: 12px 14px 14px;
    }
    .session-list {
      display: grid;
      gap: 10px;
    }
    .session-item {
      display: block;
      padding: 10px 12px;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: linear-gradient(180deg, rgba(248,243,234,0.96), rgba(241,235,225,0.9));
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.55);
      transition: transform 120ms ease, border-color 120ms ease, box-shadow 120ms ease, background 120ms ease;
    }
    .session-item.active {
      background: linear-gradient(180deg, #182028, #243444);
      color: #fff;
      border-color: #182028;
      box-shadow: 0 14px 26px rgba(24,32,40,0.22);
      transform: translateY(-1px);
    }
    .session-link {
      display: block;
    }
    .session-title {
      font-weight: 700;
      line-height: 1.35;
    }
    .session-sub {
      display: flex;
      justify-content: space-between;
      gap: 8px;
      margin-top: 8px;
      font-size: 12px;
      color: var(--muted);
    }
    .session-item.active .session-sub,
    .session-item.active .muted {
      color: rgba(255,255,255,0.72);
    }
    .session-item form {
      margin-top: 8px;
    }
    .session-item input[type="text"] {
      font-size: 13px;
      padding: 8px 10px;
    }
    .session-item .stack {
      gap: 8px;
    }
    .session-actions {
      display: flex;
      gap: 8px;
      margin-top: 8px;
    }
    .session-actions button {
      padding: 8px 10px;
      font-size: 13px;
    }
    .session-manage {
      margin-top: 8px;
      border-radius: 14px;
      background: rgba(255,255,255,0.08);
      border-color: rgba(255,255,255,0.10);
    }
    .session-manage summary {
      font-size: 13px;
      font-weight: 700;
    }
    .session-item.active .session-manage summary {
      color: rgba(255,255,255,0.88);
    }
    .session-list.archived .session-item {
      background: rgba(245,241,232,0.86);
      border-style: dashed;
    }
    .session-list.archived .session-title {
      color: var(--muted);
    }
    .chat-thread {
      display: grid;
      gap: 12px;
      align-content: start;
      min-height: 100%;
      max-width: 1180px;
      margin: 0 auto;
    }
    .chat-thread.empty {
      align-content: center;
      justify-items: center;
    }
    .chat-layout {
      display: grid;
      grid-template-columns: 292px minmax(0, 1fr);
      gap: 18px;
      align-items: stretch;
      height: calc(100vh - 36px);
      min-height: calc(100vh - 36px);
      max-height: calc(100vh - 36px);
      transition: grid-template-columns 160ms ease;
    }
    .chat-layout.rail-collapsed {
      grid-template-columns: 0 minmax(0, 1fr);
      gap: 0;
    }
    .chat-rail {
      display: grid;
      gap: 16px;
      align-content: start;
      min-height: 0;
      padding-right: 2px;
      overflow: hidden;
      transition: opacity 160ms ease, transform 160ms ease;
    }
    .chat-layout.rail-collapsed .chat-rail {
      padding-right: 0;
      opacity: 0;
      pointer-events: none;
      transform: translateX(-18px);
    }
    .chat-stage-pane {
      display: grid;
      grid-template-rows: auto minmax(0, 1fr) auto;
      height: 100%;
      min-height: calc(100vh - 36px);
      max-height: calc(100vh - 36px);
      overflow: hidden;
      background: linear-gradient(180deg, rgba(255,251,244,0.98), rgba(250,244,235,0.94));
      box-shadow: 0 16px 40px rgba(24,32,40,0.1);
    }
    .chat-stage-head {
      padding: 16px 18px 14px;
      border-bottom: 1px solid rgba(217,207,191,0.9);
      background:
        radial-gradient(circle at top right, rgba(15,118,110,0.16), transparent 28%),
        linear-gradient(180deg, rgba(255,250,240,0.96), rgba(250,246,237,0.9));
    }
    .chat-stage-head-top {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 4px;
    }
    .chat-stage-head h1 {
      margin: 0;
      font-size: 24px;
    }
    .chat-stage-head p {
      margin: 6px 0 0;
      color: var(--muted);
      max-width: 72ch;
      font-size: 13px;
      line-height: 1.5;
    }
    .chat-message-viewport {
      min-height: 0;
      overflow: auto;
      padding: 18px 18px 72px;
      background:
        linear-gradient(180deg, rgba(255,251,243,0.9), rgba(247,241,229,0.72)),
        repeating-linear-gradient(
          180deg,
          rgba(24,32,40,0.02) 0,
          rgba(24,32,40,0.02) 1px,
          transparent 1px,
          transparent 36px
        );
      scrollbar-width: thin;
      scrollbar-color: rgba(24,32,40,0.22) rgba(255,255,255,0.18);
    }
    .chat-message-viewport::-webkit-scrollbar {
      width: 12px;
    }
    .chat-message-viewport::-webkit-scrollbar-thumb {
      background: rgba(24,32,40,0.22);
      border-radius: 999px;
      border: 2px solid transparent;
      background-clip: padding-box;
    }
    .chat-message-viewport::-webkit-scrollbar-track {
      background: rgba(255,255,255,0.18);
      border-radius: 999px;
    }
    .scroll-bottom-button {
      position: absolute;
      right: 24px;
      bottom: 104px;
      z-index: 3;
      border: 1px solid rgba(217,207,191,0.96);
      border-radius: 999px;
      padding: 8px 12px;
      background: rgba(255,250,240,0.96);
      color: var(--ink);
      box-shadow: 0 10px 24px rgba(24,32,40,0.12);
      opacity: 0;
      pointer-events: none;
      transform: translateY(8px);
      transition: opacity 140ms ease, transform 140ms ease;
    }
    .scroll-bottom-button.visible {
      opacity: 1;
      pointer-events: auto;
      transform: translateY(0);
    }
    .chat-compose-wrap {
      border-top: 1px solid rgba(217,207,191,0.9);
      padding: 10px 14px 12px;
      background: rgba(255,250,240,0.94);
      backdrop-filter: blur(8px);
    }
    .chat-session-card {
      display: grid;
      gap: 10px;
      align-content: start;
      min-height: 0;
      background: linear-gradient(180deg, rgba(245,239,229,0.96), rgba(239,232,221,0.94));
      border-color: rgba(205,192,174,0.95);
      box-shadow: 0 12px 30px rgba(24,32,40,0.08);
    }
    .chat-layout.rail-collapsed .chat-rail-toggle .chat-rail-toggle-icon {
      transform: rotate(180deg);
    }
    .chat-rail-toggle {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      border: 1px solid rgba(217,207,191,0.95);
      border-radius: 999px;
      background: rgba(255,255,255,0.82);
      color: var(--ink);
      font-size: 12px;
      font-weight: 700;
      box-shadow: 0 8px 22px rgba(24,32,40,0.08);
      white-space: nowrap;
    }
    .chat-rail-toggle:hover {
      transform: translateY(-1px);
      box-shadow: 0 10px 26px rgba(24,32,40,0.12);
    }
    .chat-rail-toggle-icon {
      font-family: "IBM Plex Mono", monospace;
      font-size: 11px;
      line-height: 1;
      transition: transform 160ms ease;
    }
    .message {
      border: 1px solid rgba(217,207,191,0.88);
      border-radius: 24px;
      padding: 14px 16px;
      background: rgba(255,250,240,0.94);
      box-shadow: 0 14px 28px rgba(24,32,40,0.07);
      max-width: min(980px, 100%);
    }
    .message.user {
      justify-self: end;
      background: linear-gradient(135deg, rgba(15,118,110,0.16), rgba(250,255,253,0.96));
      border-bottom-right-radius: 8px;
    }
    .message.assistant {
      justify-self: start;
      background: linear-gradient(135deg, rgba(217,119,6,0.10), rgba(255,250,240,0.98));
      border-bottom-left-radius: 8px;
    }
    .message.pending {
      opacity: 0.82;
    }
    .message.assistant.pending {
      position: relative;
      border-color: rgba(15,118,110,0.34);
      box-shadow: 0 16px 32px rgba(15,118,110,0.10);
    }
    .message.assistant.pending::after {
      content: "";
      position: absolute;
      inset: 0;
      border-radius: inherit;
      background: linear-gradient(90deg, transparent, rgba(15,118,110,0.08), transparent);
      transform: translateX(-100%);
      animation: pending-sweep 1.8s linear infinite;
      pointer-events: none;
    }
    @keyframes pending-sweep {
      100% { transform: translateX(100%); }
    }
    @keyframes msgFadeIn {
      from { opacity: 0; transform: translateY(12px); }
      to   { opacity: 1; transform: translateY(0); }
    }
    .message { animation: msgFadeIn 0.32s ease both; }
    .message:nth-last-child(1) { animation-delay: 0.04s; }
    .message:nth-last-child(2) { animation-delay: 0s; }
    .typing-cursor::after {
      content: "▍";
      display: inline;
      animation: cursorBlink 0.6s step-end infinite;
      color: var(--accent);
      font-weight: 300;
    }
    @keyframes cursorBlink {
      50% { opacity: 0; }
    }
    .message-body pre {
      background: #1e1e2e;
      color: #cdd6f4;
      border-radius: 12px;
      padding: 14px 16px;
      overflow-x: auto;
      font-family: "IBM Plex Mono", "Fira Code", monospace;
      font-size: 13px;
      line-height: 1.55;
      margin: 10px 0;
      position: relative;
    }
    .message-body pre code {
      background: none;
      padding: 0;
      border-radius: 0;
      color: inherit;
      font-size: inherit;
    }
    .message-body code {
      background: rgba(24,32,40,0.07);
      padding: 2px 6px;
      border-radius: 6px;
      font-family: "IBM Plex Mono", monospace;
      font-size: 0.9em;
    }
    .message-body h1, .message-body h2, .message-body h3 {
      margin: 16px 0 8px;
      font-family: "IBM Plex Serif", Georgia, serif;
      line-height: 1.3;
    }
    .message-body h1 { font-size: 1.3em; }
    .message-body h2 { font-size: 1.15em; }
    .message-body h3 { font-size: 1.05em; }
    .message-body blockquote {
      border-left: 3px solid var(--accent);
      margin: 10px 0;
      padding: 4px 14px;
      color: var(--muted);
    }
    .message-body hr {
      border: none;
      border-top: 1px solid var(--line);
      margin: 14px 0;
    }
    .message-body strong { font-weight: 700; }
    .message-body em { font-style: italic; }
    .code-copy-btn {
      position: absolute;
      top: 8px;
      right: 8px;
      background: rgba(255,255,255,0.12);
      border: 1px solid rgba(255,255,255,0.18);
      border-radius: 8px;
      color: rgba(255,255,255,0.7);
      font-size: 11px;
      padding: 4px 10px;
      cursor: pointer;
      transition: background 150ms, color 150ms;
    }
    .code-copy-btn:hover { background: rgba(255,255,255,0.22); color: #fff; }
    .code-copy-btn.copied { background: rgba(22,163,74,0.3); color: #4ade80; }
    @keyframes spinPulse {
      0%, 80%, 100% { transform: scale(0); opacity: 0.5; }
      40% { transform: scale(1); opacity: 1; }
    }
    .sending-dots {
      display: inline-flex;
      gap: 4px;
      vertical-align: middle;
      margin-left: 6px;
    }
    .sending-dots span {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--accent);
      animation: spinPulse 1.2s ease-in-out infinite;
    }
    .sending-dots span:nth-child(2) { animation-delay: 0.15s; }
    .sending-dots span:nth-child(3) { animation-delay: 0.3s; }
    .connection-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #4ade80;
      margin-right: 5px;
      transition: background 300ms;
    }
    .connection-dot.disconnected { background: var(--danger); }
    .connection-dot.reconnecting { background: var(--accent-2); animation: cursorBlink 1s step-end infinite; }
    .message-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
      align-items: center;
      margin-bottom: 8px;
    }
    .message .pill {
      padding: 3px 8px;
      font-size: 11px;
    }
    .pill.intent {
      background: rgba(24,32,40,0.08);
      color: var(--ink);
    }
    .pill.stage-warn {
      background: rgba(217,119,6,0.14);
      color: var(--accent-2);
    }
    .pill.stage-live {
      background: rgba(15,118,110,0.14);
      color: var(--accent);
    }
    .pill.stage-alert {
      background: rgba(180,83,9,0.14);
      color: #9a3412;
    }
    .pill.stage-danger {
      background: rgba(185,28,28,0.12);
      color: var(--danger);
    }
    .pill.stage-ok {
      background: rgba(22,163,74,0.12);
      color: #166534;
    }
    .message-body {
      font-size: 14px;
      line-height: 1.65;
      overflow-wrap: anywhere;
    }
    .message-body p {
      margin: 0 0 12px;
    }
    .message-body p:last-child {
      margin-bottom: 0;
    }
    .message-body ul,
    .message-body ol {
      margin: 8px 0 12px 20px;
      padding: 0;
    }
    .message-body li + li {
      margin-top: 6px;
    }
    .message-body a {
      color: var(--accent);
      text-decoration: underline;
      text-underline-offset: 2px;
    }
    .message-content {
      display: grid;
      gap: 0;
    }
    .chat-stream::-webkit-scrollbar,
    .chat-sidebar::-webkit-scrollbar {
      width: 10px;
    }
    .chat-stream::-webkit-scrollbar-thumb,
    .chat-sidebar::-webkit-scrollbar-thumb {
      background: rgba(24,32,40,0.18);
      border-radius: 999px;
      border: 2px solid transparent;
      background-clip: padding-box;
    }
    .chat-stream::-webkit-scrollbar-track,
    .chat-sidebar::-webkit-scrollbar-track {
      background: transparent;
    }
    .message-footer {
      margin-top: 12px;
    }
    .message-section {
      margin-top: 14px;
      padding-top: 10px;
      padding-left: 14px;
      border-top: 1px dashed rgba(24,32,40,0.09);
    }
    .message-section-label {
      font-size: 10px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 6px;
      font-family: "IBM Plex Mono", monospace;
    }
    .message-task-card {
      border-top-color: rgba(15,118,110,0.18);
    }
    .task-card-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
      margin-bottom: 4px;
    }
    .task-card-row:last-child {
      margin-bottom: 0;
    }
    .task-card-label {
      font-size: 10px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      font-family: "IBM Plex Mono", monospace;
      min-width: 40px;
    }
    .task-card-id {
      font-family: "IBM Plex Mono", monospace;
      font-size: 12px;
      color: var(--accent);
      font-weight: 700;
      text-decoration: none;
      word-break: break-all;
    }
    .task-card-id:hover {
      text-decoration: underline;
    }
    .task-card-chip {
      display: inline-flex;
      align-items: center;
      padding: 3px 10px;
      border-radius: 999px;
      font-size: 11px;
      font-weight: 600;
      background: rgba(24,32,40,0.06);
      color: var(--ink);
      border: 1px solid rgba(24,32,40,0.08);
    }
    .task-card-chip.state-done {
      background: rgba(15,118,110,0.12);
      border-color: rgba(15,118,110,0.25);
      color: rgb(10,85,80);
    }
    .task-card-chip.state-failed,
    .task-card-chip.state-blocked {
      background: rgba(185,28,28,0.10);
      border-color: rgba(185,28,28,0.25);
      color: rgb(145,24,24);
    }
    .task-card-chip.state-awaiting_approval,
    .task-card-chip.state-awaiting_confirmation {
      background: rgba(217,119,6,0.12);
      border-color: rgba(217,119,6,0.28);
      color: rgb(140,70,10);
    }
    .message-tool-trace {
      border-top-color: rgba(15,118,110,0.14);
      background: rgba(15,118,110,0.04);
      border-radius: 6px;
      padding: 10px 12px;
    }
    .tool-trace-row {
      font-family: "IBM Plex Mono", monospace;
      font-size: 12px;
      color: var(--ink);
      margin-top: 6px;
      padding-bottom: 6px;
      border-bottom: 1px dotted rgba(15,118,110,0.10);
    }
    .tool-trace-row:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
    .tool-trace-name {
      color: rgb(10,85,80);
      font-weight: 700;
    }
    .tool-trace-args {
      color: var(--muted);
      margin-left: 6px;
    }
    .tool-trace-result {
      color: rgba(24,32,40,0.78);
      margin-top: 3px;
      padding-left: 14px;
      word-break: break-word;
    }
    .tool-trace-error {
      color: rgb(145,24,24);
      margin-top: 3px;
      padding-left: 14px;
    }
    .message-diagnostics {
      margin-top: 12px;
      padding-top: 8px;
      border-top: 1px dashed rgba(24,32,40,0.09);
    }
    .message-diagnostics > summary {
      cursor: pointer;
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      font-family: "IBM Plex Mono", monospace;
      list-style: none;
      padding: 4px 0;
    }
    .message-diagnostics > summary::-webkit-details-marker {
      display: none;
    }
    .message-diagnostics > summary::before {
      content: "▸ ";
      color: var(--muted);
      display: inline-block;
      transition: transform 0.12s ease;
    }
    .message-diagnostics[open] > summary::before {
      content: "▾ ";
    }
    .diagnostics-body {
      padding: 8px 0 2px 14px;
      border-left: 2px solid rgba(24,32,40,0.08);
      margin-top: 6px;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .diagnostics-section {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .diagnostics-label {
      font-size: 10px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      font-family: "IBM Plex Mono", monospace;
    }
    .diagnostics-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
    }
    .diagnostics-muted {
      font-size: 12px;
      color: var(--muted);
      line-height: 1.5;
    }
    .message-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 10px;
    }
    .message-links a {
      font-size: 13px;
      color: var(--accent);
      font-weight: 700;
    }
    .message-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 10px;
    }
    .message-resource-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 10px;
    }
    .message-resource-grid.compact {
      gap: 6px;
    }
    .resource-card {
      display: block;
      border: 1px solid rgba(217,207,191,0.95);
      border-radius: 14px;
      padding: 9px 11px;
      background: rgba(255,255,255,0.86);
      box-shadow: 0 4px 12px rgba(24,32,40,0.05);
      position: relative;
      overflow: hidden;
      min-width: 0;
      max-width: 320px;
      flex: 0 1 240px;
    }
    .message-resource-grid.compact .resource-card {
      padding: 7px 10px;
      border-radius: 999px;
      max-width: 100%;
      flex: 0 1 auto;
      min-height: 0;
    }
    .message-resource-grid.compact .resource-header {
      align-items: center;
      gap: 8px;
    }
    .message-resource-grid.compact .resource-icon {
      min-width: 24px;
      height: 24px;
      border-radius: 999px;
      font-size: 9px;
    }
    .message-resource-grid.compact .resource-kicker {
      display: none;
    }
    .message-resource-grid.compact .resource-title {
      font-size: 12px;
    }
    .message-resource-grid.compact .resource-sub {
      display: none;
    }
    .message-resource-grid.compact .resource-preview {
      margin-top: 6px;
      padding-top: 6px;
    }
    .message-resource-grid.compact .resource-action {
      margin-top: 6px;
      padding: 6px 8px;
      font-size: 11px;
    }
    .resource-card:hover {
      transform: translateY(-1px);
      box-shadow: 0 10px 20px rgba(24,32,40,0.08);
    }
    .resource-card.task {
      border-color: rgba(15,118,110,0.24);
      background: linear-gradient(180deg, rgba(15,118,110,0.08), rgba(255,255,255,0.92));
    }
    .resource-card.artifact {
      border-color: rgba(217,119,6,0.28);
      background: linear-gradient(180deg, rgba(217,119,6,0.08), rgba(255,255,255,0.92));
    }
    .resource-card.approval {
      border-color: rgba(185,28,28,0.22);
      background: linear-gradient(180deg, rgba(185,28,28,0.07), rgba(255,255,255,0.92));
    }
    .resource-card.action {
      border-color: rgba(24,32,40,0.14);
      background: linear-gradient(180deg, rgba(24,32,40,0.05), rgba(255,255,255,0.92));
    }
    .resource-card:focus-visible {
      outline: 2px solid rgba(15,118,110,0.35);
      outline-offset: 2px;
    }
    .resource-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
    }
    .resource-icon {
      min-width: 32px;
      height: 32px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: 10px;
      font-size: 10px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--ink);
      background: rgba(24,32,40,0.08);
      font-family: "IBM Plex Mono", monospace;
    }
    .resource-kicker {
      font-size: 10px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 4px;
      font-family: "IBM Plex Mono", monospace;
    }
    .resource-title {
      font-weight: 700;
      line-height: 1.35;
      color: var(--ink);
      font-size: 13px;
    }
    .resource-sub {
      margin-top: 4px;
      color: var(--muted);
      font-size: 11px;
      line-height: 1.4;
      word-break: break-word;
    }
    .resource-preview {
      margin-top: 10px;
      padding-top: 10px;
      border-top: 1px dashed rgba(217,207,191,0.95);
      color: var(--muted);
      font-size: 12px;
      line-height: 1.5;
      opacity: 0;
      max-height: 0;
      overflow: hidden;
      transform: translateY(4px);
      transition: opacity 140ms ease, transform 140ms ease, max-height 140ms ease;
    }
    .resource-card:hover .resource-preview,
    .resource-card:focus-within .resource-preview {
      opacity: 1;
      max-height: 120px;
      transform: translateY(0);
    }
    .resource-action {
      width: 100%;
      margin-top: 8px;
      padding: 8px 10px;
      font-size: 12px;
    }
    .toolbar-form {
      display: grid;
      gap: 12px;
      grid-template-columns: 1.2fr repeat(3, minmax(0, 180px)) auto;
      align-items: end;
    }
    .toolbar-actions {
      display: flex;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
    }
    .dense-list {
      display: grid;
      gap: 10px;
    }
    .dense-item {
      position: relative;
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 12px;
      padding: 12px 14px;
      border: 1px solid rgba(217,207,191,0.95);
      border-radius: 16px;
      background: rgba(255,255,255,0.76);
    }
    .dense-item.active {
      border-color: rgba(15,118,110,0.35);
      box-shadow: 0 10px 20px rgba(15,118,110,0.08);
      background: linear-gradient(180deg, rgba(15,118,110,0.06), rgba(255,255,255,0.9));
    }
    .dense-item:hover,
    .dense-item:focus-visible {
      border-color: rgba(24,32,40,0.16);
      box-shadow: 0 12px 28px rgba(24,32,40,0.1);
    }
    .dense-title {
      font-weight: 700;
      line-height: 1.35;
    }
    .dense-sub {
      margin-top: 6px;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.5;
    }
    .dense-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      align-items: flex-start;
      justify-content: flex-end;
    }
    .dense-preview {
      position: absolute;
      top: calc(100% - 6px);
      left: 14px;
      right: 14px;
      padding: 10px 12px;
      border-radius: 14px;
      border: 1px solid rgba(217,207,191,0.95);
      background: rgba(255,250,240,0.98);
      color: var(--muted);
      font-size: 12px;
      line-height: 1.5;
      box-shadow: var(--shadow);
      opacity: 0;
      transform: translateY(-4px);
      pointer-events: none;
      transition: opacity 140ms ease, transform 140ms ease;
      z-index: 3;
    }
    .dense-item:hover .dense-preview,
    .dense-item:focus-within .dense-preview {
      opacity: 1;
      transform: translateY(0);
    }
    .detail-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px 14px;
    }
    .detail-grid strong {
      display: block;
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: var(--muted);
      margin-bottom: 4px;
    }
    .decision-card {
      border: 1px solid rgba(217,207,191,0.95);
      border-radius: 18px;
      padding: 14px;
      background: rgba(255,255,255,0.82);
    }
    .decision-card.danger {
      border-color: rgba(185,28,28,0.22);
      background: linear-gradient(180deg, rgba(185,28,28,0.05), rgba(255,255,255,0.9));
    }
    .decision-actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      margin-top: 12px;
    }
    .inline-input {
      width: 100%;
      margin-top: 10px;
    }
    details {
      margin-top: 10px;
      border: 1px solid var(--line);
      border-radius: 12px;
      background: rgba(255,255,255,0.78);
      padding: 10px 12px;
    }
    details summary {
      cursor: pointer;
      font-weight: 700;
    }
    .empty-thread {
      width: min(760px, 100%);
      max-width: 760px;
      margin: 0 auto;
      text-align: center;
      padding: 24px 24px;
      border: 1px dashed rgba(217,207,191,0.9);
      border-radius: 24px;
      background: rgba(255,250,240,0.72);
    }
    .empty-thread h3 {
      margin: 0 0 6px;
      font-family: "IBM Plex Serif", Georgia, serif;
      font-size: 24px;
    }
    .empty-thread-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      justify-content: center;
      margin-top: 14px;
    }
    .empty-thread-pill {
      display: inline-flex;
      align-items: center;
      border: 1px solid rgba(217,207,191,0.96);
      border-radius: 999px;
      padding: 6px 10px;
      font-size: 12px;
      font-weight: 700;
      background: rgba(255,255,255,0.82);
      color: var(--ink);
      cursor: pointer;
    }
    .composer {
      position: sticky;
      top: 28px;
    }
    pre {
      white-space: pre-wrap;
      word-break: break-word;
      background: #fff;
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 12px;
      margin: 0;
      font: 13px/1.5 "IBM Plex Mono", monospace;
    }
    @media (max-width: 960px) {
      .shell { grid-template-columns: 1fr; }
      .nav { border-right: 0; border-bottom: 1px solid var(--line); }
      .shell.nav-collapsed { grid-template-columns: 1fr; }
      .shell.nav-collapsed .brand,
      .shell.nav-collapsed .nav-label {
        display: initial;
      }
      .shell.nav-collapsed .nav a {
        justify-content: flex-start;
        padding-left: 14px;
        padding-right: 14px;
      }
      .grid.two, .grid.three, .split, .chat-shell, .chat-app { grid-template-columns: 1fr; }
      .toolbar-form { grid-template-columns: 1fr; }
      .detail-grid { grid-template-columns: 1fr; }
      .chat-layout {
        grid-template-columns: 1fr;
        height: auto;
        min-height: auto;
        max-height: none;
      }
      .chat-layout.rail-collapsed {
        grid-template-columns: 1fr;
        gap: 18px;
      }
      .chat-rail,
      .chat-layout.rail-collapsed .chat-rail {
        opacity: 1;
        pointer-events: auto;
        transform: none;
      }
      .chat-stage-pane {
        height: auto;
        min-height: auto;
        max-height: none;
      }
      .chat-stage-head-top {
        flex-wrap: wrap;
      }
      .chat-message-viewport {
        min-height: 260px;
        max-height: none;
        padding: 14px 12px 18px;
      }
      .chat-compose-wrap {
        padding: 10px 10px 12px;
      }
      .scroll-bottom-button {
        right: 16px;
        bottom: 96px;
      }
      .topbar-row {
        flex-direction: column;
      }
      .topbar-actions {
        width: 100%;
        justify-content: flex-start;
      }
      .chat-stream {
        padding: 16px 12px 18px;
        min-height: 220px;
        max-height: 52vh;
      }
      .empty-thread {
        padding: 24px 18px;
      }
      .empty-thread-actions {
        gap: 8px;
      }
    }
  </style>
</head>
<body{{if .BodyClass}} class="{{.BodyClass}}"{{end}}{{if .BodyStyle}} style="{{.BodyStyle}}"{{end}}>
  <div class="shell" id="app-shell">
    <nav class="nav" id="app-nav">
      <div class="nav-top">
        <div class="brand">MnemosyneOS</div>
        <button type="button" class="nav-toggle" id="nav-toggle" aria-expanded="true" title="Collapse navigation">«</button>
      </div>
      {{range .Nav}}
        <a href="{{.Href}}" class="{{if .Active}}active{{end}}" title="{{.Name}}">
          <span class="nav-glyph">{{.Short}}</span>
          <span class="nav-label">{{.Name}}</span>
        </a>
      {{end}}
    </nav>
    <main{{if .MainClass}} class="{{.MainClass}}"{{end}}{{if .MainStyle}} style="{{.MainStyle}}"{{end}}>
{{end}}

{{define "footer"}}
    </main>
  </div>
  <script>
    (function () {
      const cache = new Map();
      const shell = document.getElementById("app-shell");
      const navToggle = document.getElementById("nav-toggle");
      const navStorageKey = "mnemosyne.nav.collapsed";
      const applyNavCollapsed = function (collapsed) {
        if (!shell || !navToggle) return;
        shell.classList.toggle("nav-collapsed", collapsed);
        navToggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
        navToggle.setAttribute("title", collapsed ? "Expand navigation" : "Collapse navigation");
        navToggle.textContent = collapsed ? "»" : "«";
      };
      if (navToggle && shell) {
        const stored = window.localStorage ? window.localStorage.getItem(navStorageKey) : "";
        applyNavCollapsed(stored === "1");
        navToggle.addEventListener("click", function () {
          const next = !shell.classList.contains("nav-collapsed");
          applyNavCollapsed(next);
          if (window.localStorage) {
            window.localStorage.setItem(navStorageKey, next ? "1" : "0");
          }
        });
      }
      const selectors = "[data-preview-url]";
      const loadPreview = async function (node) {
        if (!node) return;
        const url = node.getAttribute("data-preview-url");
        if (!url) return;
        const target = node.querySelector(".resource-preview, .dense-preview");
        if (!target) return;
        if (cache.has(url)) {
          target.innerHTML = cache.get(url);
          return;
        }
        try {
          const response = await fetch(url, { headers: { "Accept": "text/html" } });
          if (!response.ok) return;
          const html = await response.text();
          cache.set(url, html);
          target.innerHTML = html;
        } catch (_) {}
      };
      const bindPreview = function (node) {
        if (!node || node.dataset.previewBound === "1") return;
        node.dataset.previewBound = "1";
        node.addEventListener("mouseenter", function () { loadPreview(node); });
        node.addEventListener("focusin", function () { loadPreview(node); });
      };
      document.querySelectorAll(selectors).forEach(bindPreview);
      const observer = new MutationObserver(function (mutations) {
        mutations.forEach(function (mutation) {
          mutation.addedNodes.forEach(function (added) {
            if (!(added instanceof HTMLElement)) return;
            if (added.matches && added.matches(selectors)) {
              bindPreview(added);
            }
            added.querySelectorAll && added.querySelectorAll(selectors).forEach(bindPreview);
          });
        });
      });
      observer.observe(document.body, { childList: true, subtree: true });
    }());
  </script>
</body>
</html>
{{end}}

{{define "dashboard"}}
  {{template "header" .}}
    <h1>AgentOS Dashboard</h1>
    <div class="sub">A single operator view for runtime health, current focus, gated work, and the most recent execution signals.</div>
    {{if .ModelUnconfigured}}
    <div class="setup-banner" style="margin-bottom:18px;">
      <strong>Model not configured.</strong>
      LLM-powered features (chat replies, task routing, skill execution) are not available yet.
      <a href="/ui/models">Configure a model provider</a> to get started, or run <code>mnemosynectl init --provider &lt;provider&gt; --api-key &lt;key&gt;</code> from the terminal.
    </div>
    {{end}}
    <div class="grid four" style="margin-bottom:18px;">
      <div class="card"><h3>Runtime</h3><div class="metric">{{.Runtime.Status}}</div><div class="muted">profile {{.Runtime.ExecutionProfile}}</div></div>
      <div class="card"><h3>Open Tasks</h3><div class="metric">{{.Summary.OpenTasks}}</div><div class="muted">inbox, planned, active, blocked, or awaiting approval</div></div>
      <div class="card"><h3>Pending Approvals</h3><div class="metric">{{.Summary.PendingApprovals}}</div><div class="muted">gated actions waiting for an operator</div></div>
      <div class="card"><h3>Failed Actions</h3><div class="metric">{{.Summary.FailedActions}}</div><div class="muted">recent execution records with failure status</div></div>
    </div>
    <div class="card" style="margin-bottom:18px;">
      <h2>System Metrics <span class="muted" style="font-size:0.85rem;font-weight:normal;">/metrics API</span></h2>
      <div class="detail-grid" style="grid-template-columns: repeat(4, 1fr);">
        <div><strong>Total Tasks</strong><div class="metric">{{.Metrics.TotalTasks}}</div><div class="muted">{{range $k, $v := .Metrics.TasksByState}}{{$k}}: {{$v}} {{end}}</div></div>
        <div><strong>Total Actions</strong><div class="metric">{{.Metrics.TotalActions}}</div><div class="muted">{{range $k, $v := .Metrics.ActionsByStatus}}{{$k}}: {{$v}} {{end}}</div></div>
        <div><strong>Total Memory Cards</strong><div class="metric">{{.Metrics.TotalMemoryCards}}</div><div class="muted">{{range $k, $v := .Metrics.MemoryByStatus}}{{$k}}: {{$v}} {{end}}</div></div>
        <div><strong>Active Skills</strong><div class="metric">{{.Metrics.ActiveSkills}}</div><div class="muted">enabled and loaded modules</div></div>
      </div>
    </div>
    <div class="split">
      <div class="stack">
        <div class="card">
          <h2>Runtime Focus</h2>
          <div class="stack">
            <div><span class="pill">{{.Runtime.Status}}</span><span class="pill warn">{{.Runtime.ExecutionProfile}}</span></div>
            <div class="detail-grid">
              <div><strong>Runtime ID</strong><div>{{.Runtime.RuntimeID}}</div></div>
              <div><strong>Updated</strong><div>{{.Runtime.UpdatedAt.Format "Jan 2 15:04:05"}}</div></div>
              <div><strong>Active User</strong><div>{{if .Runtime.ActiveUserID}}{{.Runtime.ActiveUserID}}{{else}}unknown{{end}}</div></div>
              <div><strong>Session</strong><div>{{if .Runtime.SessionID}}{{derefString .Runtime.SessionID}}{{else}}none{{end}}</div></div>
            </div>
            {{with .ActiveTask}}
              <div class="decision-card">
                <div class="dense-title">Focus Task</div>
                <div class="dense-sub">{{if .Title}}{{.Title}}{{else}}{{.TaskID}}{{end}}</div>
                <div class="dense-sub">{{preview .Goal 180}}</div>
                <div class="decision-actions">
                  <a class="resource-card task" href="/ui/tasks?task_id={{queryEscape .TaskID}}" data-preview-url="{{previewURL (printf "/ui/tasks?task_id=%s" (queryEscape .TaskID)) ""}}">
                    <div class="resource-kicker">Task</div>
                    <strong>{{.State}}</strong>
                    <span>{{.TaskID}}</span>
                    <div class="resource-preview">Open the current focus task and inspect its runtime detail, next action, and artifacts.</div>
                  </a>
                </div>
              </div>
            {{else}}
              <div class="muted">No active focus task right now.</div>
            {{end}}
          </div>
        </div>
        <div class="card">
          <h2>Recent Execution</h2>
          <div class="dense-list">
            {{range .Actions}}
              <div class="dense-item" data-preview-url="{{if .TaskID}}{{previewURL (printf "/ui/tasks?task_id=%s" (queryEscape .TaskID)) ""}}{{end}}">
                <div>
                  <div class="dense-title">{{.Kind}}</div>
                  <div class="dense-sub">{{if .TaskID}}{{.TaskID}}{{else}}{{.ActionID}}{{end}}</div>
                  <div class="dense-sub">{{if .Command}}{{preview .Command 160}}{{else if .Path}}{{.Path}}{{else}}{{preview .Error 160}}{{end}}</div>
                </div>
                <div class="dense-meta">
                  <span class="pill {{if eq .Status "failed"}}danger{{else if eq .Status "completed"}}warn{{end}}">{{.Status}}</span>
                  <span class="pill">{{.ExecutionProfile}}</span>
                </div>
                <div class="dense-preview">Inspect the latest execution signal. This is where shell, file read, and file write actions surface first.</div>
              </div>
            {{else}}
              <div class="muted">No actions recorded yet.</div>
            {{end}}
          </div>
        </div>
      </div>
      <div class="stack">
        <div class="card">
          <h2>Task Queue</h2>
          <div class="dense-list">
            {{range .Tasks}}
              <a class="dense-item {{if and $.ActiveTask (eq $.ActiveTask.TaskID .TaskID)}}active{{end}}" href="/ui/tasks?task_id={{queryEscape .TaskID}}" data-preview-url="{{previewURL (printf "/ui/tasks?task_id=%s" (queryEscape .TaskID)) ""}}">
                <div>
                  <div class="dense-title">{{if .Title}}{{.Title}}{{else}}{{.TaskID}}{{end}}</div>
                  <div class="dense-sub">{{preview .Goal 160}}</div>
                  <div class="dense-sub">{{.TaskID}} · {{.UpdatedAt.Format "Jan 2 15:04"}}</div>
                </div>
                <div class="dense-meta">
                  <span class="pill">{{.State}}</span>
                  {{if .SelectedSkill}}<span class="pill warn">{{.SelectedSkill}}</span>{{end}}
                </div>
                <div class="dense-preview">Open the runtime task queue and continue from this task's current state, next action, or failure reason.</div>
              </a>
            {{else}}
              <div class="muted">No tasks yet.</div>
            {{end}}
          </div>
        </div>
        <div class="card">
          <h2>Pending Approvals</h2>
          <div class="dense-list">
            {{range .Approvals}}
              <a class="dense-item {{if and $.PendingApproval (eq $.PendingApproval.ApprovalID .ApprovalID)}}active{{end}}" href="/ui/approvals?approval_id={{queryEscape .ApprovalID}}" data-preview-url="{{previewURL (printf "/ui/approvals?approval_id=%s" (queryEscape .ApprovalID)) ""}}">
                <div>
                  <div class="dense-title">{{preview .Summary 110}}</div>
                  <div class="dense-sub">{{.ApprovalID}}</div>
                  <div class="dense-sub">{{if .TaskID}}{{.TaskID}}{{else}}No linked task{{end}}</div>
                </div>
                <div class="dense-meta">
                  <span class="pill danger">{{.Status}}</span>
                  <span class="pill">{{.ExecutionProfile}}</span>
                </div>
                <div class="dense-preview">Jump into the approval decision panel with full task context and one-click approve or deny actions.</div>
              </a>
            {{else}}
              <div class="muted">No pending approvals.</div>
            {{end}}
          </div>
        </div>
      </div>
    </div>
  {{template "footer" .}}
{{end}}

{{define "chat"}}
  {{template "header" .}}
    <div class="chat-layout" id="chat-layout">
      <aside class="chat-rail">
        <section class="chat-panel chat-session-card">
            <div class="sidebar-header">
              <div class="chat-kicker">Workspace</div>
              <h2>Conversations</h2>
            </div>
          <div class="sidebar-section">
            <form method="post" action="/ui/chat/sessions" class="inline" style="margin-bottom:12px;">
              <button type="submit">New Session</button>
            </form>
            <input id="session-search" type="text" placeholder="Search sessions" style="margin-bottom:12px;">
            <div class="session-list">
              {{range .Sessions}}
                <div class="session-item {{if eq $.ActiveSessionID .SessionID}}active{{end}}" data-session-filter="{{lower .Title}} {{.SessionID}}">
                  <a href="/ui/chat?session={{queryEscape .SessionID}}" class="session-link">
                    <div class="session-title">{{.Title}}</div>
                    <div class="session-sub">
                      <span>{{.MessageCount}} messages</span>
                      <span>{{.UpdatedAt.Format "Jan 2"}}</span>
                    </div>
                  </a>
                  {{if eq $.ActiveSessionID .SessionID}}
                    <details class="session-manage">
                      <summary>Session settings</summary>
                      <form method="post" action="/ui/chat/sessions/{{queryEscape .SessionID}}/rename" class="stack">
                      <input type="text" name="title" value="{{$.ActiveTitle}}" placeholder="Session title">
                      <button class="secondary" type="submit">Rename</button>
                      </form>
                      <div class="session-actions">
                        <form method="post" action="/ui/chat/sessions/{{queryEscape .SessionID}}/archive" class="inline">
                          <button class="danger" type="submit">Archive</button>
                        </form>
                        <form method="post" action="/ui/chat/sessions/{{queryEscape .SessionID}}/delete" class="inline" onsubmit="return confirm('Delete this session permanently?');">
                          <button class="danger" type="submit">Delete Permanently</button>
                        </form>
                      </div>
                    </details>
                  {{end}}
                </div>
              {{else}}
                <div class="muted">No sessions yet.</div>
              {{end}}
            </div>
            {{if .Archived}}
              <details class="stack" style="margin-top:14px;">
                <summary>Archived Sessions ({{len .Archived}})</summary>
                <div class="session-list archived">
                  {{range .Archived}}
                    <div class="session-item" data-session-filter="{{lower .Title}} {{.SessionID}}">
                      <div class="session-title">{{.Title}}</div>
                      <div class="session-sub">
                        <span>{{.MessageCount}} messages</span>
                        <span>{{.UpdatedAt.Format "Jan 2"}}</span>
                      </div>
                      <div class="session-actions">
                        <form method="post" action="/ui/chat/sessions/{{queryEscape .SessionID}}/restore" class="inline">
                          <button class="secondary" type="submit">Restore</button>
                        </form>
                        <form method="post" action="/ui/chat/sessions/{{queryEscape .SessionID}}/delete" class="inline" onsubmit="return confirm('Delete this archived session permanently?');">
                          <button class="danger" type="submit">Delete Permanently</button>
                        </form>
                      </div>
                    </div>
                  {{end}}
                </div>
              </details>
            {{end}}
          </div>
        </section>
      </aside>

      <section class="chat-panel chat-stage-pane">
        <div class="chat-stage-head">
          <div class="chat-stage-head-top">
            <div class="chat-kicker">AgentOS Chat</div>
            <button type="button" class="chat-rail-toggle" id="chat-rail-toggle" aria-expanded="true" title="Collapse workspace">
              <span class="chat-rail-toggle-icon">«</span>
              <span class="chat-rail-toggle-label">Hide workspace</span>
            </button>
          </div>
          <h1>{{.ActiveTitle}}</h1>
          {{if .Messages}}
            <p>Ask for work, continue a task, or inspect the current thread state without leaving the conversation.</p>
          {{else}}
            <p>Use the thread like a real operator console: ask for work, watch the runtime respond, approve gated actions, and inspect artifacts without leaving the conversation.</p>
          {{end}}
          <div id="session-state-panel">{{template "chat_session_state" .SessionState}}</div>
        </div>
        {{if .Error}}
          <div class="chat-error">{{.Error}}</div>
        {{end}}
        {{if .ModelUnconfigured}}
          <div class="setup-banner">
            <strong>No model configured.</strong>
            Chat will use fallback replies until you <a href="/ui/models">set up a model provider</a>.
          </div>
        {{end}}
        <div class="chat-message-viewport" id="chat-stream">
          <div id="chat-messages">{{template "chat_messages" .}}</div>
        </div>
        <button type="button" class="scroll-bottom-button" id="scroll-bottom-button">回到底部</button>
        <div class="chat-compose-wrap">
          <form method="post" action="/ui/chat" class="composer-form">
            <input type="hidden" name="session_id" value="{{.Form.SessionID}}">
            <input type="hidden" name="requested_by" value="{{.Form.RequestedBy}}">
            <input type="hidden" name="source" value="{{.Form.Source}}">
            <textarea id="message" class="composer-input" name="message" placeholder="Tell AgentOS what to do next. Search, inspect, plan, edit, or continue a task.">{{.Form.Message}}</textarea>
            <div class="composer-toolbar">
              <div class="composer-meta">
                <span>session: {{.ActiveSessionID}}</span>
                <span>profile:
                  <select id="chat_profile" name="execution_profile" style="width:auto;border:0;background:transparent;padding:0 0 0 6px;">
                    <option value="user" {{if eq .Form.Profile "user"}}selected{{end}}>user</option>
                    <option value="root" {{if eq .Form.Profile "root"}}selected{{end}}>root</option>
                  </select>
                </span>
              </div>
              <div class="composer-toolbar-right">
                <div class="composer-status" id="composer-status"><span class="connection-dot" id="connection-dot"></span>Ready</div>
                <button type="submit">Send Message</button>
              </div>
            </div>
          </form>
        </div>
      </section>
    </div>
    <script>
      (function () {
        const target = document.getElementById("chat-messages");
        const stream = document.getElementById("chat-stream");
        const form = document.querySelector('form[action="/ui/chat"]');
        const message = document.getElementById("message");
        const errorBanner = document.querySelector(".chat-error");
        const composerStatus = document.getElementById("composer-status");
        var BT = String.fromCharCode(96);
        var fenceRe = new RegExp(BT + BT + BT + "(\\w*)\\n([\\s\\S]*?)" + BT + BT + BT, "g");
        var inlineCodeRe = new RegExp(BT + "([^" + BT + "]+)" + BT, "g");
        const renderMd = function (raw) {
          var html = raw;
          var codeBlocks = [];
          html = html.replace(fenceRe, function (_, lang, code) {
            var idx = codeBlocks.length;
            codeBlocks.push('<pre' + (lang ? ' data-lang="' + escapeHTML(lang) + '"' : '') + '><code>' + escapeHTML(code.replace(/\n$/, '')) + '</code></pre>');
            return '\x00CB' + idx + '\x00';
          });
          html = escapeHTML(html);
          html = html.replace(inlineCodeRe, '<code>$1</code>');
          html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
          var lines = html.split('\n');
          var out = [];
          for (var i = 0; i < lines.length; i++) {
            var line = lines[i].trim();
            var cbMatch = line.match(/\x00CB(\d+)\x00/);
            if (cbMatch) { out.push(codeBlocks[parseInt(cbMatch[1])]); continue; }
            if (line === '---' || line === '***') { out.push('<hr>'); continue; }
            if (line.indexOf('### ') === 0) { out.push('<h3>' + line.slice(4) + '</h3>'); continue; }
            if (line.indexOf('## ') === 0) { out.push('<h2>' + line.slice(3) + '</h2>'); continue; }
            if (line.indexOf('# ') === 0) { out.push('<h1>' + line.slice(2) + '</h1>'); continue; }
            if (line.indexOf('&gt; ') === 0) { out.push('<blockquote>' + line.slice(5) + '</blockquote>'); continue; }
            if (line === '') { out.push(''); continue; }
            out.push('<p>' + line + '</p>');
          }
          return out.join('');
        };
        const addCopyButtons = function (root) {
          (root || document).querySelectorAll('pre:not([data-copy-bound])').forEach(function (pre) {
            pre.setAttribute('data-copy-bound', '1');
            const btn = document.createElement('button');
            btn.className = 'code-copy-btn';
            btn.textContent = 'Copy';
            btn.addEventListener('click', function (e) {
              e.preventDefault(); e.stopPropagation();
              const code = pre.querySelector('code');
              const text = code ? code.textContent : pre.textContent;
              navigator.clipboard.writeText(text).then(function () {
                btn.textContent = 'Copied!';
                btn.classList.add('copied');
                setTimeout(function () { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 1800);
              });
            });
            pre.style.position = 'relative';
            pre.appendChild(btn);
          });
        };
        setInterval(function () { addCopyButtons(); }, 800);
        const sessionStatePanel = document.getElementById("session-state-panel");
        const scrollBottomButton = document.getElementById("scroll-bottom-button");
        const sessionSearch = document.getElementById("session-search");
        const chatLayout = document.getElementById("chat-layout");
        const chatRailToggle = document.getElementById("chat-rail-toggle");
        if (!target || !form || !stream) return;
        const railStorageKey = "mnemosyne.chat.railCollapsed";
        const escapeHTML = function (value) {
          return value
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/\"/g, "&quot;")
            .replace(/'/g, "&#39;");
        };
        const stageLabel = function (stage) {
          switch ((stage || "").trim()) {
            case "routing": return "Routing";
            case "queued": return "Queued";
            case "searching": return "Searching";
            case "planning": return "Planning";
            case "reading": return "Reading";
            case "writing": return "Writing";
            case "executing": return "Executing";
            case "triaging_email": return "Triaging Email";
            case "searching_github": return "Searching GitHub";
            case "consolidating": return "Consolidating";
            case "summarizing": return "Summarizing";
            case "persisting": return "Persisting";
            case "writing_memory": return "Writing Memory";
            case "running": return "Running";
            case "tool_call": return "Calling Tool";
            case "tool_result": return "Tool Done";
            case "awaiting_approval": return "Approval Needed";
            case "blocked": return "Blocked";
            case "failed": return "Failed";
            case "done": return "Done";
            case "responded": return "Responded";
            case "working": return "Working";
            default: return stage || "";
          }
        };
        const stageStatusText = function (stage) {
          return stageLabel(stage) || "Ready";
        };
        const stageClass = function (stage) {
          switch ((stage || "").trim()) {
            case "routing":
            case "queued":
              return "stage-warn";
            case "searching":
            case "planning":
            case "reading":
            case "writing":
            case "executing":
            case "triaging_email":
            case "searching_github":
            case "consolidating":
            case "summarizing":
            case "persisting":
            case "writing_memory":
            case "running":
            case "working":
            case "tool_call":
            case "tool_result":
              return "stage-live";
            case "awaiting_approval":
            case "blocked":
              return "stage-alert";
            case "failed":
              return "stage-danger";
            case "done":
            case "responded":
              return "stage-ok";
            default:
              return "";
          }
        };
        const buildAssistantMetaHTML = function (opts) {
          const out = ['<span class="pill warn">assistant</span>'];
          if (opts.intentKind) {
            out.push('<span class="pill intent">' + escapeHTML(opts.intentKind) + '</span>');
          }
          if (opts.stage) {
            const cls = stageClass(opts.stage);
            out.push('<span class="pill' + (cls ? ' ' + cls : '') + '">' + escapeHTML(stageLabel(opts.stage)) + '</span>');
          }
          if (opts.skill) {
            out.push('<span class="pill">' + escapeHTML(opts.skill) + '</span>');
          }
          if (opts.taskState) {
            out.push('<span class="pill danger">' + escapeHTML(opts.taskState) + '</span>');
          }
          return out.join("");
        };
        const setOptimisticAssistant = function (node, opts) {
          if (!node) return;
          const pendingStages = new Set(["routing", "queued", "running", "working", "searching", "planning", "reading", "writing", "executing", "triaging_email", "searching_github", "consolidating", "summarizing", "persisting", "writing_memory", "tool_call", "tool_result"]);
          node.className = pendingStages.has((opts.stage || "").trim()) ? "message assistant pending" : "message assistant";
          const body = escapeHTML(opts.body || "Working on it...").replace(/\n/g, "<br>");
          node.innerHTML =
            '<div class="message-content">' +
              '<div class="message-meta">' + buildAssistantMetaHTML(opts) + '</div>' +
              '<div class="message-body">' + body + '</div>' +
            '</div>';
        };
        let pinnedToBottom = false;
        let hydratedOnce = false;
        const updateScrollBottomButton = function () {
          if (!scrollBottomButton) return;
          const distance = stream.scrollHeight - stream.scrollTop - stream.clientHeight;
          scrollBottomButton.classList.toggle("visible", distance > 160);
        };
        const keepBottom = function (force) {
          if (force || pinnedToBottom) {
            stream.scrollTop = stream.scrollHeight;
          }
          updateScrollBottomButton();
        };
        const connectionDot = document.getElementById("connection-dot");
        const setComposerStatus = function (text, isError) {
          if (!composerStatus) return;
          const dot = connectionDot ? connectionDot.outerHTML : "";
          composerStatus.innerHTML = dot + escapeHTML(text || "");
          composerStatus.classList.toggle("error", !!isError);
        };
        const setConnectionState = function (state) {
          if (!connectionDot) return;
          connectionDot.className = "connection-dot" + (state === "connected" ? "" : state === "reconnecting" ? " reconnecting" : " disconnected");
        };
        const resizeComposer = function () {
          if (!message) return;
          message.style.height = "0px";
          message.style.height = Math.max(96, message.scrollHeight) + "px";
        };
        const ensureThread = function () {
          let thread = target.querySelector(".chat-thread");
          if (!thread) {
            target.innerHTML = '<div class="chat-thread"></div>';
            thread = target.querySelector(".chat-thread");
          }
          return thread;
        };
        const optimisticRender = function (text) {
          const body = escapeHTML(text).replace(/\n/g, "<br>");
          const thread = ensureThread();
          const userNode = document.createElement("div");
          userNode.className = "message user pending";
          userNode.dataset.pendingRole = "user";
          userNode.innerHTML =
            '<div class="message-meta"><span class="pill">user</span></div>' +
            '<div class="message-body">' + body + '</div>';
          const assistantNode = document.createElement("div");
          assistantNode.className = "message assistant pending";
          assistantNode.dataset.pendingRole = "assistant";
          setOptimisticAssistant(assistantNode, {
            stage: "routing",
            body: "Routing the request and preparing the next step..."
          });
          thread.appendChild(userNode);
          thread.appendChild(assistantNode);
          keepBottom(true);
          return { userNode, assistantNode };
        };
        const upsertMessageHTML = function (payload) {
          if (!payload || !payload.message_id || (!payload.html && !payload.inner_html)) return;
          const thread = ensureThread();
          const existing = thread.querySelector('[data-message-id="' + payload.message_id + '"]');
          if (existing) {
            if (payload.class_name) {
              existing.className = payload.class_name;
            }
            if (payload.inner_html) {
              existing.innerHTML = payload.inner_html;
            } else if (payload.html) {
              const swap = document.createElement("div");
              swap.innerHTML = payload.html.trim();
              const nextNode = swap.firstElementChild;
              if (nextNode) {
                existing.innerHTML = nextNode.innerHTML;
                existing.className = nextNode.className;
              }
            }
          } else {
            const roleMatch = payload.class_name && payload.class_name.match(/\b(user|assistant)\b/);
            const pendingRole = roleMatch ? roleMatch[1] : "";
            const pendingNode = pendingRole ? thread.querySelector('.message.pending[data-pending-role="' + pendingRole + '"]') : null;
            if (pendingNode) {
              pendingNode.dataset.messageId = payload.message_id;
              delete pendingNode.dataset.pendingRole;
              if (payload.class_name) {
                pendingNode.className = payload.class_name;
              }
              if (payload.inner_html) {
                pendingNode.innerHTML = payload.inner_html;
              } else if (payload.html) {
                const swap = document.createElement("div");
                swap.innerHTML = payload.html.trim();
                const nextNode = swap.firstElementChild;
                if (nextNode) {
                  pendingNode.innerHTML = nextNode.innerHTML;
                  pendingNode.className = nextNode.className;
                }
              }
            } else {
              if (!payload.html) return;
              const wrapper = document.createElement("div");
              wrapper.innerHTML = payload.html.trim();
              const next = wrapper.firstElementChild;
              if (!next) return;
              thread.appendChild(next);
            }
          }
          if (sessionStatePanel && payload.session_state_html) {
            sessionStatePanel.innerHTML = payload.session_state_html;
          }
          keepBottom(false);
        };
        stream.addEventListener("scroll", function () {
          pinnedToBottom = stream.scrollHeight - stream.scrollTop - stream.clientHeight < 40;
          updateScrollBottomButton();
        });
        if (scrollBottomButton) {
          scrollBottomButton.addEventListener("click", function () {
            stream.scrollTop = stream.scrollHeight;
            pinnedToBottom = true;
            updateScrollBottomButton();
          });
        }
        if (sessionSearch) {
          sessionSearch.addEventListener("input", function () {
            const query = (sessionSearch.value || "").trim().toLowerCase();
            document.querySelectorAll("[data-session-filter]").forEach(function (node) {
              const haystack = (node.getAttribute("data-session-filter") || "").toLowerCase();
              node.style.display = !query || haystack.indexOf(query) >= 0 ? "" : "none";
            });
          });
        }
        const applyRailCollapsed = function (collapsed) {
          if (!chatLayout || !chatRailToggle) return;
          chatLayout.classList.toggle("rail-collapsed", collapsed);
          chatRailToggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
          chatRailToggle.setAttribute("title", collapsed ? "Show workspace" : "Hide workspace");
          const label = chatRailToggle.querySelector(".chat-rail-toggle-label");
          if (label) {
            label.textContent = collapsed ? "Show workspace" : "Hide workspace";
          }
        };
        if (chatRailToggle && chatLayout) {
          const stored = window.localStorage ? window.localStorage.getItem(railStorageKey) : "";
          applyRailCollapsed(stored === "1");
          chatRailToggle.addEventListener("click", function () {
            const next = !chatLayout.classList.contains("rail-collapsed");
            applyRailCollapsed(next);
            if (window.localStorage) {
              window.localStorage.setItem(railStorageKey, next ? "1" : "0");
            }
          });
        }
        document.querySelectorAll("[data-prompt]").forEach(function (node) {
          node.addEventListener("click", function () {
            if (!message) return;
            message.value = node.getAttribute("data-prompt") || "";
            resizeComposer();
            message.focus();
          });
        });
        const refreshMessages = async function () {
          const response = await fetch("/ui/chat/messages?session={{queryEscape .ActiveSessionID}}", {
            headers: { "Accept": "text/html" }
          });
          if (!response.ok) {
            throw new Error("message refresh failed");
          }
          target.innerHTML = await response.text();
          if (!hydratedOnce) {
            stream.scrollTop = 0;
            hydratedOnce = true;
            updateScrollBottomButton();
          } else {
            keepBottom(false);
          }
        };
        let source = null;
        let reconnectTimer = null;
        const connectEvents = function () {
          if (source) {
            source.close();
          }
          source = new EventSource("/ui/chat/events?session={{queryEscape .ActiveSessionID}}");
          source.addEventListener("full", function (event) {
            setConnectionState("connected");
            target.innerHTML = event.data;
            addCopyButtons(target);
            setComposerStatus("Synced");
            if (!hydratedOnce) {
              stream.scrollTop = 0;
              hydratedOnce = true;
              updateScrollBottomButton();
            } else {
              keepBottom(false);
            }
          });
          source.addEventListener("patch", function (event) {
            try {
              const payload = JSON.parse(event.data);
              upsertMessageHTML(payload);
              if (payload.message_id) {
                delete rawContentMap[payload.message_id];
                const el = ensureThread().querySelector('[data-message-id="' + payload.message_id + '"]');
                if (el) {
                  const body = el.querySelector(".message-body");
                  if (body) body.classList.remove("typing-cursor");
                  addCopyButtons(el);
                }
              }
              setComposerStatus(stageStatusText(payload.stage || "responded"));
            } catch (_) {}
          });
          const rawContentMap = {};
          source.addEventListener("delta", function (event) {
            try {
              const payload = JSON.parse(event.data);
              if (!payload || !payload.message_id) return;
              const existing = ensureThread().querySelector('[data-message-id="' + payload.message_id + '"]');
              if (!existing) return;
              if (payload.class_name) {
                existing.className = payload.class_name;
              }
              const body = existing.querySelector(".message-body");
              if (body && payload.delta) {
                const mid = payload.message_id;
                if (!rawContentMap[mid]) rawContentMap[mid] = body.textContent || "";
                rawContentMap[mid] += payload.delta;
                body.innerHTML = renderMd(rawContentMap[mid]);
                body.classList.add("typing-cursor");
                addCopyButtons(body);
              }
              const meta = existing.querySelector(".message-meta");
              if (meta) {
                meta.innerHTML = buildAssistantMetaHTML({
                  intentKind: payload.intent_kind,
                  stage: payload.stage,
                  skill: payload.selected_skill,
                  taskState: payload.task_state
                });
              }
              if (sessionStatePanel && payload.session_state_html) {
                sessionStatePanel.innerHTML = payload.session_state_html;
              }
              keepBottom(false);
              setComposerStatus(stageStatusText(payload.stage || "running"));
            } catch (_) {}
          });
          source.onerror = function () {
            source.close();
            source = null;
            setConnectionState("reconnecting");
            setComposerStatus("Reconnecting...", false);
            if (!reconnectTimer) {
              reconnectTimer = window.setTimeout(async function () {
                reconnectTimer = null;
                try {
                  await refreshMessages();
                } catch (_) {}
                connectEvents();
              }, 1200);
            }
          };
        };
        connectEvents();
        form.addEventListener("submit", async function (event) {
          event.preventDefault();
          const submit = form.querySelector('button[type="submit"]');
          const text = (message && message.value ? message.value : "").trim();
          if (!text) return;
          const payload = new URLSearchParams();
          const formData = new FormData(form);
          formData.forEach(function (value, key) {
            payload.append(key, value);
          });
          let optimisticNodes = optimisticRender(text);
          if (message) message.value = "";
          resizeComposer();
          if (submit) {
            submit.disabled = true;
            submit.dataset.label = submit.textContent;
            submit.innerHTML = 'Sending<span class="sending-dots"><span></span><span></span><span></span></span>';
          }
          setComposerStatus("Routing...");
          try {
            if (errorBanner) {
              errorBanner.remove();
            }
            const response = await fetch(form.action, {
              method: "POST",
              headers: {
                "Accept": "application/json",
                "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8"
              },
              body: payload.toString()
            });
            if (!response.ok) {
              let detail = "chat send failed";
              try {
                const data = await response.json();
                if (data && data.error) {
                  detail = data.error;
                }
              } catch (_) {}
              throw new Error(detail);
            }
            const data = await response.json();
            if (optimisticNodes && data) {
              if (data.user_message && data.user_message.message_id) {
                optimisticNodes.userNode.dataset.messageId = data.user_message.message_id;
                optimisticNodes.userNode.classList.remove("pending");
                delete optimisticNodes.userNode.dataset.pendingRole;
            }
            if (data.assistant_message && data.assistant_message.message_id) {
              optimisticNodes.assistantNode.dataset.messageId = data.assistant_message.message_id;
              delete optimisticNodes.assistantNode.dataset.pendingRole;
                setOptimisticAssistant(optimisticNodes.assistantNode, {
                  intentKind: data.assistant_message.intent_kind,
                  stage: data.assistant_message.stage || "queued",
                  skill: data.assistant_message.selected_skill,
                  taskState: data.assistant_message.task_state,
                  body: data.assistant_message.content || "Queued in the runtime."
                });
              }
            }
            setComposerStatus(stageStatusText((data.assistant_message && data.assistant_message.stage) || "queued"));
            window.setTimeout(function () {
              refreshMessages().catch(function () {});
            }, 300);
            window.setTimeout(function () {
              refreshMessages().catch(function () {});
            }, 1200);
            keepBottom(true);
            if (message) message.focus();
          } catch (error) {
            setComposerStatus(error.message || "Send failed", true);
            window.location.href = "/ui/chat?session={{queryEscape .ActiveSessionID}}&error=" + encodeURIComponent(error.message || "chat send failed");
          } finally {
            if (submit) {
              submit.disabled = false;
              submit.textContent = submit.dataset.label || "Send Message";
            }
          }
        });
        if (message) {
          resizeComposer();
          message.addEventListener("input", function () {
            resizeComposer();
          });
          message.addEventListener("keydown", function (event) {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              form.requestSubmit();
            }
          });
        }
        setComposerStatus("Ready");
        stream.scrollTop = 0;
        hydratedOnce = true;
        pinnedToBottom = stream.scrollHeight - stream.clientHeight < 40;
        updateScrollBottomButton();
      }());
    </script>
  {{template "footer" .}}
{{end}}

{{define "chat_messages"}}
  <div class="chat-thread{{if not .Messages}} empty{{end}}">
    {{range .Messages}}
      {{template "chat_message" (dict "ActiveSessionID" $.ActiveSessionID "Message" .)}}
    {{else}}
      <div class="empty-thread">
        <h3>Start the thread</h3>
        <div class="muted">Ask AgentOS to inspect the repo, search the web, triage email, or continue a blocked task. The runtime details stay attached to the conversation instead of taking over the whole page.</div>
        <div class="empty-thread-actions">
          <button type="button" class="empty-thread-pill" data-prompt="Search the web for the latest project context">Search the web</button>
          <button type="button" class="empty-thread-pill" data-prompt="Inspect the repository and summarize the current state">Inspect the repo</button>
          <button type="button" class="empty-thread-pill" data-prompt="Check email inbox and summarize important threads">Check email</button>
          <button type="button" class="empty-thread-pill" data-prompt="Continue the current task and tell me the next action">Continue a task</button>
        </div>
      </div>
    {{end}}
  </div>
{{end}}

{{define "chat_session_state"}}
  <div class="session-state-card">
    <div class="session-state-inline">
      <span class="session-state-label">Session State</span>
      {{if .Topic}}<span class="session-chip active">Topic {{preview .Topic 28}}</span>{{end}}
      {{if .FocusTaskID}}<span class="session-chip">Focus {{preview .FocusTaskID 14}}</span>{{end}}
      {{if .PendingAction}}<span class="session-chip">{{if eq .PendingAction "confirm_task_intent"}}Awaiting confirmation{{else}}Pending {{preview .PendingAction 16}}{{end}}</span>{{end}}
      {{if .PendingQuestion}}<span class="session-chip">Question {{preview .PendingQuestion 28}}</span>{{end}}
      {{if and (not .Topic) (not .FocusTaskID) (not .PendingAction) (not .PendingQuestion)}}<span class="session-chip">Steady</span>{{end}}
    </div>
  </div>
{{end}}

{{define "chat_message"}}
  <div class="{{if or (eq .Message.Stage "queued") (eq .Message.Stage "running") (eq .Message.Stage "working") (eq .Message.Stage "searching") (eq .Message.Stage "planning") (eq .Message.Stage "reading") (eq .Message.Stage "writing") (eq .Message.Stage "executing") (eq .Message.Stage "triaging_email") (eq .Message.Stage "searching_github") (eq .Message.Stage "consolidating") (eq .Message.Stage "summarizing") (eq .Message.Stage "persisting") (eq .Message.Stage "writing_memory") (eq .Message.Stage "awaiting_confirmation")}}message {{.Message.Role}} pending{{else}}message {{.Message.Role}}{{end}}" data-message-id="{{.Message.MessageID}}">
    {{template "chat_message_inner" .}}
  </div>
{{end}}

{{define "chat_message_inner"}}
  <div class="message-content">
    {{/* Header: only role + stage. Intent / skill / task_state now live in the
         structured task card below so the top row stays uncluttered. */}}
    <div class="message-meta">
      <span class="pill {{if eq .Message.Role "assistant"}}warn{{end}}">{{.Message.Role}}</span>
      {{if .Message.Stage}}<span class="pill {{chatStageClass .Message.Stage}}">{{chatStageLabel .Message.Stage}}</span>{{end}}
    </div>

    <div class="message-body">{{renderChatContentHTML .Message.Role .Message.Content}}</div>

    {{if .Message.ToolTrace}}
      <div class="message-section message-tool-trace">
        <div class="message-section-label">Tools used ({{len .Message.ToolTrace}})</div>
        {{range .Message.ToolTrace}}
          <div class="tool-trace-row">
            <span class="tool-trace-name">{{.ToolName}}</span>
            {{if .Arguments}}<span class="tool-trace-args">{{formatToolArgs .Arguments}}</span>{{end}}
            {{if .Error}}
              <div class="tool-trace-error">✗ {{.Error}}</div>
            {{else if .ResultPreview}}
              <div class="tool-trace-result">→ {{.ResultPreview}}</div>
            {{end}}
          </div>
        {{end}}
      </div>
    {{end}}

    {{if or .Message.TaskID .Message.SelectedSkill .Message.TaskState .Message.ExecutionProfile}}
      <div class="message-section message-task-card">
        <div class="message-section-label">Task</div>
        {{if .Message.TaskID}}
          <div class="task-card-row">
            <span class="task-card-label">ID</span>
            <a class="task-card-id" href="/ui/tasks?task_id={{.Message.TaskID}}">{{.Message.TaskID}}</a>
          </div>
        {{end}}
        <div class="task-card-row">
          {{if .Message.TaskState}}<span class="task-card-chip state-{{.Message.TaskState}}">State: {{.Message.TaskState}}</span>{{end}}
          {{if .Message.SelectedSkill}}<span class="task-card-chip">Skill: {{.Message.SelectedSkill}}</span>{{end}}
          {{if .Message.ExecutionProfile}}<span class="task-card-chip">Profile: {{.Message.ExecutionProfile}}</span>{{end}}
        </div>
      </div>
    {{end}}

    {{if .Message.Links}}
      <div class="message-section">
        <div class="message-section-label">Attachments</div>
        <div class="message-resource-grid compact">
          {{range .Message.Links}}
            {{$kind := resourceKind .Label .Href}}
            <a class="resource-card {{$kind}}" href="{{.Href}}" data-preview-url="{{previewURL .Href ""}}">
              <div class="resource-header">
                <div>
                  <div class="resource-kicker">{{$kind}}</div>
                  <div class="resource-title">{{.Label}}</div>
                </div>
                <div class="resource-icon">{{resourceIcon $kind}}</div>
              </div>
              <div class="resource-sub">{{.Href}}</div>
              <div class="resource-preview">{{resourcePreview .Label .Href}}</div>
            </a>
          {{end}}
        </div>
      </div>
    {{end}}

    {{if .Message.Actions}}
      <div class="message-section">
        <div class="message-section-label">Actions</div>
        <div class="message-resource-grid compact">
          {{range .Message.Actions}}
            <form class="resource-card action" method="post" action="{{.Href}}" data-preview-url="{{previewURL .Href .Method}}">
              <div class="resource-header">
                <div>
                  <div class="resource-kicker">Action</div>
                  <div class="resource-title">{{.Label}}</div>
                </div>
                <div class="resource-icon">{{resourceIcon "action"}}</div>
              </div>
              <div class="resource-sub">{{if .Method}}{{.Method}}{{else}}POST{{end}} {{.Href}}</div>
              <div class="resource-preview">{{actionPreview .Label .Href .Method}}</div>
              <input type="hidden" name="session_id" value="{{$.ActiveSessionID}}">
              <button class="secondary resource-action" type="submit">{{.Label}}</button>
            </form>
          {{end}}
        </div>
      </div>
    {{end}}

    {{/* Collapse every diagnostic panel (intent, memory, recent tasks) into a
         single <details> so the card stays compact. Users who want the raw
         traces can still open it. */}}
    {{$hasIntent := .Message.IntentKind}}
    {{$ctx := .Message.Context}}
    {{$hasRecall := false}}
    {{$hasRecentTasks := false}}
    {{with $ctx}}
      {{if .RecallHits}}{{$hasRecall = true}}{{end}}
      {{if .RecentTasks}}{{$hasRecentTasks = true}}{{end}}
    {{end}}
    {{if or $hasIntent $hasRecall $hasRecentTasks}}
      <details class="message-diagnostics">
        <summary>Diagnostics</summary>
        <div class="diagnostics-body">
          {{if $hasIntent}}
            <div class="diagnostics-section">
              <div class="diagnostics-label">Intent</div>
              <div class="diagnostics-row">
                <span class="task-card-chip">{{.Message.IntentKind}}{{if hasConfidence .Message.IntentConfidence}} ({{printf "%.2f" .Message.IntentConfidence}}){{end}}</span>
                {{if .Message.IntentReason}}<span class="diagnostics-muted">{{.Message.IntentReason}}</span>{{end}}
              </div>
            </div>
          {{end}}
          {{with $ctx}}
            {{if .RecallHits}}
              <div class="diagnostics-section">
                <div class="diagnostics-label">Relevant Memory ({{len .RecallHits}})</div>
                <table>
                  <tr><th>Source</th><th>Card</th><th>Snippet</th></tr>
                  {{range .RecallHits}}
                    <tr>
                      <td>{{.Source}}</td>
                      <td><a href="/ui/memory?card_id={{.CardID}}">{{.CardType}}</a><div class="muted">{{.CardID}}</div></td>
                      <td>{{.Snippet}}</td>
                    </tr>
                  {{end}}
                </table>
              </div>
            {{end}}
            {{if .RecentTasks}}
              <div class="diagnostics-section">
                <div class="diagnostics-label">Recent Tasks ({{len .RecentTasks}})</div>
                <table>
                  <tr><th>Task</th><th>State</th><th>Skill</th></tr>
                  {{range .RecentTasks}}
                    <tr>
                      <td><a href="/ui/tasks?task_id={{.TaskID}}">{{.Title}}</a><div class="muted">{{.TaskID}}</div></td>
                      <td>{{.State}}</td>
                      <td>{{.SelectedSkill}}</td>
                    </tr>
                  {{end}}
                </table>
              </div>
            {{end}}
          {{end}}
        </div>
      </details>
    {{end}}
  </div>
{{end}}

{{define "artifact"}}
  {{template "header" .}}
    <h1>Artifact</h1>
    <div class="sub">Artifact output generated by the AgentOS runtime.</div>
    <div class="card">
      <div class="muted">{{.Path}}</div>
      <div style="margin-top:8px;">
        <a href="/ui/artifacts/view?path={{queryEscape .Path}}&raw=1">Raw</a>
        <a href="/ui/artifacts/view?path={{queryEscape .Path}}&download=1">Download</a>
      </div>
      <pre style="margin-top:12px;">{{.Content}}</pre>
    </div>
  {{template "footer" .}}
{{end}}

{{define "tasks"}}
  {{template "header" .}}
    <h1>Tasks</h1>
    <div class="sub">Filter the runtime queue, inspect the selected unit of work, and act without scanning raw metadata first.</div>
    <div class="grid three" style="margin-bottom:18px;">
      <div class="card"><h3>Total</h3><div class="metric">{{.Summary.Total}}</div><div class="muted">all runtime tasks</div></div>
      <div class="card"><h3>In Flight</h3><div class="metric">{{.Summary.InFlight}}</div><div class="muted">inbox, planned, active</div></div>
      <div class="card"><h3>Needs Review</h3><div class="metric">{{.Summary.AwaitingApproval}}</div><div class="muted">waiting for approval</div></div>
    </div>
    <div class="card" style="margin-bottom:18px;">
      <form method="get" action="/ui/tasks" class="toolbar-form">
        <div>
          <label for="task_query"><strong>Search</strong></label>
          <input id="task_query" type="text" name="query" value="{{.Filter.Query}}" placeholder="task id, title, goal, next action">
        </div>
        <div>
          <label for="task_state"><strong>State</strong></label>
          <select id="task_state" name="state">
            <option value="">all</option>
            <option value="inbox" {{if eq .Filter.State "inbox"}}selected{{end}}>inbox</option>
            <option value="planned" {{if eq .Filter.State "planned"}}selected{{end}}>planned</option>
            <option value="active" {{if eq .Filter.State "active"}}selected{{end}}>active</option>
            <option value="awaiting_approval" {{if eq .Filter.State "awaiting_approval"}}selected{{end}}>awaiting_approval</option>
            <option value="blocked" {{if eq .Filter.State "blocked"}}selected{{end}}>blocked</option>
            <option value="done" {{if eq .Filter.State "done"}}selected{{end}}>done</option>
            <option value="failed" {{if eq .Filter.State "failed"}}selected{{end}}>failed</option>
          </select>
        </div>
        <div>
          <label for="task_skill"><strong>Skill</strong></label>
          <select id="task_skill" name="skill">
            <option value="">all</option>
            {{range .Skills}}<option value="{{.}}" {{if eq $.Filter.Skill .}}selected{{end}}>{{.}}</option>{{end}}
          </select>
        </div>
        <div>
          <label for="task_profile"><strong>Profile</strong></label>
          <select id="task_profile" name="profile">
            <option value="">all</option>
            {{range .Profiles}}<option value="{{.}}" {{if eq $.Filter.Profile .}}selected{{end}}>{{.}}</option>{{end}}
          </select>
        </div>
        <div class="toolbar-actions">
          <button type="submit">Filter</button>
          {{if .HasFilter}}<a href="/ui/tasks" class="toggle-button">Reset</a>{{end}}
        </div>
      </form>
    </div>
    <div class="split">
      <div class="card">
        <h2>Queue</h2>
        <div class="dense-list">
          {{range .Tasks}}
            <a class="dense-item {{if and $.Selected (eq $.Selected.TaskID .TaskID)}}active{{end}}" href="/ui/tasks?task_id={{.TaskID}}{{if $.Filter.State}}&state={{queryEscape $.Filter.State}}{{end}}{{if $.Filter.Skill}}&skill={{queryEscape $.Filter.Skill}}{{end}}{{if $.Filter.Profile}}&profile={{queryEscape $.Filter.Profile}}{{end}}{{if $.Filter.Query}}&query={{queryEscape $.Filter.Query}}{{end}}" data-preview-url="{{previewURL (printf "/ui/tasks?task_id=%s" (queryEscape .TaskID)) ""}}">
              <div>
                <div class="dense-title">{{if .Title}}{{.Title}}{{else}}{{.TaskID}}{{end}}</div>
                <div class="dense-sub">{{preview .Goal 160}}</div>
                <div class="dense-sub">{{.TaskID}} · updated {{.UpdatedAt.Format "Jan 2 15:04"}}</div>
              </div>
              <div class="dense-meta">
                <span class="pill">{{.State}}</span>
                {{if .SelectedSkill}}<span class="pill warn">{{.SelectedSkill}}</span>{{end}}
                <span class="pill">{{.ExecutionProfile}}</span>
                {{if .RequiresApproval}}<span class="pill danger">approval</span>{{end}}
              </div>
            </a>
          {{else}}
            <div class="muted">No tasks available for the current filter.</div>
          {{end}}
        </div>
      </div>
      <div class="card">
        <h2>Task Detail</h2>
        {{with .Selected}}
          <div class="stack">
            <div><span class="pill">{{.State}}</span>{{if .SelectedSkill}}<span class="pill warn">{{.SelectedSkill}}</span>{{end}}<span class="pill">{{.ExecutionProfile}}</span>{{if .RequiresApproval}}<span class="pill danger">requires approval</span>{{end}}</div>
            <div><strong>{{.Title}}</strong></div>
            <div class="muted">{{.Goal}}</div>
            <div class="detail-grid">
              <div><strong>Task ID</strong><div>{{.TaskID}}</div></div>
              <div><strong>Updated</strong><div>{{.UpdatedAt.Format "Jan 2 15:04:05"}}</div></div>
              <div><strong>Requested By</strong><div>{{if .RequestedBy}}{{.RequestedBy}}{{else}}unknown{{end}}</div></div>
              <div><strong>Source</strong><div>{{if .Source}}{{.Source}}{{else}}unknown{{end}}</div></div>
              <div><strong>Next Action</strong><div>{{if .NextAction}}{{.NextAction}}{{else}}None{{end}}</div></div>
              <div><strong>Failure</strong><div>{{if .FailureReason}}{{.FailureReason}}{{else}}None{{end}}</div></div>
            </div>
            <div class="decision-card">
              <div class="dense-title">Operator Action</div>
              <div class="dense-sub">Run or rerun this task after reviewing its current state and next action.</div>
              <div class="decision-actions">
                <form class="inline" method="post" action="/ui/tasks/{{.TaskID}}/run">
                  <button type="submit">Run Task</button>
                </form>
              </div>
            </div>
            <div>
              <strong>Metadata</strong>
              <pre>{{printf "%#v" .Metadata}}</pre>
            </div>
          </div>
        {{else}}
          <div class="muted">Select a task from the list.</div>
        {{end}}
      </div>
    </div>
    <div class="card" style="margin-top:18px;">
      <h2>Create Task</h2>
      <form method="post" action="/ui/tasks" class="stack">
        <div class="grid two">
          <div>
            <label for="title"><strong>Title</strong></label>
            <input id="title" type="text" name="title" value="{{.Form.Title}}" placeholder="Search recent GitHub issues">
          </div>
          <div>
            <label for="execution_profile"><strong>Profile</strong></label>
            <select id="execution_profile" name="execution_profile">
              <option value="user" {{if eq .Form.Profile "user"}}selected{{end}}>user</option>
              <option value="root" {{if eq .Form.Profile "root"}}selected{{end}}>root</option>
            </select>
          </div>
        </div>
        <div>
          <label for="goal"><strong>Goal</strong></label>
          <textarea id="goal" name="goal" placeholder="Search GitHub issues about approval flow and summarize the signal.">{{.Form.Goal}}</textarea>
        </div>
        <div class="grid two">
          <div>
            <label for="requested_by"><strong>Requested By</strong></label>
            <input id="requested_by" type="text" name="requested_by" value="{{.Form.RequestedBy}}">
          </div>
          <div>
            <label for="source"><strong>Source</strong></label>
            <input id="source" type="text" name="source" value="{{.Form.Source}}">
          </div>
        </div>
        <div class="grid three">
          <div>
            <label for="path"><strong>Path</strong></label>
            <input id="path" type="text" name="path" value="{{.Form.Path}}" placeholder="notes/todo.txt">
          </div>
          <div>
            <label for="command"><strong>Command</strong></label>
            <input id="command" type="text" name="command" value="{{.Form.Command}}" placeholder="pwd">
          </div>
          <div>
            <label for="query"><strong>Query</strong></label>
            <input id="query" type="text" name="query" value="{{.Form.Query}}" placeholder="approval agentos">
          </div>
        </div>
        <div class="grid two">
          <div>
            <label for="args"><strong>Args</strong></label>
            <input id="args" type="text" name="args" value="{{.Form.Args}}" placeholder="--json state">
          </div>
          <div>
            <label for="content"><strong>Content</strong></label>
            <textarea id="content" name="content" placeholder="Optional file content for file-edit tasks.">{{.Form.Content}}</textarea>
          </div>
        </div>
        <label><input type="checkbox" name="requires_approval" {{if .Form.RequiresApproval}}checked{{end}}> requires approval</label>
        <div><button type="submit">Create Task</button></div>
      </form>
    </div>
  {{template "footer" .}}
{{end}}

{{define "approvals"}}
  {{template "header" .}}
    <h1>Approvals</h1>
    <div class="sub">Review privileged actions with enough context to decide quickly: what is being requested, why it is risky, and what task it would unblock.</div>
    <div class="grid three" style="margin-bottom:18px;">
      <div class="card"><h3>Total</h3><div class="metric">{{.Summary.Total}}</div><div class="muted">all approval records</div></div>
      <div class="card"><h3>Pending</h3><div class="metric">{{.Summary.Pending}}</div><div class="muted">awaiting operator decision</div></div>
      <div class="card"><h3>Denied</h3><div class="metric">{{.Summary.Denied}}</div><div class="muted">explicitly rejected</div></div>
    </div>
    <div class="card" style="margin-bottom:18px;">
      <form method="get" action="/ui/approvals" class="toolbar-form">
        <div>
          <label for="approval_query"><strong>Search</strong></label>
          <input id="approval_query" type="text" name="query" value="{{.Filter.Query}}" placeholder="summary, task id, metadata">
        </div>
        <div>
          <label for="approval_status"><strong>Status</strong></label>
          <select id="approval_status" name="status">
            <option value="">all</option>
            <option value="pending" {{if eq .Filter.Status "pending"}}selected{{end}}>pending</option>
            <option value="approved" {{if eq .Filter.Status "approved"}}selected{{end}}>approved</option>
            <option value="denied" {{if eq .Filter.Status "denied"}}selected{{end}}>denied</option>
            <option value="consumed" {{if eq .Filter.Status "consumed"}}selected{{end}}>consumed</option>
          </select>
        </div>
        <div>
          <label for="approval_action"><strong>Action Kind</strong></label>
          <select id="approval_action" name="action">
            <option value="">all</option>
            {{range .Actions}}<option value="{{.}}" {{if eq $.Filter.Action .}}selected{{end}}>{{.}}</option>{{end}}
          </select>
        </div>
        <div>
          <label for="approval_profile"><strong>Profile</strong></label>
          <select id="approval_profile" name="profile">
            <option value="">all</option>
            {{range .Profiles}}<option value="{{.}}" {{if eq $.Filter.Profile .}}selected{{end}}>{{.}}</option>{{end}}
          </select>
        </div>
        <div class="toolbar-actions">
          <button type="submit">Filter</button>
          {{if .HasFilter}}<a href="/ui/approvals" class="toggle-button">Reset</a>{{end}}
        </div>
      </form>
    </div>
    <div class="split">
      <div class="card">
        <h2>Queue</h2>
        <div class="dense-list">
          {{range .Approvals}}
            <a class="dense-item {{if and $.Selected (eq $.Selected.ApprovalID .ApprovalID)}}active{{end}}" href="/ui/approvals?approval_id={{.ApprovalID}}{{if $.Filter.Status}}&status={{queryEscape $.Filter.Status}}{{end}}{{if $.Filter.Action}}&action={{queryEscape $.Filter.Action}}{{end}}{{if $.Filter.Profile}}&profile={{queryEscape $.Filter.Profile}}{{end}}{{if $.Filter.Query}}&query={{queryEscape $.Filter.Query}}{{end}}" data-preview-url="{{previewURL (printf "/ui/approvals?approval_id=%s" (queryEscape .ApprovalID)) ""}}">
              <div>
                <div class="dense-title">{{.Summary}}</div>
                <div class="dense-sub">{{.ApprovalID}} · {{if .TaskID}}{{.TaskID}}{{else}}no task{{end}}</div>
                <div class="dense-sub">{{.CreatedAt.Format "Jan 2 15:04"}} · requested by {{if .RequestedBy}}{{.RequestedBy}}{{else}}unknown{{end}}</div>
              </div>
              <div class="dense-meta">
                <span class="pill danger">{{.ExecutionProfile}}</span>
                <span class="pill warn">{{.ActionKind}}</span>
                <span class="pill">{{.Status}}</span>
              </div>
            </a>
          {{else}}
            <div class="muted">No approvals found for the current filter.</div>
          {{end}}
        </div>
      </div>
      <div class="card">
        <h2>Decision Panel</h2>
        {{with .Selected}}
          <div class="stack">
            <div><span class="pill danger">{{.ExecutionProfile}}</span><span class="pill warn">{{.ActionKind}}</span><span class="pill">{{.Status}}</span></div>
            <div><strong>{{.Summary}}</strong></div>
            <div class="detail-grid">
              <div><strong>Approval ID</strong><div>{{.ApprovalID}}</div></div>
              <div><strong>Task</strong><div>{{if .TaskID}}<a href="/ui/tasks?task_id={{.TaskID}}">{{.TaskID}}</a>{{else}}None{{end}}</div></div>
              <div><strong>Requested By</strong><div>{{if .RequestedBy}}{{.RequestedBy}}{{else}}unknown{{end}}</div></div>
              <div><strong>Updated</strong><div>{{.UpdatedAt.Format "Jan 2 15:04:05"}}</div></div>
              <div><strong>Denied Reason</strong><div>{{if .DeniedReason}}{{.DeniedReason}}{{else}}None{{end}}</div></div>
              <div><strong>Expires</strong><div>{{if .ExpiresAt}}{{.ExpiresAt.Format "Jan 2 15:04:05"}}{{else}}None{{end}}</div></div>
            </div>
            <div class="decision-card danger">
              <div class="dense-title">Risk Summary</div>
              <div class="dense-sub">This request grants or finalizes a privileged action. Check the task context and metadata before approving.</div>
              <div class="detail-grid" style="margin-top:12px;">
                <div><strong>Action Kind</strong><div>{{.ActionKind}}</div></div>
                <div><strong>Execution Profile</strong><div>{{.ExecutionProfile}}</div></div>
              </div>
            </div>
            {{if eq .Status "pending"}}
              <div class="decision-card">
                <div class="dense-title">Decision</div>
                <div class="dense-sub">Approve to unblock execution, or deny with a reason so the runtime state remains explainable.</div>
                <div class="decision-actions">
                  <form class="inline" method="post" action="/ui/approvals/{{.ApprovalID}}/approve">
                    <button class="secondary" type="submit">Approve</button>
                  </form>
                </div>
                <form method="post" action="/ui/approvals/{{.ApprovalID}}/deny">
                  <input class="inline-input" type="text" name="reason" placeholder="Reason for denial">
                  <div class="decision-actions">
                    <button class="danger" type="submit">Deny</button>
                  </div>
                </form>
              </div>
            {{end}}
            <div><strong>Metadata</strong><pre>{{dictPreview .Metadata}}</pre></div>
            {{with $.SelectedTask}}
              <div>
                <strong>Task Context</strong>
                <pre>{{printf "title=%s\ngoal=%s\nstate=%s\nskill=%s\nnext=%s\nmetadata=%#v" .Title .Goal .State .SelectedSkill .NextAction .Metadata}}</pre>
              </div>
            {{end}}
          </div>
        {{else}}
          <div class="muted">Select an approval to inspect its context.</div>
        {{end}}
      </div>
    </div>
  {{template "footer" .}}
{{end}}

{{define "recall"}}
  {{template "header" .}}
    <h1>Recall</h1>
    <div class="sub">Query unified memory across web, email, and GitHub cards, then inspect the selected hit without leaving the control plane.</div>
    <div class="grid three" style="margin-bottom:18px;">
      <div class="card"><h3>Total Hits</h3><div class="metric">{{.Summary.Total}}</div><div class="muted">cards returned by the current recall</div></div>
      <div class="card"><h3>Top Source</h3><div class="metric">{{if .Summary.TopSource}}{{.Summary.TopSource}}{{else}}-{{end}}</div><div class="muted">source with the strongest footprint</div></div>
      <div class="card"><h3>Selected</h3><div class="metric">{{.Summary.Selected}}</div><div class="muted">detail pane follows the active hit</div></div>
    </div>
    <div class="card" style="margin-bottom:18px;">
      <form method="get" action="/ui/recall" class="toolbar-form">
        <div style="grid-column: span 2;">
          <label for="query"><strong>Query</strong></label>
          <input id="query" type="text" name="query" value="{{.Query}}" placeholder="approval agentos">
        </div>
        <div>
          <label><strong>Sources</strong></label>
          <div class="stack" style="gap:8px;">
            <label><input type="checkbox" name="source" value="web" {{if containsSource .Sources "web"}}checked{{end}}> Web</label>
            <label><input type="checkbox" name="source" value="email" {{if containsSource .Sources "email"}}checked{{end}}> Email</label>
            <label><input type="checkbox" name="source" value="github" {{if containsSource .Sources "github"}}checked{{end}}> GitHub</label>
          </div>
        </div>
        <div class="toolbar-actions">
          <button type="submit">Run Recall</button>
          {{if .HasFilter}}<a href="/ui/recall" class="toggle-button">Reset</a>{{end}}
        </div>
      </form>
      {{if .SourceCounts}}
        <div class="stack" style="margin-top:14px; gap:10px;">
          <div class="muted">Source mix</div>
          <div>
            {{range .SourceCounts}}<span class="pill {{if eq .Source "email"}}warn{{else if eq .Source "github"}}danger{{end}}">{{.Source}} {{.Count}}</span>{{end}}
          </div>
        </div>
      {{end}}
    </div>
    <div class="split">
      <div class="card">
        <h2>Result Queue</h2>
        <div class="dense-list">
          {{range .Response.Hits}}
            <a class="dense-item {{if and $.Selected (eq $.Selected.CardID .CardID)}}active{{end}}" href="/ui/recall?query={{queryEscape $.Query}}{{range $.Sources}}&source={{queryEscape .}}{{end}}&card_id={{queryEscape .CardID}}">
              <div>
                <div class="dense-title">{{preview (recallHitTitle .) 96}}</div>
                <div class="dense-sub">{{preview .Snippet 180}}</div>
                <div class="dense-sub">{{.CardType}} · {{.CardID}}</div>
              </div>
              <div class="dense-meta">
                <span class="pill">{{.Source}}</span>
                <span class="pill warn">{{printf "%.1f" .Score}}</span>
                {{if .MatchedFields}}<span class="pill">{{len .MatchedFields}} fields</span>{{end}}
              </div>
              <div class="dense-preview">Inspect the matched snippet, hit score, and selected card fields without leaving the recall queue.</div>
            </a>
          {{else}}
            <div class="muted">{{if .HasFilter}}No recall hits matched the current query.{{else}}Run a recall query to inspect cross-connector memory.{{end}}</div>
          {{end}}
        </div>
      </div>
      <div class="card">
        <h2>Hit Detail</h2>
        {{with .Selected}}
          <div class="stack">
            <div><span class="pill">{{.Source}}</span><span class="pill warn">{{.CardType}}</span><span class="pill">{{printf "%.1f" .Score}}</span></div>
            <div><strong>{{recallHitTitle .}}</strong></div>
            <div class="muted">{{.CardID}}</div>
            {{if .Snippet}}
              <div class="decision-card">
                <div class="dense-title">Matched Snippet</div>
                <div class="dense-sub">{{.Snippet}}</div>
              </div>
            {{end}}
            <div class="detail-grid">
              <div><strong>Matched Fields</strong><div>{{if .MatchedFields}}{{join .MatchedFields ", "}}{{else}}None{{end}}</div></div>
              <div><strong>Version</strong><div>{{.Card.Version}}</div></div>
              <div><strong>Status</strong><div>{{.Card.Status}}</div></div>
              <div><strong>Created</strong><div>{{.Card.CreatedAt.Format "Jan 2 15:04:05"}}</div></div>
            </div>
            <div>
              <strong>Card Fields</strong>
              <div class="dense-list" style="margin-top:10px;">
                {{range recallDetailFields .Card}}
                  <div class="dense-item" style="cursor:default;">
                    <div>
                      <div class="dense-title">{{index . 0}}</div>
                      <div class="dense-sub">{{preview (index . 1) 220}}</div>
                    </div>
                  </div>
                {{else}}
                  <div class="muted">No card fields available for this hit.</div>
                {{end}}
              </div>
            </div>
          </div>
        {{else}}
          <div class="muted">Select a recall hit to inspect its fields and matched context.</div>
        {{end}}
      </div>
    </div>
  {{template "footer" .}}
{{end}}

{{define "memory"}}
  {{template "header" .}}
    <h1>Memory</h1>
    <div class="sub">Inspect the latest durable cards, then review the selected card's fields and relationship edges in one place.</div>
    <div class="grid three" style="margin-bottom:18px;">
      <div class="card"><h3>Total Cards</h3><div class="metric">{{.Summary.Total}}</div><div class="muted">latest versions currently loaded</div></div>
      <div class="card"><h3>Card Types</h3><div class="metric">{{.Summary.CardTypes}}</div><div class="muted">distinct schemas represented</div></div>
      <div class="card"><h3>Selected Edges</h3><div class="metric">{{.Summary.Edges}}</div><div class="muted">relationships for the active card</div></div>
    </div>
    {{if .CardTypes}}
      <div class="card" style="margin-bottom:18px;">
        <div class="muted">Type mix</div>
        <div class="stack" style="margin-top:12px; gap:10px;">
          <div>{{range .CardTypes}}<span class="pill">{{.CardType}} {{.Count}}</span>{{end}}</div>
        </div>
      </div>
    {{end}}
    <div class="split">
      <div class="card">
        <h2>Card Queue</h2>
        <div class="dense-list">
          {{range .Cards}}
            <a class="dense-item {{if and $.Selected (eq $.Selected.CardID .CardID)}}active{{end}}" href="/ui/memory?card_id={{queryEscape .CardID}}">
              <div>
                <div class="dense-title">{{preview (memoryCardTitle .) 96}}</div>
                <div class="dense-sub">{{preview (printf "%v" .Content) 180}}</div>
                <div class="dense-sub">{{.CardID}} · {{.CreatedAt.Format "Jan 2 15:04"}}</div>
              </div>
              <div class="dense-meta">
                <span class="pill">{{.CardType}}</span>
                <span class="pill warn">v{{.Version}}</span>
                {{if .Status}}<span class="pill">{{.Status}}</span>{{end}}
              </div>
              <div class="dense-preview">Open the selected card detail, inspect its structured fields, and follow relationship edges to connected memory.</div>
            </a>
          {{else}}
            <div class="muted">No memory cards available.</div>
          {{end}}
        </div>
      </div>
      <div class="card">
        <h2>Card Detail</h2>
        {{with .Selected}}
          <div class="stack">
            <div><span class="pill">{{.CardType}}</span><span class="pill warn">v{{.Version}}</span>{{if .Status}}<span class="pill">{{.Status}}</span>{{end}}</div>
            <div><strong>{{memoryCardTitle .}}</strong></div>
            <div class="muted">{{.CardID}}</div>
            <div class="detail-grid">
              <div><strong>Created</strong><div>{{.CreatedAt.Format "Jan 2 15:04:05"}}</div></div>
              <div><strong>Previous Version</strong><div>{{if .PrevVersion}}{{.PrevVersion}}{{else}}None{{end}}</div></div>
              <div><strong>Provenance</strong><div>{{if .Provenance.Source}}{{.Provenance.Source}}{{else}}unknown{{end}}</div></div>
              <div><strong>Confidence</strong><div>{{if hasConfidence .Provenance.Confidence}}{{printf "%.2f" .Provenance.Confidence}}{{else}}n/a{{end}}</div></div>
            </div>
            <div>
              <strong>Card Fields</strong>
              <div class="dense-list" style="margin-top:10px;">
                {{range recallDetailFields .}}
                  <div class="dense-item" style="cursor:default;">
                    <div>
                      <div class="dense-title">{{index . 0}}</div>
                      <div class="dense-sub">{{preview (index . 1) 220}}</div>
                    </div>
                  </div>
                {{else}}
                  <div class="muted">No card fields available.</div>
                {{end}}
              </div>
            </div>
            <div>
              <strong>Relationships</strong>
              <div class="dense-list" style="margin-top:10px;">
                {{range $.SelectedEdges}}
                  <a class="dense-item" href="/ui/memory?card_id={{queryEscape (memoryEdgePeer . $.Selected.CardID)}}">
                    <div>
                      <div class="dense-title">{{.EdgeType}}</div>
                      <div class="dense-sub">Peer {{memoryEdgePeer . $.Selected.CardID}}</div>
                      <div class="dense-sub">{{.EdgeID}}</div>
                    </div>
                    <div class="dense-meta">
                      {{if .Confidence}}<span class="pill warn">{{printf "%.2f" .Confidence}}</span>{{end}}
                      {{if .Weight}}<span class="pill">{{printf "%.2f" .Weight}}</span>{{end}}
                    </div>
                    <div class="dense-preview">Jump directly to the connected peer card and continue walking the active memory relationship chain.</div>
                  </a>
                {{else}}
                  <div class="muted">No relationship edges for this card.</div>
                {{end}}
              </div>
            </div>
          </div>
        {{else}}
          <div class="muted">Select a card from the queue to inspect its fields and connected edges.</div>
        {{end}}
      </div>
    </div>
  {{template "footer" .}}
{{end}}

{{define "models"}}
  {{template "header" .}}
    <h1>Model Settings</h1>
    <div class="sub">Pick a provider, paste your API key, and run a live connectivity + <code>tool_calls</code> check — AgentOS needs the model to call tools for skills, chat, and runtime flows. Changes apply instantly; no restart required.</div>

    <style>
      .models-bar {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        align-items: center;
        margin-bottom: 12px;
      }
      .models-bar .pill { cursor: default; }
      .model-status-banner {
        border-radius: 16px;
        padding: 14px 16px;
        margin-bottom: 14px;
        border: 1px solid rgba(217,207,191,0.95);
        background: rgba(255,255,255,0.82);
        display: flex;
        flex-wrap: wrap;
        gap: 12px;
        row-gap: 8px;
        align-items: center;
      }
      .model-status-banner.ok {
        border-color: rgba(15,118,110,0.45);
        background: rgba(15,118,110,0.07);
      }
      .model-status-banner.warn {
        border-color: rgba(217,119,6,0.45);
        background: rgba(217,119,6,0.08);
      }
      .model-status-banner.err {
        border-color: rgba(185,28,28,0.5);
        background: rgba(185,28,28,0.08);
      }
      .model-status-banner .dot {
        width: 10px;
        height: 10px;
        border-radius: 999px;
        display: inline-block;
        flex: 0 0 10px;
      }
      .model-status-banner .dot.ok { background: #15803d; box-shadow: 0 0 0 3px rgba(21,128,61,0.18); }
      .model-status-banner .dot.warn { background: #b45309; box-shadow: 0 0 0 3px rgba(180,83,9,0.18); }
      .model-status-banner .dot.err { background: #b91c1c; box-shadow: 0 0 0 3px rgba(185,28,28,0.18); }
      .model-status-banner .dot.idle { background: #9ca3af; box-shadow: 0 0 0 3px rgba(156,163,175,0.18); }
      .model-status-banner .title { font-weight: 700; }
      .model-status-banner .detail { color: var(--muted); font-size: 13px; flex-basis: 100%; }
      .model-status-banner .actions { margin-left: auto; display: flex; gap: 8px; flex-wrap: wrap; }
      .welcome-card {
        border: 2px dashed rgba(15,118,110,0.4);
        border-radius: 16px;
        padding: 16px 18px;
        background: rgba(15,118,110,0.05);
        margin-bottom: 14px;
      }
      .welcome-card h2 { margin: 0 0 6px 0; font-size: 16px; }
      .welcome-card ol { margin: 8px 0 0 20px; padding: 0; font-size: 13px; line-height: 1.7; }
      .welcome-card ol code {
        font-family: "IBM Plex Mono", monospace;
        background: rgba(255,255,255,0.7);
        padding: 1px 6px;
        border-radius: 6px;
        font-size: 12px;
      }
      .snippet-row {
        display: flex;
        gap: 8px;
        align-items: center;
        margin-top: 8px;
        flex-wrap: wrap;
      }
      .snippet-row code {
        flex: 1;
        background: rgba(255,255,255,0.85);
        border: 1px solid rgba(217,207,191,0.9);
        border-radius: 8px;
        padding: 6px 10px;
        font-family: "IBM Plex Mono", monospace;
        font-size: 12px;
        overflow-x: auto;
      }
      .provider-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
        gap: 12px;
      }
      .provider-card {
        border: 2px solid rgba(217,207,191,0.95);
        border-radius: 18px;
        padding: 14px;
        background: rgba(255,255,255,0.82);
        cursor: pointer;
        transition: border-color 120ms ease, background 120ms ease, transform 80ms ease;
        display: flex;
        flex-direction: column;
        gap: 8px;
        min-height: 152px;
      }
      .provider-card:hover { background: rgba(255,255,255,0.96); border-color: rgba(15,118,110,0.35); }
      .provider-card.active {
        border-color: var(--accent);
        background: rgba(15,118,110,0.07);
        box-shadow: 0 6px 20px rgba(15,118,110,0.15);
      }
      .provider-card .title { font-weight: 700; font-size: 15px; }
      .provider-card .tagline { color: var(--muted); font-size: 12px; line-height: 1.45; }
      .provider-card .meta { display: flex; flex-wrap: wrap; gap: 6px; margin-top: auto; }
      .provider-card a.docs-link {
        font-size: 11px;
        color: var(--accent);
        text-decoration: underline;
      }
      .key-row {
        display: flex;
        gap: 8px;
        align-items: stretch;
      }
      .key-row input { flex: 1; }
      .key-row .eye-toggle {
        padding: 0 14px;
        font-family: inherit;
        border: 1px solid var(--line);
        background: rgba(255,255,255,0.9);
        border-radius: 10px;
        cursor: pointer;
        font-size: 13px;
      }
      .suggest-chips {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
        margin-top: 8px;
      }
      .suggest-chip {
        font-size: 12px;
        padding: 4px 10px;
        border-radius: 999px;
        border: 1px solid rgba(217,207,191,0.9);
        background: rgba(255,255,255,0.82);
        cursor: pointer;
        color: var(--ink);
      }
      .suggest-chip:hover { border-color: var(--accent); color: var(--accent); }
      .profile-chip {
        display: inline-flex;
        gap: 6px;
        align-items: center;
        padding: 4px 10px;
        border-radius: 999px;
        border: 1px solid rgba(217,207,191,0.95);
        background: rgba(255,255,255,0.88);
        font-size: 12px;
      }
      .profile-chip form { display: inline; }
      .profile-chip button.linklike {
        background: transparent;
        border: 0;
        padding: 0;
        color: var(--accent);
        cursor: pointer;
        font-size: 12px;
        font-weight: 700;
      }
      .profile-chip button.linklike.danger { color: var(--danger); }
      .test-result {
        padding: 12px 14px;
        border-radius: 14px;
        border: 1px solid rgba(217,207,191,0.95);
        background: rgba(255,250,240,0.82);
        font-size: 13px;
        line-height: 1.5;
      }
      .test-result.ok { border-color: rgba(15,118,110,0.45); background: rgba(15,118,110,0.07); }
      .test-result.err { border-color: rgba(185,28,28,0.5); background: rgba(185,28,28,0.07); }
      .test-result pre { max-height: 180px; overflow: auto; margin-top: 8px; }
      .test-result .row { display: flex; flex-wrap: wrap; gap: 12px; row-gap: 4px; }
      .persist-banner {
        border: 1px solid rgba(217,119,6,0.35);
        background: rgba(217,119,6,0.08);
        color: var(--ink);
        border-radius: 14px;
        padding: 10px 14px;
        font-size: 13px;
      }
      .persist-banner.ok {
        border-color: rgba(15,118,110,0.4);
        background: rgba(15,118,110,0.07);
      }
      .persist-banner code {
        font-family: "IBM Plex Mono", monospace;
        font-size: 12px;
        background: rgba(255,255,255,0.7);
        padding: 1px 6px;
        border-radius: 6px;
      }
    </style>

    {{/* ───── Current model health banner (A) ───── */}}
    {{if .LastTest}}
      {{if .LastTest.Ok}}
        <div class="model-status-banner ok">
          <span class="dot ok"></span>
          <div>
            <div class="title">Model online · {{.LastTest.Provider}} / {{.LastTest.Model}}</div>
            <div class="detail">
              Last test {{.LastTest.AgeHuman}} · {{.LastTest.LatencyMs}}ms
              {{if .LastTest.ToolCallsChecked}}
                · tool_calls {{if .LastTest.ToolCallsSupported}}<strong style="color:#15803d;">supported ✓</strong>{{else}}<strong style="color:#b45309;">not supported ✗</strong>{{end}}
              {{end}}
              {{if not .LastTest.Matches}}
                · <span style="color:#b45309;">form differs from tested config — Save to apply, or Re-test.</span>
              {{end}}
            </div>
          </div>
          <div class="actions">
            <button type="button" class="secondary" id="retest-banner-btn">Re-test now</button>
          </div>
        </div>
      {{else}}
        <div class="model-status-banner err">
          <span class="dot err"></span>
          <div>
            <div class="title">Model unreachable · {{if .LastTest.Provider}}{{.LastTest.Provider}}{{else}}n/a{{end}}{{if .LastTest.Model}} / {{.LastTest.Model}}{{end}}</div>
            <div class="detail">
              Last test {{.LastTest.AgeHuman}} failed: {{if .LastTest.Error}}{{.LastTest.Error}}{{else}}unknown error{{end}}
            </div>
          </div>
          <div class="actions">
            <button type="button" class="secondary" id="retest-banner-btn">Re-test now</button>
          </div>
        </div>
      {{end}}
    {{else}}
      <div class="model-status-banner warn">
        <span class="dot {{if .IsFirstRun}}warn{{else}}idle{{end}}"></span>
        <div>
          <div class="title">
            {{if .IsFirstRun}}
              No model configured yet
            {{else}}
              Never tested · {{if .Config.Provider}}{{.Config.Provider}}{{else}}none{{end}}{{if .Config.Conversation.Model}} / {{.Config.Conversation.Model}}{{end}}
            {{end}}
          </div>
          <div class="detail">
            {{if .IsFirstRun}}
              Click a provider card below, paste your API key, then run Test Connection to bring the AgentOS online.
            {{else}}
              Click "Test Connection" below to verify this model answers chat turns and supports tool_calls.
            {{end}}
          </div>
        </div>
        {{if not .IsFirstRun}}
        <div class="actions">
          <button type="button" class="secondary" id="retest-banner-btn">Test now</button>
        </div>
        {{end}}
      </div>
    {{end}}

    {{/* ───── First-run welcome card (B) ───── */}}
    {{if .IsFirstRun}}
      <div class="welcome-card">
        <h2>Get your AgentOS model online in 3 steps</h2>
        <ol>
          <li>Pick a provider card below. <strong>SiliconFlow</strong> is the fastest zero-spend path — click "get API key ↗" on the card.</li>
          <li>Paste the API key into the <code>API Key</code> field (show/hide eye button is next to it).</li>
          <li>Click <strong>Test Connection</strong>. A green badge + <code>tool_calls ✓</code> means the model can run skills, chat, and AgentOS workflows. Then click <strong>Save Model Settings</strong>.</li>
        </ol>
      </div>
    {{end}}

    {{/* ───── Persistence banner (F) ───── */}}
    {{if .PersistsSecrets}}
      <div class="persist-banner ok" style="margin-bottom:12px;">
        API keys <strong>will persist</strong> to <code>{{if .ConfigPath}}{{.ConfigPath}}{{else}}runtime/model/config.json{{end}}</code>. Make sure this file is gitignored.
      </div>
    {{else}}
      <div class="persist-banner" style="margin-bottom:12px;">
        <strong>Safe mode:</strong> saving will strip the API key from <code>{{if .ConfigPath}}{{.ConfigPath}}{{else}}runtime/model/config.json{{end}}</code> on disk, so the key only lives in memory until the daemon restarts.
        <div class="snippet-row">
          <span class="muted" style="font-size:12px;">To persist keys, add this to <code>.env.local</code> and restart the daemon:</span>
        </div>
        <div class="snippet-row">
          <code id="persist-snippet">{{.PersistSnippet}}</code>
          <button type="button" class="secondary" id="copy-persist-btn" style="white-space:nowrap;">Copy</button>
        </div>
      </div>
    {{end}}

    <div class="models-bar">
      {{if .SuccessMessage}}<span class="pill">{{.SuccessMessage}}</span>{{end}}
      {{if .TestMessage}}<span class="pill secondary">{{.TestMessage}}</span>{{end}}
      {{if .ErrorMessage}}<span class="pill danger">{{.ErrorMessage}}</span>{{end}}
      {{if .ProfileHint}}<span class="pill secondary">{{.ProfileHint}}</span>{{end}}
    </div>

    {{if .ProfilesEnabled}}
    <div class="card" style="margin-bottom:14px;">
      <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center;justify-content:space-between;">
        <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center;">
          <strong>Saved profiles:</strong>
          {{if .ProfileNames}}
            {{range .ProfileNames}}
              <span class="profile-chip">
                <span>{{.}}</span>
                <form method="get" action="/ui/models" title="Load into form"><input type="hidden" name="load_profile" value="{{.}}"><button type="submit" class="linklike">load</button></form>
                <form method="post" action="/ui/models/profile/apply" title="Apply as active config"><input type="hidden" name="profile_apply_name" value="{{.}}"><button type="submit" class="linklike">apply</button></form>
                <form method="post" action="/ui/models/profile/delete" onsubmit="return confirm('Delete profile {{.}}?');"><input type="hidden" name="profile_delete_name" value="{{.}}"><button type="submit" class="linklike danger">delete</button></form>
              </span>
            {{end}}
          {{else}}
            <span class="muted">no profiles saved yet</span>
          {{end}}
        </div>
        <div class="muted" style="font-size:12px;">Active: <strong>{{if .Config.Provider}}{{.Config.Provider}}{{else}}none{{end}}</strong>{{if .Config.Conversation.Model}} · {{.Config.Conversation.Model}}{{end}}</div>
      </div>
    </div>
    {{end}}

    <div class="split">
      <div class="card stack">
        <form id="{{.FormID}}" method="post" action="/ui/models" class="stack">

          <div>
            <div class="dense-title">1 · Choose a provider</div>
            <div class="muted" style="font-size:12px;margin-bottom:10px;">Click a card to auto-fill Base URL and recommended model. You can still tweak everything by hand below.</div>
            <div class="provider-grid" id="provider-cards">
              {{range .Presets}}
                <div class="provider-card{{if eq $.Config.Provider .ID}} active{{end}}"
                     data-provider="{{.ID}}"
                     data-base-url="{{.BaseURL}}"
                     data-default-model="{{.DefaultModel}}"
                     data-recommended="{{.RecommendedModel}}"
                     data-suggested="{{join .SuggestedModels "|"}}"
                     data-api-key-url="{{.APIKeyURL}}"
                     data-docs-url="{{.DocsURL}}"
                     data-notes="{{.Notes}}">
                  <div class="title">{{.Label}}</div>
                  <div class="tagline">{{.Tagline}}</div>
                  <div class="meta">
                    {{if .SupportsTools}}<span class="pill">tool_calls ✓</span>{{end}}
                    {{if .APIKeyURL}}<a class="docs-link" href="{{.APIKeyURL}}" target="_blank" rel="noopener">get API key ↗</a>{{end}}
                    {{if .DocsURL}}<a class="docs-link" href="{{.DocsURL}}" target="_blank" rel="noopener">docs ↗</a>{{end}}
                  </div>
                </div>
              {{end}}
            </div>
            <select id="provider" name="provider" style="margin-top:10px;">
              {{range .Providers}}
                <option value="{{.}}" {{if eq $.Config.Provider .}}selected{{end}}>{{.}}</option>
              {{end}}
            </select>
            <div class="muted" id="provider-notes" style="font-size:12px;margin-top:6px;"></div>
          </div>

          <div>
            <div class="dense-title">2 · Credentials</div>
            <div style="margin-top:8px;">
              <label class="muted" for="base_url">Base URL</label>
              <input id="base_url" type="text" name="base_url" value="{{.Config.BaseURL}}" placeholder="https://api.siliconflow.cn/v1">
            </div>
            <div style="margin-top:10px;">
              <label class="muted" for="api_key">API Key</label>
              <div class="key-row">
                <input id="api_key" type="password" name="api_key" value="" placeholder="Leave blank to keep current key" autocomplete="off" spellcheck="false">
                <button type="button" class="eye-toggle" id="eye-toggle" aria-label="Show API key">show</button>
              </div>
              <div class="muted" style="margin-top:6px;font-size:12px;">
                {{if .HasAPIKey}}Current key: <code>{{.MaskedAPIKey}}</code>{{else}}No API key configured.{{end}}
              </div>
            </div>
          </div>

          <div>
            <div class="dense-title">3 · Main model</div>
            <div class="muted" style="font-size:12px;margin-top:4px;">This is the <strong>Conversation</strong> model. By default the same name is used for Routing and Skills — uncheck "sync" if you want them separate.</div>
            <div style="margin-top:8px;">
              <label class="muted" for="conversation_model">Conversation Model</label>
              <input id="conversation_model" type="text" name="conversation_model" value="{{.Config.Conversation.Model}}" placeholder="e.g. Qwen/Qwen2.5-7B-Instruct">
              <div class="suggest-chips" id="suggest-chips"></div>
            </div>
            <label class="muted" style="display:inline-flex;align-items:center;gap:8px;margin-top:10px;font-size:13px;">
              <input type="checkbox" id="sync-models" checked>
              Sync the same model across Routing and Skills profiles (recommended)
            </label>
          </div>

          <details id="advanced-profiles">
            <summary>Advanced · per-profile overrides</summary>
            <div class="grid three" style="margin-top:12px;">
              <div class="decision-card">
                <div class="dense-title">Conversation Model</div>
                <div class="stack" style="margin-top:10px;">
                  <input type="number" name="conversation_max_tokens" value="{{.Config.Conversation.MaxTokens}}" placeholder="1600" min="0" max="128000" step="10">
                  <input type="number" name="conversation_temperature" value="{{printf "%.2f" .Config.Conversation.Temperature}}" placeholder="0.20" min="0" max="2" step="0.05">
                  <div class="muted" style="font-size:11px;">max_tokens · temperature</div>
                </div>
              </div>
              <div class="decision-card">
                <div class="dense-title">Routing Model</div>
                <div class="stack" style="margin-top:10px;">
                  <input type="text" id="routing_model" name="routing_model" value="{{.Config.Routing.Model}}" placeholder="same as Conversation">
                  <input type="number" name="routing_max_tokens" value="{{.Config.Routing.MaxTokens}}" placeholder="220" min="0" max="128000" step="10">
                  <div class="muted" style="font-size:11px;">Routing is always temperature 0.</div>
                </div>
              </div>
              <div class="decision-card">
                <div class="dense-title">Skills / Summary Model</div>
                <div class="stack" style="margin-top:10px;">
                  <input type="text" id="skills_model" name="skills_model" value="{{.Config.Skills.Model}}" placeholder="same as Conversation">
                  <input type="number" name="skills_max_tokens" value="{{.Config.Skills.MaxTokens}}" placeholder="1200" min="0" max="128000" step="10">
                  <input type="number" name="skills_temperature" value="{{printf "%.2f" .Config.Skills.Temperature}}" placeholder="0.20" min="0" max="2" step="0.05">
                </div>
              </div>
            </div>
          </details>

          <div class="grid two">
            <button type="submit">Save Model Settings</button>
            <button type="button" id="test-connection-btn" class="secondary">Test Connection · probe tool_calls</button>
          </div>

          <div id="test-result" class="test-result" style="display:none;"></div>

          {{if .ProfilesEnabled}}
            <div class="decision-card">
              <div class="dense-title">Save current form as a named profile</div>
              <div class="muted" style="font-size:12px;margin-top:6px;">Profiles include the API key — they're written to the local profiles file so you can switch providers in one click later.</div>
              <div class="key-row" style="margin-top:10px;">
                <input type="text" name="profile_save_name" value="" placeholder="e.g. siliconflow-work or deepseek-home">
                <button type="submit" class="secondary" formaction="/ui/models/profile/save">Save as profile</button>
              </div>
            </div>
          {{end}}
        </form>
      </div>

      <div class="card stack">
        <h2>Runtime Notes</h2>
        <div class="decision-card">
          <div class="dense-title">Current Provider</div>
          <div class="dense-sub">{{if .Config.Provider}}{{.Config.Provider}}{{else}}none{{end}}</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Base URL</div>
          <div class="dense-sub">{{if .Config.BaseURL}}{{.Config.BaseURL}}{{else}}not configured{{end}}</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Conversation Profile</div>
          <div class="dense-sub">{{.Config.Conversation.Model}} · max {{.Config.Conversation.MaxTokens}} · temp {{printf "%.2f" .Config.Conversation.Temperature}}</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Routing Profile</div>
          <div class="dense-sub">{{.Config.Routing.Model}} · max {{.Config.Routing.MaxTokens}} · temp 0.00</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Skills / Summary Profile</div>
          <div class="dense-sub">{{.Config.Skills.Model}} · max {{.Config.Skills.MaxTokens}} · temp {{printf "%.2f" .Config.Skills.Temperature}}</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Persistence</div>
          <div class="dense-sub">{{if .PersistsSecrets}}Keys persist to <code>{{.ConfigPath}}</code>{{else}}Safe mode · keys NOT written to disk. Set <code>MNEMOSYNE_MODEL_CONFIG_PATH</code> to a gitignored file to persist.{{end}}</div>
        </div>
        <div class="decision-card">
          <div class="dense-title">Supported APIs</div>
          <div class="dense-sub">OpenAI-compatible chat completions. The model MUST support <code>tools</code> / <code>tool_calls</code> — use the Test button to verify before running skills or chat.</div>
        </div>
      </div>
    </div>

    <script>
      (function() {
        var cards = document.querySelectorAll('#provider-cards .provider-card');
        var providerSelect = document.getElementById('provider');
        var baseURL = document.getElementById('base_url');
        var conversationModel = document.getElementById('conversation_model');
        var routingModel = document.getElementById('routing_model');
        var skillsModel = document.getElementById('skills_model');
        var sync = document.getElementById('sync-models');
        var chips = document.getElementById('suggest-chips');
        var notes = document.getElementById('provider-notes');
        var eye = document.getElementById('eye-toggle');
        var apiKey = document.getElementById('api_key');
        var testBtn = document.getElementById('test-connection-btn');
        var testResult = document.getElementById('test-result');
        var form = document.getElementById({{.FormID}});

        function applyPresetFromCard(card, force) {
          if (!card) return;
          var provider = card.getAttribute('data-provider');
          var preBase = card.getAttribute('data-base-url') || '';
          var defaultModel = card.getAttribute('data-default-model') || '';
          var suggested = (card.getAttribute('data-suggested') || '').split('|').filter(Boolean);
          var note = card.getAttribute('data-notes') || '';

          if (providerSelect) providerSelect.value = provider;

          if (baseURL && (force || !baseURL.value.trim()) && preBase) baseURL.value = preBase;
          if (conversationModel && (force || !conversationModel.value.trim()) && defaultModel) {
            conversationModel.value = defaultModel;
            syncModelIfNeeded();
          }

          if (chips) {
            chips.innerHTML = '';
            suggested.forEach(function(name) {
              var b = document.createElement('button');
              b.type = 'button';
              b.className = 'suggest-chip';
              b.textContent = name;
              b.addEventListener('click', function() {
                conversationModel.value = name;
                syncModelIfNeeded();
              });
              chips.appendChild(b);
            });
          }

          if (notes) notes.textContent = note;

          cards.forEach(function(c) { c.classList.remove('active'); });
          card.classList.add('active');
        }

        function syncModelIfNeeded() {
          if (!sync || !sync.checked) return;
          if (routingModel) routingModel.value = conversationModel.value;
          if (skillsModel) skillsModel.value = conversationModel.value;
        }

        cards.forEach(function(card) {
          card.addEventListener('click', function() { applyPresetFromCard(card, true); });
        });
        if (conversationModel) {
          conversationModel.addEventListener('input', syncModelIfNeeded);
        }
        if (sync) {
          sync.addEventListener('change', function() {
            if (sync.checked) syncModelIfNeeded();
          });
        }

        // Populate suggest chips / notes for the active provider on first load.
        var active = document.querySelector('#provider-cards .provider-card.active');
        if (active) applyPresetFromCard(active, false);

        if (eye && apiKey) {
          eye.addEventListener('click', function() {
            if (apiKey.type === 'password') {
              apiKey.type = 'text';
              eye.textContent = 'hide';
            } else {
              apiKey.type = 'password';
              eye.textContent = 'show';
            }
          });
        }

        // diagnoseError maps a raw error string from the model gateway into an
        // actionable hint shown under the red "Connection failed" card. This is
        // where we give OpenClaw-like "here is what to fix" guidance — most
        // real failures map to quota / auth / tool_calls / network.
        function diagnoseError(raw) {
          var msg = String(raw || '').toLowerCase();
          if (/quota|insufficient_quota|exceeded .*quota|billing|rate.?limit|too many requests/.test(msg)) {
            return 'Quota or rate limit exhausted on this provider. Either top up the account, or switch to a cheaper provider card below (SiliconFlow / DeepSeek both have free starter credits).';
          }
          if (/invalid.*api.?key|unauthorized|401|api key.*invalid|incorrect api key/.test(msg)) {
            return 'API key looks wrong. Click "get API key ↗" on the active provider card, generate a fresh key, paste it into the API Key field, then click Test Connection again.';
          }
          if (/403|forbidden|permission/.test(msg)) {
            return 'The provider refused the key (403). Common causes: this key does not have access to that model, the account is suspended, or IP is geo-blocked.';
          }
          if (/404|not.?found|unknown model|model.*not.*exist/.test(msg)) {
            return 'Model name not recognised by the provider. Click a chip under "Main model" to pick a known-good model for this provider.';
          }
          if (/timeout|deadline exceeded|i\/o timeout/.test(msg)) {
            return 'Request timed out — the endpoint is slow or unreachable. Check network / VPN / that the Base URL is correct.';
          }
          if (/no such host|dial tcp|connection refused|network|dns/.test(msg)) {
            return 'Cannot reach the Base URL. Double-check the URL ends with /v1 (or the compatible endpoint expected by the provider), and that your network allows outbound HTTPS.';
          }
          if (/context deadline|canceled/.test(msg)) {
            return 'Request was canceled before it finished. Retry or raise MNEMOSYNE timeouts.';
          }
          if (/unmarshal|parse|invalid json/.test(msg)) {
            return 'Got a response that is not OpenAI-compatible. Make sure the provider supports the chat/completions schema.';
          }
          return '';
        }

        function diagnoseTools(detail) {
          var msg = String(detail || '').toLowerCase();
          if (/plain text|without calling the tool|replied without/.test(msg)) {
            return 'This model does not emit tool_calls — AgentOS skills and the chat agent loop will fall back to plain replies. Switch to a model that supports tool_calls, for example Qwen/Qwen2.5-7B-Instruct or deepseek-chat.';
          }
          if (/probe failed|400|invalid/.test(msg)) {
            return 'tool_calls probe errored out. Some OpenAI-compatible gateways do not accept the "tools" field — try a different model or provider.';
          }
          return '';
        }

        function renderTestResult(data) {
          if (!testResult) return;
          testResult.style.display = '';
          if (data.error) {
            var hint = diagnoseError(data.error);
            testResult.className = 'test-result err';
            testResult.innerHTML =
              '<strong>✗ Connection failed</strong><br>' +
              '<span class="muted">' + escapeHTML(data.error) + '</span>' +
              (hint ? '<div style="margin-top:8px;padding:8px 10px;border-radius:10px;background:rgba(185,28,28,0.08);font-size:12px;line-height:1.45;"><strong>Fix suggestion:</strong> ' + escapeHTML(hint) + '</div>' : '');
            return;
          }
          var toolBadge = '';
          if (data.tool_calls_checked) {
            if (data.tool_calls_supported) {
              toolBadge = '<span class="pill">tool_calls ✓</span>';
            } else {
              toolBadge = '<span class="pill warn">tool_calls ✗</span>';
            }
          }
          var toolHint = '';
          if (data.tool_calls_checked && !data.tool_calls_supported) {
            var t = diagnoseTools(data.tool_calls_detail);
            if (t) {
              toolHint = '<div style="margin-top:8px;padding:8px 10px;border-radius:10px;background:rgba(217,119,6,0.1);font-size:12px;line-height:1.45;"><strong>Heads up:</strong> ' + escapeHTML(t) + '</div>';
            }
          }
          testResult.className = 'test-result ok';
          testResult.innerHTML =
            '<strong>✓ Connection ok</strong>' +
            '<div class="row" style="margin-top:6px;">' +
              '<span class="pill secondary">' + escapeHTML(data.provider || '') + '</span>' +
              '<span class="pill secondary">' + escapeHTML(data.model || '') + '</span>' +
              '<span class="pill secondary">' + (data.latency_ms || 0) + ' ms</span>' +
              toolBadge +
            '</div>' +
            (data.reply_preview ? '<div class="muted" style="margin-top:8px;font-size:12px;">reply: ' + escapeHTML(data.reply_preview) + '</div>' : '') +
            (data.tool_calls_detail ? '<div class="muted" style="margin-top:6px;font-size:12px;">tool_calls probe: ' + escapeHTML(data.tool_calls_detail) + '</div>' : '') +
            toolHint +
            '<div class="row" style="margin-top:10px;gap:8px;">' +
              '<button type="button" id="save-apply-btn">Save &amp; apply</button>' +
              '<span class="muted" style="font-size:12px;align-self:center;">Writes the current form as the active config.</span>' +
            '</div>';

          var saveApply = document.getElementById('save-apply-btn');
          if (saveApply) {
            saveApply.addEventListener('click', function() {
              if (!form) return;
              form.setAttribute('action', '/ui/models');
              form.submit();
            });
          }
        }

        function escapeHTML(s) {
          return String(s || '').replace(/[&<>"']/g, function(c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
          });
        }

        function runTest() {
          if (!form) return;
          var fd = new FormData(form);
          if (testBtn) testBtn.disabled = true;
          var banner = document.getElementById('retest-banner-btn');
          if (banner) banner.disabled = true;
          testResult.style.display = '';
          testResult.className = 'test-result';
          testResult.innerHTML = '<span class="muted">Testing connection and probing tool_calls…</span>';

          fetch('/ui/models/test.json', { method: 'POST', body: fd })
            .then(function(resp) { return resp.json(); })
            .then(renderTestResult)
            .catch(function(err) {
              renderTestResult({ error: 'request failed: ' + (err && err.message ? err.message : err) });
            })
            .finally(function() {
              if (testBtn) testBtn.disabled = false;
              if (banner) banner.disabled = false;
            });
        }

        if (testBtn) {
          testBtn.addEventListener('click', runTest);
        }
        var retestBanner = document.getElementById('retest-banner-btn');
        if (retestBanner) {
          retestBanner.addEventListener('click', function() {
            runTest();
            var r = document.getElementById('test-result');
            if (r && r.scrollIntoView) r.scrollIntoView({ behavior: 'smooth', block: 'center' });
          });
        }

        var copyBtn = document.getElementById('copy-persist-btn');
        if (copyBtn) {
          copyBtn.addEventListener('click', function() {
            var node = document.getElementById('persist-snippet');
            if (!node) return;
            var text = node.textContent || '';
            var done = function() {
              var old = copyBtn.textContent;
              copyBtn.textContent = 'Copied ✓';
              setTimeout(function() { copyBtn.textContent = old; }, 1500);
            };
            if (navigator.clipboard && navigator.clipboard.writeText) {
              navigator.clipboard.writeText(text).then(done).catch(function() {
                // fall through to legacy path
                legacyCopy(text, done);
              });
            } else {
              legacyCopy(text, done);
            }
          });
        }

        function legacyCopy(text, done) {
          var ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.left = '-9999px';
          document.body.appendChild(ta);
          ta.select();
          try { document.execCommand('copy'); } catch (e) {}
          document.body.removeChild(ta);
          if (done) done();
        }
      })();
    </script>
  {{template "footer" .}}
{{end}}

{{define "skills"}}
  {{template "header" .}}
    <h1>Skills</h1>
    <div class="sub">Manage builtin, manifest, and external skills without editing runtime dispatch code. This view shows the live registry plus manifest load health.</div>
    <div class="split">
      <div class="card stack">
        <h2>Registry</h2>
        {{if .SuccessMessage}}<div><span class="pill">{{.SuccessMessage}}</span></div>{{end}}
        {{if .ErrorMessage}}<div><span class="pill danger">{{.ErrorMessage}}</span></div>{{end}}
        <form method="post" action="/ui/skills/reload" class="inline">
          <button type="submit" class="secondary">Reload Skills</button>
        </form>
        <table>
          <thead>
            <tr><th>Name</th><th>Source</th><th>Execution</th><th>Maintenance</th><th>State</th></tr>
          </thead>
          <tbody>
            {{range .Skills}}
              <tr>
                <td>
                  <strong>{{.Name}}</strong>
                  {{if .Uses}}<div class="muted">uses {{.Uses}}</div>{{end}}
                  {{if .Description}}<div class="muted">{{.Description}}</div>{{end}}
                  {{if .External}}<div class="muted">external {{.External.Kind}} · {{.External.Command}}</div>{{end}}
                </td>
                <td><span class="pill {{if eq .Source "builtin"}}warn{{end}}">{{if .Source}}{{.Source}}{{else}}runtime{{end}}</span></td>
                <td>{{if .ExecutionProfile}}{{.ExecutionProfile}}{{else}}default{{end}}</td>
                <td>
                  {{if .MaintenancePolicy}}
                    <div class="muted">{{if .MaintenancePolicy.Enabled}}enabled{{else}}disabled{{end}} · {{if .MaintenancePolicy.Scope}}{{.MaintenancePolicy.Scope}}{{else}}scope:any{{end}}</div>
                    {{if .MaintenancePolicy.AllowedCardTypes}}<div class="muted">{{join .MaintenancePolicy.AllowedCardTypes ", "}}</div>{{end}}
                  {{else}}
                    <span class="muted">none</span>
                  {{end}}
                </td>
                <td>
                  <form method="post" action="/ui/skills/{{queryEscape .Name}}/toggle" class="inline">
                    <input type="hidden" name="enabled" value="{{if .Enabled}}false{{else}}true{{end}}">
                    <button type="submit" class="{{if .Enabled}}warn{{else}}secondary{{end}}">{{if .Enabled}}Disable{{else}}Enable{{end}}</button>
                  </form>
                </td>
              </tr>
            {{else}}
              <tr><td colspan="5" class="muted">No skills registered.</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
      <div class="card stack">
        <h2>Manifest Health</h2>
        {{if .ManifestStatuses}}
          <div class="stack">
            {{range .ManifestStatuses}}
              <div class="decision-card">
                <div class="dense-title">{{if .Name}}{{.Name}}{{else}}{{.Path}}{{end}}</div>
                <div class="dense-sub">{{.Path}}</div>
                <div style="margin-top:8px;">
                  {{if .Loaded}}<span class="pill">loaded</span>{{else}}<span class="pill danger">error</span>{{end}}
                  {{if .Version}}<span class="pill">v{{.Version}}</span>{{end}}
                  {{if .Source}}<span class="pill warn">{{.Source}}</span>{{end}}
                  {{if .Uses}}<span class="pill">uses {{.Uses}}</span>{{end}}
                  {{if .ExternalKind}}<span class="pill">external {{.ExternalKind}}</span>{{end}}
                </div>
                {{if .Name}}<div style="margin-top:8px;"><a class="pill" href="/ui/skills?manifest={{queryEscape .Name}}">Edit</a></div>{{end}}
                {{if .Error}}<div class="muted" style="margin-top:8px;">{{.Error}}</div>{{end}}
              </div>
            {{end}}
          </div>
        {{else}}
          <div class="muted">No manifest files have been loaded yet.</div>
        {{end}}
      </div>
    </div>
    <div class="card stack" style="margin-top:18px;">
      <h2>Save Manifest</h2>
      <div class="muted">Register or update a manifest-backed skill by pasting JSON. This writes to the runtime skills directory and reloads the registry.</div>
      <form method="post" action="/ui/skills/manifests" class="stack">
        <div>
          <label class="muted" for="manifest_json">Manifest JSON</label>
          <textarea id="manifest_json" name="manifest_json" style="min-height:320px;font-family:'IBM Plex Mono',monospace;">{{.ManifestJSON}}</textarea>
        </div>
        <button type="submit">Save Manifest</button>
      </form>
    </div>
  {{template "footer" .}}
{{end}}
`
