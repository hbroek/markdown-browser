import './style.css';
import './schemes.css';
import {Navigate, Reload, Back, Forward, History, ClearHistory, FullHistory, BookmarkFor, NewTab, CloseTab, SummarizePage} from '../wailsjs/go/main/App';
import {OnFileDrop} from '../wailsjs/runtime/runtime';
import * as view from './view.js';
import {initConsole, toggleConsole} from './console.js';
import {initBookmarks, openBookmark} from './bookmarks.js';
import {initPanel, panelShortcut, refreshPanel} from './bookmarkpanel.js';
import {initStorage} from './storage.js';
import {initMenu, showCredentialState} from './theme.js';
import {initAI, openAI, aiReady} from './ai.js';
import {initCredentials, openCredentials, credentialOrigins} from './creds.js';

const $ = (id) => document.getElementById(id);
const address = $('address');
const content = $('content');
const sourceBtn = $('source');

// Each tab is {id, page, pending, summary, showSource, seq, loading, scroll}. A restored tab
// that was not selected has only `pending`, its address, until it is first shown.
// The Go side keeps the tab's back/forward history under the same id; the last page shown is kept here
// so switching tabs does not fetch again.
const tabs = [];
let active = null;
let saved = null;        // the bookmark for the active tab's page, if any

const currentPage = () => active?.page ?? null;

function render() {
    const p = currentPage();
    const showSource = active?.showSource ?? false;
    sourceBtn.disabled = p?.kind !== 'markdown';
    $('summarize').disabled = !p?.url || p.kind === 'error' || isSearch(p.url);
    sourceBtn.setAttribute('aria-pressed', String(showSource && !sourceBtn.disabled));
    const body = (
        !p ? view.welcome()
            : p.kind === 'markdown' ? (showSource ? view.source(p) : view.markdown(p))
            : p.kind === 'fallback' ? view.fallback(p, (origin) => openCredentials({origin}))
            : view.error(p, () => goActive((id) => Navigate(id, p.url)), (page) => openCredentials({origin: page.origin, kind: page.authScheme === 'basic' ? 'basic' : page.authScheme === 'bearer' ? 'bearer' : 'cookie'})));
    content.replaceChildren(...(active?.summary ? [view.summary(active.summary, closeSummary), body] : [body]));
    content.scrollTop = 0;
}

// Bookmark searches live at "bookmarks:search?q=..."; show them as typed: "?query".
const SEARCH_PREFIX = 'bookmarks:search?q=';
const isSearch = (url) => url.startsWith(SEARCH_PREFIX);
function displayAddress(url) {
    if (!isSearch(url)) return url;
    try { return '?' + decodeURIComponent(url.slice(SEARCH_PREFIX.length).replace(/\+/g, ' ')); } catch { return url; }
}

/** Puts the active tab's page on screen: address bar, buttons, content. */
function display() {
    const p = currentPage();
    const url = p?.url ?? active?.pending ?? '';
    address.value = url ? displayAddress(url) : '';
    document.title = p?.title ? `${p.title} - Markdown Browser` : 'Markdown Browser';
    $('back').disabled = !p?.canBack;
    $('forward').disabled = !p?.canForward;
    $('reload').disabled = !p?.url;
    document.body.classList.toggle('loading', Boolean(active?.loading));
    render();
    updateCredentialIndicator();
    updateBookmarkIndicator();
}

function show(tab, p) {
    tab.page = p;
    tab.pending = '';
    tab.summary = null;
    tab.showSource = false;
    if (tab === active) display();
    renderTabs();
}

/** Runs a navigation for `tab`; a response that was superseded in the meantime is dropped. */
async function go(tab, action) {
    const mine = ++tab.seq;
    tab.loading = true;
    if (tab === active) document.body.classList.add('loading');
    renderTabs();
    try {
        const p = await action();
        if (mine === tab.seq && tabs.includes(tab)) show(tab, p);
    } catch (e) {
        if (mine === tab.seq && tabs.includes(tab)) show(tab, {kind: 'error', error: String(e), url: tab.page?.url ?? ''});
    } finally {
        if (mine === tab.seq) {
            tab.loading = false;
            if (tab === active) document.body.classList.remove('loading');
            renderTabs();
        }
    }
}

