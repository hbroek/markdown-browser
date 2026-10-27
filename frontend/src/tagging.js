// "Tag with AI": the AI proposes a title, description and keywords for many bookmarks at once.
// Nothing is saved until the proposals have been reviewed in a table and applied.
import {BookmarkFolders, BookmarksUnder, SuggestBookmark, SaveBookmark} from '../wailsjs/go/main/App';
import {folderOptions} from './bookmarks.js';
import {aiReady, describeProvider, openAI, newRequestId, cancelRequest} from './ai.js';

const $ = (id) => document.getElementById(id);
const dialog = $('tag-dialog');
const status = $('tag-status');

const CONCURRENT = 2;     // pages read at the same time
const PAGE_CHARS = 30000; // the most text sent to the model per page (ai.MaxPageRunes)

let onChange = () => {};
let items = [];           // every bookmark under the chosen folder
let stage = 'setup';      // setup, run or review
let results = [];         // one per bookmark that was asked about
let run = null;           // {stopped, inflight: Set of request ids}

function say(text, isError = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
}

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};

const scope = () => document.querySelector('input[name="tag-scope"]:checked')?.value ?? 'missing';
const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
const untitled = (b) => !b.title.trim() || b.title.trim() === b.url.trim();

/** Which bookmarks the chosen scope asks about. */
function candidates() {
    if (scope() === 'all') return items;
    return items.filter(({bookmark: b}) => untitled(b) || !b.description.trim() || !(b.keywords ?? []).length);
}

async function describeSetup() {
    const list = candidates();
    const where = await describeProvider();
    $('tag-summary').textContent = list.length
        ? `${plural(list.length, 'bookmark')} to tag. Up to ${(list.length * PAGE_CHARS).toLocaleString()} characters of page text would be sent to ${where}.`
        : 'No bookmarks need tagging here.';
    $('tag-go').disabled = list.length === 0;
}

async function loadItems() {
    say('');
    try {
        items = (await BookmarksUnder($('tag-folder').value)) ?? [];
    } catch (e) {
        items = [];
        say(String(e), true);
    }
    describeSetup();
}

function show(next) {
    stage = next;
    $('tag-setup').hidden = next !== 'setup';
    $('tag-run').hidden = next !== 'run';
    $('tag-review').hidden = next !== 'review';
    $('tag-go').hidden = next === 'run';
    $('tag-go').textContent = next === 'review' ? 'Apply' : 'Start';
    $('tag-cancel').textContent = next === 'run' ? 'Stop' : next === 'review' ? 'Discard' : 'Cancel';
    $('tag-count').textContent = '';
    dialog.classList.toggle('review', next === 'review');
}

/** Opens the dialog for one folder, or for all bookmarks when folderId is empty. */
export async function openTagging({folderId = ''} = {}) {
    if (!(await aiReady())) return openAI();
    say('');
    show('setup');
    results = [];
    try {
        const folders = (await BookmarkFolders()) ?? [];
        $('tag-folder').replaceChildren(...folderOptions(folders).map((o) => Object.assign(document.createElement('option'),
            {value: o.id, textContent: o.id ? o.label : 'All bookmarks'})));
        $('tag-folder').value = folders.some((f) => f.id === folderId) ? folderId : '';
    } catch (e) {
        say(String(e), true);
    }
    if (!dialog.open) dialog.showModal();
    loadItems();
}

// --- running ---

/** A proposal that keeps what the user wrote unless the options ask for the AI's text. */
function proposal(item, s, all, replaceTitles) {
    const b = item.bookmark;
    return {
        title: (replaceTitles || untitled(b)) && s.title ? s.title : b.title,
        description: (all || !b.description.trim()) && s.description ? s.description : b.description,
        keywords: all || !(b.keywords ?? []).length ? (s.keywords ?? []) : b.keywords,
    };
}

async function start() {
    const list = candidates();
    const all = scope() === 'all';
    const replaceTitles = $('tag-titles').checked;
    results = [];
    run = {stopped: false, inflight: new Set(), limited: false};
    show('run');
    const progress = $('tag-progress');
    progress.max = list.length;
    progress.value = 0;
    let next = 0;

    const worker = async () => {
        while (!run.stopped && next < list.length) {
            const item = list[next++];
            const id = newRequestId();
            run.inflight.add(id);
            $('tag-current').textContent = `${progress.value} of ${list.length} \u2013 ${item.bookmark.title || item.bookmark.url}`;
            try {
                const s = await SuggestBookmark(id, item.bookmark.url);
                if (!run.stopped) results.push({item, error: '', ...proposal(item, s, all, replaceTitles)});
            } catch (e) {
                const message = String(e);
                if (run.stopped) break;
                results.push({item, error: message});
                if (/rate or quota limit/.test(message)) { // more requests would only fail
                    run.limited = true;
                    run.stopped = true;
                }
            } finally {
                run.inflight.delete(id);
                progress.value = results.length;
            }
        }
    };
    await Promise.all(Array.from({length: Math.min(CONCURRENT, list.length)}, worker));
    review(list.length);
}

