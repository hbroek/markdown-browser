// "Compare open tabs": a dialog, opened from the reading tools menu, that asks the AI what
// the chosen pages have in common and where they differ. Choosing the tabs happens here, so
// the tab bar stays as it is.
import {CompareTabs} from '../wailsjs/go/main/App';
import {aiReady, describeProvider, openAI, newRequestId, cancelRequest} from './ai.js';
import {waiting, seconds} from './view.js';

const $ = (id) => document.getElementById(id);
const dialog = $('cmp-dialog');
const status = $('cmp-status');

const MIN = 2;
const MAX = 6; // ai.MaxCompareDocs

let host = {tabs: () => [], open: () => {}};
let stage = 'setup'; // setup, run or result
let requestId = '';

function say(text, isError = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
}

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};

/** Tabs that show a real page, as {title, url}. */
function comparable() {
    return host.tabs().filter((t) => t?.url && !t.url.startsWith('bookmarks:'));
}

const chosen = () => [...$('cmp-tabs').querySelectorAll('input:checked')].map((box) => box.value);

function count() {
    const n = chosen().length;
    $('cmp-go').disabled = n < MIN || n > MAX;
    say(n > MAX ? `Choose at most ${MAX} pages.` : '', n > MAX);
}

function show(next) {
    stage = next;
    $('cmp-setup').hidden = next !== 'setup';
    $('cmp-run').hidden = next !== 'run';
    $('cmp-result').hidden = next !== 'result';
    $('cmp-go').hidden = next === 'run';
    $('cmp-go').textContent = next === 'result' ? 'Compare again' : 'Compare';
    $('cmp-cancel').textContent = next === 'run' ? 'Stop' : 'Close';
}

/**
 * Lists the tabs open now. `keep` is the addresses ticked before, when the list is rebuilt:
 * those stay ticked if their tabs are still open, and tabs opened since start unticked.
 * Without it the first six are ticked.
 */
async function fillSetup(keep = null) {
    const tabs = comparable();
    show('setup');
    $('cmp-tabs').replaceChildren(...tabs.map((t, i) => el('label', {className: 'radio'},
        el('input', {type: 'checkbox', value: t.url, checked: keep ? keep.includes(t.url) : i < MAX, onchange: count}),
        // A tab that has not been shown yet has no title: its address is all there is to show.
        el('span', {}, t.title && t.title !== t.url ? el('span', {textContent: t.title}) : '',
            el('span', {className: t.title && t.title !== t.url ? 'note cmp-url' : 'cmp-url', textContent: t.url, title: t.url})))));
    $('cmp-summary').textContent = tabs.length < MIN
        ? 'Open at least two pages in tabs to compare them.'
        : `The text of the chosen pages (${MIN} to ${MAX}) is sent to ${await describeProvider()}.`;
    count();
    if (tabs.length < MIN) $('cmp-go').disabled = true;
}

export async function openCompare() {
    if (!(await aiReady())) return openAI();
    say('');
    await fillSetup();
    if (!dialog.open) dialog.showModal();
}

async function start() {
    const urls = chosen();
    const id = (requestId = newRequestId());
    show('run');
    $('cmp-run').replaceChildren(waiting(`Reading ${urls.length} pages and comparing them`));
    say('');
    try {
        const r = await CompareTabs(id, urls, $('cmp-focus').value);
        if (requestId !== id) return; // stopped meanwhile
        $('cmp-body').innerHTML = r.html; // sanitized by the backend
        $('cmp-source').textContent = `Written by ${r.source} in ${seconds(r.millis)}; it can be wrong.`;
        show('result');
    } catch (e) {
        if (requestId !== id) return;
        show('setup');
        say(String(e), true);
    } finally {
        if (requestId === id) requestId = '';
    }
}

function stop() {
    const id = requestId;
    requestId = '';
    if (id) cancelRequest(id);
    show('setup');
    count();
}

export function initCompare(hostApi) {
    host = hostApi;
    $('cmp-go').addEventListener('click', () => {
        if (stage === 'result') { say(''); fillSetup(chosen()); } else start();
    });
    $('cmp-cancel').addEventListener('click', () => (stage === 'run' ? stop() : dialog.close()));
    dialog.addEventListener('cancel', (e) => { // Esc
        if (stage === 'run') { e.preventDefault(); stop(); }
    });
    dialog.addEventListener('close', () => { if (requestId) stop(); });
    // Links in the answer open in the browser, not in the dialog.
    $('cmp-body').addEventListener('click', (e) => {
        const a = e.target.closest('a[href]');
        if (!a) return;
        e.preventDefault();
        if (!a.getAttribute('href').startsWith('#')) host.open(a.href, e.metaKey || e.ctrlKey);
    });
}
