import './style.css';
import './schemes.css';
import {Navigate, Reload, Back, Forward, ClearHistory, FullHistory, BookmarkFor, NewTab, CloseTab, SummarizePage, ReadPage, SavePage, SaveSource, SetViewShown} from '../wailsjs/go/main/App';
import {OnFileDrop, EventsOn} from '../wailsjs/runtime/runtime';
import * as view from './view.js';
import {initConsole, toggleConsole} from './console.js';
import {initBookmarks, openBookmark} from './bookmarks.js';
import {initPanel, panelShortcut, refreshPanel, togglePanel} from './bookmarkpanel.js';
import {initStorage} from './storage.js';
import {initTagging} from './tagging.js';
import {initOrganize} from './organize.js';
import {initMenu, showCredentialState} from './theme.js';
import {initAI, openAI, aiReady, newRequestId, cancelRequest} from './ai.js';
import {initAsk, refreshAsk, toggleAsk} from './ask.js';
import {initReading} from './reading.js';
import {initSplitters} from './splitter.js';
import {initCompare, openCompare} from './compare.js';
import {initCredentials, openCredentials, credentialOrigins} from './creds.js';

const $ = (id) => document.getElementById(id);
const address = $('address');
const content = $('content');
const sourceBtn = $('source');

// Each tab is {id, page, pending, summary, chat, chatTried, showSource, seq, loading, scroll}. A restored tab
// that was not selected has only `pending`, its address, until it is first shown.
// The Go side keeps the tab's back/forward history under the same id; the last page shown is kept here
// so switching tabs does not fetch again.
const tabs = [];
let active = null;
let saved = null;        // the bookmark for the active tab's page, if any

const currentPage = () => active?.page ?? null;

/** The state of editing the active tab's source, for view.source. */
function editor() {
    const tab = active;
    return {
        draft: tab.draft ?? null,
        saving: !!tab.saving,
        error: tab.saveError ?? '',
        onEdit: () => { tab.draft = tab.page.source; tab.saveError = ''; render(); $('content').querySelector('.source-edit')?.focus(); },
        onInput: (text) => { tab.draft = text; },
        onSave: () => saveEdit(),
        onCancel: () => { tab.draft = null; tab.saveError = ''; render(); },
    };
}

/** Saves the edited source to the server; the tab then shows what is stored. */
async function saveEdit() {
    const tab = active;
    const p = tab?.page;
    if (!p?.editUrl || tab.draft == null || tab.saving) return;
    tab.saving = true;
    tab.saveError = '';
    render();
    try {
        const fresh = await SaveSource(tab.id, p.url, p.editUrl, p.etag, tab.draft);
        if (fresh.kind === 'error') throw new Error(fresh.error);
        tab.saving = false;
        show(tab, fresh);
        toast('Saved to the server.');
    } catch (e) {
        tab.saving = false;
        tab.saveError = String(e).replace(/^Error: /, '');
        if (tab === active) { render(); $('content').querySelector('.source-edit')?.focus(); }
    }
}

function render() {
    const p = currentPage();
    const showSource = active?.showSource ?? false;
    sourceBtn.disabled = p?.kind !== 'markdown';
    $('summarize').disabled = !p?.url || p.kind === 'error' || isSearch(p.url);
    sourceBtn.setAttribute('aria-pressed', String(showSource && !sourceBtn.disabled));
    const body = (
        !p ? view.welcome()
            : p.kind === 'markdown' ? (showSource ? view.source(p, editor()) : view.markdown(p))
            : p.kind === 'fallback' ? view.fallback(p, (origin) => openCredentials({origin}))
            : view.error(p, () => goActive((id) => Navigate(id, p.url)), (page) => openCredentials({origin: page.origin, kind: page.authScheme === 'basic' ? 'basic' : page.authScheme === 'bearer' ? 'bearer' : 'cookie'})));
    const extras = [copyBar(p), searchSwitch(p), active?.summary ? view.summary(active.summary, closeSummary, cancelSummary) : null].filter(Boolean);
    content.replaceChildren(...extras, body);
    content.scrollTop = 0;
}

// --- save as markdown ---

let toastTimer = 0;

/** A short message at the bottom of the window. */
function toast(text, isError = false) {
    const box = $('toast');
    box.textContent = text;
    box.classList.toggle('error', isError);
    box.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { box.hidden = true; }, isError ? 6000 : 3000);
}

/** Saves the page as a markdown file: its own markdown, or the title and links of any other page. */
async function savePage() {
    if (active?.draft != null) return saveEdit(); // while editing the source, Save means save the edit
    const p = currentPage();
    if (!p?.url || p.kind === 'error' || isSearch(p.url)) return toast('Open a page first, then save it.', true);
    try {
        const path = await SavePage(p);
        if (path) toast(`Saved ${path}`);
    } catch (e) {
        toast(String(e), true);
    }
}

