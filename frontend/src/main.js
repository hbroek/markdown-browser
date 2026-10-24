import './style.css';
import {Navigate, Reload, Back, Forward, History, ClearHistory, FullHistory} from '../wailsjs/go/main/App';
import {OnFileDrop} from '../wailsjs/runtime/runtime';
import * as view from './view.js';
import {initConsole, toggleConsole} from './console.js';
import {initCredentials, openCredentials, credentialOrigins} from './creds.js';

const $ = (id) => document.getElementById(id);
const address = $('address');
const content = $('content');
const sourceBtn = $('source');

let current = null;      // last page shown
let showSource = false;
let seq = 0;             // ignores responses to superseded navigations

function render() {
    const p = current;
    sourceBtn.disabled = p?.kind !== 'markdown';
    sourceBtn.setAttribute('aria-pressed', String(showSource && !sourceBtn.disabled));
    content.replaceChildren(
        !p ? view.welcome()
            : p.kind === 'markdown' ? (showSource ? view.source(p) : view.markdown(p))
            : p.kind === 'fallback' ? view.fallback(p, (origin) => openCredentials({origin}))
            : view.error(p, () => go(() => Navigate(p.url)), (page) => openCredentials({origin: page.origin, kind: page.authScheme === 'basic' ? 'basic' : page.authScheme === 'bearer' ? 'bearer' : 'cookie'})),
    );
    content.scrollTop = 0;
}

function show(p) {
    current = p;
    showSource = false;
    if (p.url) address.value = p.url;
    document.title = p.title ? `${p.title} - Markdown Browser` : 'Markdown Browser';
    $('back').disabled = !p.canBack;
    $('forward').disabled = !p.canForward;
    $('reload').disabled = !p.url;
    render();
    updateCredentialIndicator();
}

async function go(action) {
    const mine = ++seq;
    document.body.classList.add('loading');
    try {
        const p = await action();
        if (mine === seq) show(p);
    } catch (e) {
        if (mine === seq) show({kind: 'error', error: String(e), url: current?.url ?? ''});
    } finally {
        if (mine === seq) document.body.classList.remove('loading');
    }
}

function toggleSource() {
    if (sourceBtn.disabled) return;
    showSource = !showSource;
    render();
}

address.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') go(() => Navigate(address.value));
});
address.addEventListener('focus', () => address.select());
$('back').addEventListener('click', () => go(Back));
$('forward').addEventListener('click', () => go(Forward));
$('reload').addEventListener('click', () => go(Reload));
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
    } else {
        go(() => Navigate(a.href));
    }
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
        (url) => { closeHistory(); go(() => Navigate(url)); },
        async () => { await ClearHistory(); closeHistory(); },
        () => { closeHistory(); go(FullHistory); },
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
    else if (mod && key === 'r') { if (current?.url) go(Reload); }
    else if (mod && key === 'u') toggleSource();
    else if (mod && key === 'j') toggleConsole();
    else if (e.altKey && e.key === 'ArrowLeft' || mod && e.key === '[') { if (!$('back').disabled) go(Back); }
    else if (e.altKey && e.key === 'ArrowRight' || mod && e.key === ']') { if (!$('forward').disabled) go(Forward); }
    else handled = false;
    if (handled) e.preventDefault();
});

// Drop a file anywhere on the window to open it.
OnFileDrop((x, y, paths) => {
    if (paths?.length) go(() => Navigate(paths[0]));
}, true);

// Site credentials: after saving for the page's own site, load it again.
function originOf(url) {
    try { return new URL(url).origin; } catch { return ''; }
}
// The key button lights up when the current site has a saved credential.
async function updateCredentialIndicator() {
    const origin = originOf(current?.url ?? '');
    const saved = origin && (await credentialOrigins()).has(origin);
    $('creds').classList.toggle('active', Boolean(saved));
    $('creds').title = saved ? `Credentials saved for ${origin}` : 'Site credentials';
}

initCredentials((origin) => {
    updateCredentialIndicator();
    if (current?.url && originOf(current.url) === originOf(origin)) go(Reload);
});
$('creds').addEventListener('click', () => openCredentials({origin: originOf(current?.url ?? '').replace(/^null$/, '')}));

initConsole();
render();
address.focus();