/** Runs `fn(tabId)` in the tab that is active now. */
function goActive(fn) {
    const tab = active;
    return go(tab, () => fn(tab.id));
}

// --- tabs ---

// The open tabs (their current addresses and which is selected) survive a restart.
// Each one's back/forward history starts afresh.
const TABS_KEY = 'tabs';

function saveTabs() {
    const kept = tabs.filter((t) => t.page?.url || t.pending);
    try {
        localStorage.setItem(TABS_KEY, JSON.stringify({
            urls: kept.map((t) => t.page?.url ?? t.pending),
            active: Math.max(0, kept.indexOf(active)),
        }));
    } catch { /* not persisted */ }
}

function savedTabs() {
    try {
        const s = JSON.parse(localStorage.getItem(TABS_KEY));
        const urls = Array.isArray(s?.urls) ? s.urls.filter((u) => typeof u === 'string' && u) : [];
        return {urls, active: Number.isInteger(s?.active) ? s.active : 0};
    } catch { return {urls: [], active: 0}; }
}

async function restoreTabs() {
    const {urls, active: which} = savedTabs();
    if (!urls.length) return openTab();
    for (const url of urls) { // only the selected tab is fetched now; the rest load when shown
        const tab = await newTab();
        tab.pending = url;
        tabs.push(tab);
    }
    activate(tabs[Math.min(which, tabs.length - 1)]);
}

const tabStrip = $('tabs');

function tabLabel(t) {
    const p = t.page;
    if (!p) return t.pending ? displayAddress(t.pending) : 'New tab';
    if (p.title && p.kind !== 'error') return p.title;
    return p.url ? displayAddress(p.url) : 'New tab';
}

function renderTabs() {
    saveTabs();
    tabStrip.replaceChildren(
        ...tabs.map((t) => {
            const label = (t.loading ? '\u23F3 ' : '') + tabLabel(t);
            const close = document.createElement('button');
            close.type = 'button';
            close.className = 'tab-close';
            close.textContent = '\u00D7';
            close.title = 'Close tab';
            close.tabIndex = -1;
            close.addEventListener('click', (e) => { e.stopPropagation(); closeTab(t); });
            const el = document.createElement('div');
            el.className = 'tab';
            el.setAttribute('role', 'tab');
            el.setAttribute('aria-selected', String(t === active));
            el.tabIndex = t === active ? 0 : -1;
            el.title = label;
            const text = document.createElement('span');
            text.className = 'tab-title';
            text.textContent = label;
            el.append(text, close);
            el.addEventListener('click', () => activate(t));
            el.addEventListener('auxclick', (e) => { if (e.button === 1) { e.preventDefault(); closeTab(t); } });
            el.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); activate(t); }
            });
            return el;
        }),
    );
    tabStrip.querySelector('[aria-selected="true"]')?.scrollIntoView({block: 'nearest', inline: 'nearest'});
}

function activate(tab) {
    if (tab === active) return;
    if (active) active.scroll = content.scrollTop;
    active = tab;
    if (tab.pending && !tab.loading) { // a restored tab shows its page for the first time
        const url = tab.pending;
        go(tab, () => Navigate(tab.id, url));
    }
    display();
    content.scrollTop = tab.scroll ?? 0;
    renderTabs();
    if (!tab.page && !tab.pending) address.focus();
}

/** Opens a tab, loading `url` in it when given. A background tab does not take over the window. */
const newTab = async () => ({id: await NewTab(), page: null, pending: '', summary: null, showSource: false, seq: 0, loading: false, scroll: 0});

async function openTab(url = '', {background = false} = {}) {
    const tab = await newTab();
    const at = active ? tabs.indexOf(active) + 1 : tabs.length; // next to the current one
    tabs.splice(at, 0, tab);
    if (!background || !active) activate(tab);
    renderTabs();
    if (url) go(tab, () => Navigate(tab.id, url));
    return tab;
}