// --- copy markdown ---

/** The markdown text of the page, when it has any: not a feed, whose source is XML. */
function markdownSource(p) {
    if (p?.kind !== 'markdown' || !p.source || /^\s*<(\?xml|rss|feed|rdf:RDF)/i.test(p.source.slice(0, 200))) return '';
    return p.source;
}

async function writeClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
    } catch { // no clipboard permission in this webview: the old way, through a selection
        const box = Object.assign(document.createElement('textarea'), {value: text});
        box.style.cssText = 'position:fixed;opacity:0';
        document.body.append(box);
        box.select();
        const ok = document.execCommand('copy');
        box.remove();
        if (!ok) throw new Error('the clipboard is not available');
    }
}

let copiedTimer = 0;

/** Copies the page's markdown and says so on the button. */
async function copyMarkdown() {
    const text = markdownSource(currentPage());
    if (!text) return;
    const btn = document.querySelector('.copy-md');
    try {
        await writeClipboard(text);
        if (btn) { btn.textContent = '\u2713 Copied'; btn.classList.add('done'); }
    } catch (e) {
        if (btn) btn.textContent = 'Copy failed';
        console.error(e);
    }
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => { if (btn?.isConnected) { btn.textContent = '\u29C9 Copy markdown'; btn.classList.remove('done'); } }, 1800);
}

/** A button at the top right of the page, shown while the pointer is over the page. */
function copyBar(p) {
    if (!markdownSource(p)) return null;
    const btn = Object.assign(document.createElement('button'), {
        type: 'button', className: 'copy-md', textContent: '\u29C9 Copy markdown',
        title: `Copy the markdown of this page (${isMac ? 'Cmd' : 'Ctrl'}+Shift+C)`,
    });
    btn.addEventListener('click', copyMarkdown);
    return Object.assign(document.createElement('div'), {className: 'copy-bar'}, ).appendChild(btn).parentNode;
}

// Bookmark searches live at "bookmarks:search?q=..."; show them as typed: "?query".
// Searches by meaning ("@ ...") live at "bookmarks:meaning?q=..." and are shown as "@query".
const SEARCH_PREFIX = 'bookmarks:search?q=';
const MEANING_PREFIX = 'bookmarks:meaning?q=';
const isSearch = (url) => url.startsWith(SEARCH_PREFIX) || url.startsWith(MEANING_PREFIX);
function searchQueryOf(url) {
    const prefix = url.startsWith(MEANING_PREFIX) ? MEANING_PREFIX : SEARCH_PREFIX;
    try { return decodeURIComponent(url.slice(prefix.length).replace(/\+/g, ' ')); } catch { return ''; }
}
function displayAddress(url) {
    if (!isSearch(url)) return url;
    return (url.startsWith(MEANING_PREFIX) ? '@' : '?') + searchQueryOf(url);
}

/** On a search results page: a button to run the same search the other way. */
function searchSwitch(p) {
    if (!p?.url || !isSearch(p.url) || p.kind === 'error') return null;
    const query = searchQueryOf(p.url);
    const byMeaning = p.url.startsWith(MEANING_PREFIX);
    if (!byMeaning && !query.trim()) return null;
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'search-switch';
    button.textContent = byMeaning ? 'Search by keywords instead' : '\u2728 Search by meaning with AI';
    button.title = byMeaning ? 'Every word must match' : 'Sends the titles, addresses, keywords and descriptions of your bookmarks and history to your AI provider';
    button.addEventListener('click', () => goActive((id) => Navigate(id, (byMeaning ? '?' : '@') + query)));
    return button;
}

/** Puts the active tab's page on screen: address bar, buttons, content. */
function display() {
    const p = currentPage();
    const url = p?.url ?? active?.pending ?? '';
    address.value = url ? displayAddress(url) : '';
    document.title = p?.title ? `${p.title} - Mardo` : 'Mardo - Markdown Browser';
    $('back').disabled = !p?.canBack;
    $('forward').disabled = !p?.canForward;
    $('reload').disabled = !p?.url;
    document.body.classList.toggle('loading', Boolean(active?.loading));
    render();
    refreshAsk();
    updateCredentialIndicator();
    updateBookmarkIndicator();
}

