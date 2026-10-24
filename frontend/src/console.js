// The network console: a status bar that expands into a request list with
// request and response details. Hidden until the user turns it on.
import {NetworkLog, ClearNetworkLog} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';

const MAX_ENTRIES = 200;
const $ = (id) => document.getElementById(id);

let entries = [];
let selectedId = null;
let visible = false;
let expanded = false;

function stored(key, value) {
    try {
        if (value === undefined) return localStorage.getItem(key);
        localStorage.setItem(key, value);
    } catch { /* storage unavailable: the console just starts hidden */ }
    return null;
}

function el(tag, props = {}, ...children) {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
}

function statusClass(e) {
    if (e.status === 0 || e.status >= 400) return 'bad';
    if (e.status >= 300) return 'warn';
    return 'ok';
}

function statusLabel(e) {
    return e.status === 0 ? 'ERR' : String(e.status);
}

function formatBytes(n) {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function badge(e) {
    return el('span', {className: `badge ${statusClass(e)}`, textContent: statusLabel(e)});
}

function renderBar() {
    const last = entries[entries.length - 1];
    $('console-count').textContent = `${entries.length} request${entries.length === 1 ? '' : 's'}`;
    $('console-last').replaceChildren(...(last ? [
        badge(last),
        el('span', {className: 'net-method', textContent: last.method}),
        el('span', {className: 'net-url', textContent: last.url}),
        el('span', {className: 'net-meta', textContent: `${last.durationMs} ms`}),
    ] : []));
}

function renderList() {
    const list = $('console-list');
    const atBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 24;
    list.replaceChildren(...entries.map((e) => el('button', {
        type: 'button',
        className: 'net-row' + (e.id === selectedId ? ' selected' : ''),
        title: e.url,
        onclick: () => { selectedId = e.id; renderList(); renderDetail(); },
    },
        badge(e),
        el('span', {className: 'net-method', textContent: e.method}),
        el('span', {className: 'net-url', textContent: e.url}),
        el('span', {className: 'net-meta', textContent: `${e.durationMs} ms · ${formatBytes(e.bytes)}`}),
    )));
    if (atBottom) list.scrollTop = list.scrollHeight;
}

function headerTable(title, headers) {
    return el('section', {},
        el('h3', {textContent: `${title} (${headers?.length ?? 0})`}),
        el('table', {}, ...(headers ?? []).map((h) =>
            el('tr', {}, el('th', {textContent: h.name}), el('td', {textContent: h.value})))),
    );
}

function renderDetail() {
    const box = $('console-detail');
    const e = entries.find((x) => x.id === selectedId);
    if (!e) {
        box.replaceChildren(el('p', {className: 'note', textContent: 'Select a request to see its details.'}));
        return;
    }
    const general = [
        ['URL', e.url],
        ['Method', e.method],
        ['Status', e.status === 0 ? 'request failed' : e.statusText],
        ['Protocol', e.proto],
        ['Content-Type', e.contentType],
        ['Size', formatBytes(e.bytes)],
        ['Duration', `${e.durationMs} ms`],
        ['Time', new Date(e.time).toLocaleTimeString()],
    ].filter(([, v]) => v);
    box.replaceChildren(
        el('section', {},
            el('h3', {textContent: 'General'}),
            el('table', {}, ...general.map(([k, v]) => el('tr', {}, el('th', {textContent: k}), el('td', {textContent: v})))),
        ),
        e.error ? el('p', {className: 'net-error', textContent: e.error}) : '',
        headerTable('Request headers', e.reqHeaders),
        e.status ? headerTable('Response headers', e.respHeaders) : '',
    );
    box.scrollTop = 0;
}

function addEntry(e) {
    const i = entries.findIndex((x) => x.id === e.id);
    if (i >= 0) entries[i] = e;
    else entries.push(e);
    entries.sort((a, b) => a.id - b.id);
    if (entries.length > MAX_ENTRIES) entries = entries.slice(-MAX_ENTRIES);
    renderBar();
    if (expanded) {
        renderList();
        if (e.id === selectedId) renderDetail();
    }
}

function apply() {
    $('console').hidden = !visible;
    $('console-panel').hidden = !(visible && expanded);
    $('console-bar').setAttribute('aria-expanded', String(visible && expanded));
    $('console-toggle').setAttribute('aria-pressed', String(visible));
    $('console-toggle').title = visible ? 'Hide console (Ctrl/Cmd+J)' : 'Show console (Ctrl/Cmd+J)';
    if (visible && expanded) {
        if (selectedId === null && entries.length) selectedId = entries[entries.length - 1].id;
        renderList();
        renderDetail();
    }
}

export function toggleConsole() {
    visible = !visible;
    if (!visible) expanded = false;
    stored('console.visible', visible ? '1' : '0');
    apply();
}

export async function initConsole() {
    visible = stored('console.visible') === '1'; // hidden unless the user turned it on
    $('console-toggle').addEventListener('click', toggleConsole);
    $('console-close').addEventListener('click', toggleConsole);
    $('console-bar').addEventListener('click', () => { expanded = !expanded; apply(); });
    $('console-collapse').addEventListener('click', () => { expanded = false; apply(); });
    $('console-clear').addEventListener('click', async () => {
        await ClearNetworkLog();
        entries = [];
        selectedId = null;
        renderBar();
        renderList();
        renderDetail();
    });

    EventsOn('net', addEntry);
    try {
        for (const e of (await NetworkLog()) ?? []) addEntry(e);
    } catch { /* the live events still arrive */ }
    renderBar();
    apply();
}