function closeTab(tab) {
    const i = tabs.indexOf(tab);
    if (i < 0) return;
    CloseTab(tab.id);
    tabs.splice(i, 1);
    if (!tabs.length) {
        active = null;
        openTab(); // the window always has a tab
        return;
    }
    if (tab === active) {
        active = null; // so activate() does not store scroll for the closed tab
        activate(tabs[Math.min(i, tabs.length - 1)]);
    } else {
        renderTabs();
    }
}

function stepTab(by) {
    if (tabs.length < 2) return;
    activate(tabs[(tabs.indexOf(active) + by + tabs.length) % tabs.length]);
}

$('tab-new').addEventListener('click', () => openTab());

// --- AI summary ---

function closeSummary() {
    active.summary = null;
    render();
}

async function summarize() {
    const tab = active;
    const url = tab?.page?.url;
    if (!url || $('summarize').disabled) return;
    if (!(await aiReady())) return openAI(); // first use: choose a provider
    tab.summary = {state: 'loading', url};
    if (tab === active) render();
    let result;
    try {
        const r = await SummarizePage(url);
        result = {state: 'done', url, html: r.html, source: r.source, millis: r.millis};
    } catch (e) {
        result = {state: 'error', url, error: String(e)};
    }
    if (tab.summary?.url !== url || tab.page?.url !== url) return; // closed, or the tab moved on
    tab.summary = result;
    if (tab === active) render();
}
$('summarize').addEventListener('click', summarize);

function toggleSource() {
    if (sourceBtn.disabled) return;
    active.showSource = !active.showSource;
    render();
}

address.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { const url = address.value; goActive((id) => Navigate(id, url)); }
});
address.addEventListener('focus', () => address.select());
$('back').addEventListener('click', () => goActive(Back));
$('forward').addEventListener('click', () => goActive(Forward));
$('reload').addEventListener('click', () => goActive(Reload));
sourceBtn.addEventListener('click', toggleSource);

// Links in rendered content navigate inside the browser; fragments scroll in place.
content.addEventListener('click', (e) => {
    const a = e.target.closest('a[href]');
    if (!a) {
        if (e.target.matches('.markdown img')) e.target.classList.toggle('zoomed');
        return;
    }
    e.preventDefault();
    const href = a.getAttribute('href');
    if (href.startsWith('#')) {
        document.getElementById(decodeURIComponent(href.slice(1)))?.scrollIntoView();
    } else if (e.metaKey || e.ctrlKey) {
        openTab(a.href, {background: true}); // Cmd/Ctrl+click: a tab in the background
    } else {
        const url = a.href;
        goActive((id) => Navigate(id, url));
    }
});
content.addEventListener('auxclick', (e) => { // middle click
    const a = e.button === 1 && e.target.closest('a[href]');
    if (!a || a.getAttribute('href').startsWith('#')) return;
    e.preventDefault();
    openTab(a.href, {background: true});
});

// History dropdown: loaded fresh each time it opens.
const historyBtn = $('history');
const historyMenu = $('history-menu');

function closeHistory() {
    historyMenu.hidden = true;
    historyBtn.setAttribute('aria-expanded', 'false');
}

async function toggleHistory() {
    if (!historyMenu.hidden) return closeHistory();
    const entries = (await History()) ?? [];
    historyMenu.replaceChildren(...view.historyMenu(
        entries,
        (url) => { closeHistory(); goActive((id) => Navigate(id, url)); },
        async () => { await ClearHistory(); closeHistory(); },
        () => { closeHistory(); goActive(FullHistory); },
    ));
    historyMenu.hidden = false;
    historyBtn.setAttribute('aria-expanded', 'true');
}

historyBtn.addEventListener('click', toggleHistory);
document.addEventListener('mousedown', (e) => {
    if (!historyMenu.hidden && !e.target.closest('.menu-wrap')) closeHistory();
});
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeHistory();
});