function show(tab, p) {
    tab.page = p;
    tab.pending = '';
    tab.summary = null;
    if (tab.chat && tab.chat.url !== p.url) tab.chat = null; // a conversation is about one page; it is saved and comes back with the page
    tab.showSource = false;
    tab.draft = null;
    tab.saveError = '';
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

/** The page title of a tab, or the one saved for it while it has not been loaded. */
function tabTitle(t) {
    if (t.page) return t.page.kind !== 'error' && t.page.title ? t.page.title : '';
    return t.pendingTitle ?? '';
}

function saveTabs() {
    const kept = tabs.filter((t) => t.page?.url || t.pending);
    try {
        localStorage.setItem(TABS_KEY, JSON.stringify({
            urls: kept.map((t) => t.page?.url ?? t.pending),
            titles: kept.map((t) => tabTitle(t)), // so tabs that are not loaded yet keep their names
            active: Math.max(0, kept.indexOf(active)),
        }));
    } catch { /* not persisted */ }
}

function savedTabs() {
    try {
        const s = JSON.parse(localStorage.getItem(TABS_KEY));
        const urls = Array.isArray(s?.urls) ? s.urls : [];
        const titles = Array.isArray(s?.titles) ? s.titles : []; // absent in what older versions saved
        const kept = urls.map((u, i) => ({url: u, title: titles[i]})).filter((t) => typeof t.url === 'string' && t.url);
        return {
            urls: kept.map((t) => t.url),
            titles: kept.map((t) => (typeof t.title === 'string' ? t.title : '')),
            active: Number.isInteger(s?.active) ? s.active : 0,
        };
    } catch { return {urls: [], titles: [], active: 0}; }
}

async function restoreTabs() {
    const {urls, titles, active: which} = savedTabs();
    if (!urls.length) return openTab();
    for (const [i, url] of urls.entries()) { // only the selected tab is fetched now; the rest load when shown
        const tab = await newTab();
        tab.pending = url;
        tab.pendingTitle = titles[i] ?? '';
        tabs.push(tab);
    }
    activate(tabs[Math.min(which, tabs.length - 1)]);
}

const tabStrip = $('tabs');

function tabLabel(t) {
    const p = t.page;
    if (!p) return t.pending ? (t.pendingTitle || displayAddress(t.pending)) : 'New tab';
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
const newTab = async () => ({id: await NewTab(), page: null, pending: '', summary: null, chat: null, chatTried: '', showSource: false, seq: 0, loading: false, scroll: 0});

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

function cancelSummary() {
    if (active.summary?.requestId) cancelRequest(active.summary.requestId);
    closeSummary();
}

/**
 * Runs an AI tool on the active page and shows the result in the card above it.
 * `tool` is "summary", "outline", "translate" or "digest".
 */
export async function readPage(tool, option = '') {
    const tab = active;
    const url = tab?.page?.url;
    if (!url || $('summarize').disabled) return;
    if (!(await aiReady())) return openAI(); // first use: choose a provider
    const requestId = newRequestId();
    const label = {
        summary: ['Summary', 'Reading the page and asking the model'],
        outline: ['Outline', 'Reading the page and outlining it'],
        translate: [`Translation into ${option}`, `Translating the page into ${option}`],
        digest: ['Digest', 'Reading the feed and writing a digest'],
    }[tool];
    const mine = {state: 'loading', url, requestId, title: label[0], busy: label[1]};
    tab.summary = mine;
    if (tab === active) render();
    let result;
    try {
        const r = tool === 'summary' ? await SummarizePage(requestId, url) : await ReadPage(requestId, url, tool, option);
        result = {state: 'done', url, title: label[0], html: r.html, source: r.source, millis: r.millis};
    } catch (e) {
        result = {state: 'error', url, title: label[0], error: String(e)};
    }
    if (tab.summary !== mine || tab.page?.url !== url) return; // cancelled or closed, or the tab moved on
    tab.summary = result;
    if (tab === active) render();
}
const summarize = () => readPage('summary');
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

const menuItems = {
    credentials: () => openCredentials({origin: originOf(currentPage()?.url ?? '').replace(/^null$/, '')}),
    ai: openAI,
    savePage,
    history: () => goActive(FullHistory),
    clearHistory: () => ClearHistory(),
};

/** Runs a bookmark tool from the menu by pressing its button, with the panel open so its messages are seen. */
async function bookmarkTool(button) {
    await togglePanel(true);
    $(button).click();
}

// What the macOS menu bar does: its items have the shortcuts, and send their id here.
const menuActions = {
    'new-tab': () => openTab(),
    'close-tab': () => closeTab(active),
    'save-page': savePage,
    'copy-markdown': copyMarkdown,
    'bookmark-page': () => { if (currentPage()?.url && !isSearch(currentPage().url)) openBookmarkDialog(); },
    reload: () => { if (currentPage()?.url) goActive(Reload); },
    back: () => { if (!$('back').disabled) goActive(Back); },
    forward: () => { if (!$('forward').disabled) goActive(Forward); },
    address: () => address.focus(),
    // The menu item says Show or Hide, so it does just that (the Ctrl+B key moves into the panel first).
    bookmarks: () => ($('bookmarks-panel').hidden ? panelShortcut() : togglePanel(false)),
    'bookmarks-new-folder': () => bookmarkTool('folder-new'),
    'bookmarks-organize': () => bookmarkTool('bookmarks-organize'),
    'bookmarks-tag': () => bookmarkTool('bookmarks-tag'),
    'bookmarks-import': () => bookmarkTool('bookmarks-import'),
    'bookmarks-export': () => bookmarkTool('bookmarks-export'),
    'bookmarks-export-md': () => bookmarkTool('bookmarks-export-md'),
    'bookmarks-storage': () => bookmarkTool('storage-open'),
    console: toggleConsole,
    source: toggleSource,
    ask: toggleAsk,
    summary: summarize,
    ai: menuItems.ai,
    credentials: menuItems.credentials,
    history: menuItems.history,
    'clear-history': menuItems.clearHistory,
};
// Select All depends on where you are: a field selects its text, the bookmarks panel its
// folders and bookmarks, the chat its whole conversation, otherwise the page.
let lastPane = null;
document.addEventListener('pointerdown', (e) => { lastPane = e.target.closest('#bookmarks-panel, #ask-panel, #content'); }, true);
function selectAll() {
    const focus = document.activeElement;
    if (focus?.matches('input:not([type=checkbox]):not([type=radio]), textarea')) { focus.select(); return; }
    const pane = focus?.closest?.('#bookmarks-panel, #ask-panel, #content') ?? lastPane;
    const target = pane?.id === 'bookmarks-panel' ? $('bookmarks-tree')
        : pane?.id === 'ask-panel' ? $('ask-log')
        : content;
    const range = document.createRange();
    range.selectNodeContents(target);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
}
menuActions['select-all'] = selectAll;
EventsOn('menu', (id) => menuActions[id]?.());

// The macOS View menu says Show or Hide for these four. Watch them rather than every place that changes them.
const viewShown = {};
function syncViewMenu() {
    const now = {
        bookmarks: !$('bookmarks-panel').hidden,
        console: !$('console').hidden,
        source: sourceBtn.getAttribute('aria-pressed') === 'true',
        ask: !$('ask-panel').hidden,
    };
    for (const [id, shown] of Object.entries(now)) {
        if (viewShown[id] !== shown) { viewShown[id] = shown; if (navigator.platform.startsWith('Mac')) SetViewShown(id, shown); }
    }
}
const viewObserver = new MutationObserver(syncViewMenu);
for (const [id, attr] of [['bookmarks-panel', 'hidden'], ['console', 'hidden'], ['ask-panel', 'hidden']]) viewObserver.observe($(id), {attributes: true, attributeFilter: [attr]});
viewObserver.observe(sourceBtn, {attributes: true, attributeFilter: ['aria-pressed']});
syncViewMenu();

// Keyboard shortcuts. "mod" is Cmd on macOS and Ctrl elsewhere.
const isMac = navigator.platform.startsWith('Mac');
/** True for the keys the macOS menu bar has as shortcuts. */
function menuOwnsKey(e, mod, key) {
    if (!mod || e.altKey) return false;
    if (e.shiftKey) return key === 'a' || key === 'c';
    return 'lrujbdtws[]'.includes(key) && key.length === 1;
}
document.addEventListener('keydown', (e) => {
    const mod = isMac ? e.metaKey : e.ctrlKey;
    const key = e.key.toLowerCase();
    if (isMac && menuOwnsKey(e, mod, key)) return; // the menu bar runs it and sends a "menu" event
    let handled = true;
    if (mod && key === 'l') address.focus();
    else if (mod && key === 'r') { if (currentPage()?.url) goActive(Reload); }
    else if (mod && key === 'u') toggleSource();
    else if (mod && e.shiftKey && key === 'a') toggleAsk();
    else if (mod && e.shiftKey && key === 'c') copyMarkdown();
    else if (mod && !e.shiftKey && key === 's') savePage();
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
initTagging(() => { updateBookmarkIndicator(); refreshPanel(); });
initOrganize(() => { updateBookmarkIndicator(); refreshPanel(); });

initMenu(menuItems);
initAI();
initAsk({
    tab: () => active,
    open: (url, newTab) => (newTab ? openTab(url, {background: true}) : goActive((id) => Navigate(id, url))),
});
initReading({page: currentPage, run: readPage, compare: openCompare});
initSplitters();
initCompare({
    // A restored tab that has not been shown yet has only its address (tab.pending); the Go side reads it from there.
    tabs: () => tabs.filter((t) => t.page?.kind !== 'error').map((t) => ({title: tabTitle(t), url: t.page?.url || t.pending || ''})),
    open: (url, newTab) => (newTab ? openTab(url, {background: true}) : openTab(url)),
});
initConsole();
restoreTabs();
address.focus();