function stop() {
    if (!run) return;
    run.stopped = true;
    for (const id of run.inflight) cancelRequest(id);
}

// --- review ---

const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const keywordText = (list) => (list ?? []).join(', ');

function review(asked) {
    const ok = results.filter((r) => !r.error);
    const failed = results.filter((r) => r.error);
    const notes = [`${plural(ok.length, 'suggestion')}`];
    if (failed.length) notes.push(`${failed.length} could not be read (listed at the end)`);
    if (run.stopped && results.length < asked) notes.push(run.limited ? 'stopped: the provider\'s rate or quota limit was reached' : `stopped after ${results.length} of ${asked}`);
    $('tag-review-note').textContent = `${notes.join('; ')}. Edit anything, untick what you do not want, then Apply.`;
    show('review');

    const table = $('tag-table');
    const head = el('thead', {}, el('tr', {},
        el('th', {}, el('input', {type: 'checkbox', id: 'tag-all', checked: true, title: 'Select all', onchange: (e) => {
            for (const box of table.querySelectorAll('.pick')) box.checked = e.target.checked;
            countPicked();
        }})),
        el('th', {textContent: 'Title'}), el('th', {textContent: 'Description'}), el('th', {textContent: 'Keywords'})));
    const rows = ok.map((r) => {
        const b = r.item.bookmark;
        const changed = r.title !== b.title || r.description !== b.description || !same(r.keywords, b.keywords ?? []);
        const row = el('tr', {},
            el('td', {}, el('input', {type: 'checkbox', className: 'pick', checked: changed, onchange: countPicked})),
            el('td', {}, el('input', {type: 'text', className: 'f-title', value: r.title}),
                el('div', {className: 'note tag-url', textContent: r.item.path ? `${r.item.path} \u00B7 ${b.url}` : b.url, title: b.url})),
            el('td', {}, el('textarea', {className: 'f-desc', rows: 3, value: r.description})),
            el('td', {}, el('input', {type: 'text', className: 'f-kw', value: keywordText(r.keywords)})));
        row.result = r;
        return row;
    });
    const skipped = failed.map((r) => el('tr', {className: 'tag-failed'},
        el('td', {}),
        el('td', {colSpan: 3}, el('strong', {textContent: r.item.bookmark.title || r.item.bookmark.url}), ` \u2013 ${r.error}`)));
    table.replaceChildren(head, el('tbody', {}, ...rows, ...skipped));
    $('tag-go').disabled = !rows.length;
    countPicked();
}

const picked = () => [...$('tag-table').querySelectorAll('tbody tr')].filter((row) => row.result && row.querySelector('.pick').checked);

function countPicked() {
    const n = picked().length;
    $('tag-count').textContent = `${n} selected`;
    $('tag-go').textContent = `Apply ${n}`;
    $('tag-go').disabled = n === 0;
}

async function apply() {
    const rows = picked();
    $('tag-go').disabled = true;
    $('tag-cancel').disabled = true;
    let saved = 0;
    const failures = [];
    for (const row of rows) {
        say(`Saving ${saved + failures.length + 1} of ${rows.length}\u2026`);
        const b = row.result.item.bookmark;
        try {
            await SaveBookmark({
                ...b,
                title: row.querySelector('.f-title').value,
                description: row.querySelector('.f-desc').value,
                keywords: row.querySelector('.f-kw').value.split(','),
            });
            saved++;
        } catch (e) {
            failures.push(`${b.title || b.url}: ${e}`);
        }
    }
    $('tag-cancel').disabled = false;
    onChange();
    if (failures.length) {
        say(`Saved ${saved}; ${failures.length} failed. ${failures[0]}`, true);
        $('tag-go').disabled = false;
    } else {
        dialog.close();
    }
}

export function initTagging(changed) {
    onChange = changed;
    $('bookmarks-tag').addEventListener('click', () => openTagging());
    $('tag-folder').addEventListener('change', loadItems);
    for (const input of document.querySelectorAll('input[name="tag-scope"]')) input.addEventListener('change', describeSetup);
    $('tag-cancel').addEventListener('click', () => {
        if (stage === 'run') return stop(); // goes on to the review of what was read so far
        dialog.close();
    });
    $('tag-go').addEventListener('click', () => (stage === 'review' ? apply() : start()));
    dialog.addEventListener('cancel', (e) => { // Esc
        if (stage === 'run') { e.preventDefault(); stop(); }
    });
    dialog.addEventListener('close', () => { if (run) stop(); });
}