// Keyboard shortcuts. "mod" is Cmd on macOS and Ctrl elsewhere.
const isMac = navigator.platform.startsWith('Mac');
document.addEventListener('keydown', (e) => {
    const mod = isMac ? e.metaKey : e.ctrlKey;
    const key = e.key.toLowerCase();
    let handled = true;
    if (mod && key === 'l') address.focus();
    else if (mod && key === 'r') { if (currentPage()?.url) goActive(Reload); }
    else if (mod && key === 'u') toggleSource();
    else if (mod && key === 't') openTab();
    else if (mod && key === 'w') closeTab(active);
    else if (e.ctrlKey && e.key === 'Tab') stepTab(e.shiftKey ? -1 : 1);
    else if (mod && e.shiftKey && (e.key === '{' || e.key === '[')) stepTab(-1);
    else if (mod && e.shiftKey && (e.key === '}' || e.key === ']')) stepTab(1);
    else if (mod && !e.shiftKey && !e.altKey && /^[1-9]$/.test(e.key)) activate(tabs[e.key === '9' ? tabs.length - 1 : Number(e.key) - 1] ?? active);
    else if (mod && key === 'j') toggleConsole();
    else if (mod && key === 'b') panelShortcut();
    else if (mod && key === 'd') { if (currentPage()?.url && !isSearch(currentPage().url)) openBookmarkDialog(); }
    else if (e.altKey && e.key === 'ArrowLeft' || mod && e.key === '[') { if (!$('back').disabled) goActive(Back); }
    else if (e.altKey && e.key === 'ArrowRight' || mod && e.key === ']') { if (!$('forward').disabled) goActive(Forward); }
    else handled = false;
    if (handled) e.preventDefault();
});

// Drop a file anywhere on the window to open it.
OnFileDrop((x, y, paths) => {
    if (paths?.length) goActive((id) => Navigate(id, paths[0]));
}, true);

// Site credentials: after saving for the page's own site, load it again.
function originOf(url) {
    try { return new URL(url).origin; } catch { return ''; }
}
// The key button lights up when the current site has a saved credential.
async function updateCredentialIndicator() {
    const origin = originOf(currentPage()?.url ?? '');
    const saved = origin && (await credentialOrigins()).has(origin);
    showCredentialState(Boolean(saved), origin);
}

initCredentials((origin) => {
    updateCredentialIndicator();
    if (currentPage()?.url && originOf(currentPage().url) === originOf(origin)) goActive(Reload);
});

// The ribbon is an outline until the current page is bookmarked, then filled.
const bookmarkBtn = $('bookmark-add');
async function updateBookmarkIndicator() {
    const url = currentPage()?.url ?? '';
    bookmarkBtn.disabled = !url || isSearch(url); // a results page is not a page to save
    let found = null;
    if (url && !isSearch(url)) {
        try { found = await BookmarkFor(url); } catch { /* no backend: stays unsaved */ }
    }
    if ((currentPage()?.url ?? '') !== url) return; // navigated away meanwhile
    saved = found;
    bookmarkBtn.classList.toggle('active', Boolean(saved));
    bookmarkBtn.title = saved ? 'Edit bookmark (Ctrl/Cmd+D)' : 'Add bookmark (Ctrl/Cmd+D)';
}
function openBookmarkDialog() {
    openBookmark({bookmark: saved, url: currentPage().url, title: currentPage().title});
}
bookmarkBtn.addEventListener('click', openBookmarkDialog);
initBookmarks(() => { updateBookmarkIndicator(); refreshPanel(); });
initPanel((url, newTab) => newTab ? openTab(url) : goActive((id) => Navigate(id, url)), updateBookmarkIndicator);
initStorage(() => { updateBookmarkIndicator(); refreshPanel(); });

initMenu(() => openCredentials({origin: originOf(currentPage()?.url ?? '').replace(/^null$/, '')}), openAI);
initAI();
initConsole();
restoreTabs();
address.focus();
