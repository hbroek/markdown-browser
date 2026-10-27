// "Sort unfiled": the AI proposes a folder for each bookmark that is in no folder.
// Nothing is moved until the proposals have been reviewed in a table and applied.
import {BookmarkFolders, UnfiledBookmarks, SuggestFolders, ApplyFiling} from '../wailsjs/go/main/App';
import {aiReady, describeProvider, openAI, newRequestId, cancelRequest} from './ai.js';

const $ = (id) => document.getElementById(id);
const dialog = $('org-dialog');
const status = $('org-status');

const BATCH = 50;      // bookmarks filed per request (ai.MaxOrganizeBatch)
const SEP = ' / ';     // joins the names in a folder path (ai.FolderSep)

let onChange = () => {};
let unfiled = [];      // bookmarks in no folder
let known = new Set(); // lower-case paths of the folders that exist
let stage = 'setup';   // setup, run or review
let results = [];      // {bookmark, folder}, one per bookmark that was asked about
let run = null;        // {stopped, inflight: Set of request ids}

function say(text, isError = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
}

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};
const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** Folder paths such as "Reading / Rust", from the flat folder list. */
function pathsOf(folders) {
    const byId = new Map(folders.map((f) => [f.id, f]));
    return folders.map((f) => {
        const names = [f.name];
        for (let p = byId.get(f.parentId), n = 0; p && n < 20; p = byId.get(p.parentId), n++) names.unshift(p.name);
        return names.join(SEP);
    }).sort((a, b) => a.localeCompare(b));
}

function show(next) {
    stage = next;
    $('org-setup').hidden = next !== 'setup';
    $('org-run').hidden = next !== 'run';
    $('org-review').hidden = next !== 'review';
    $('org-go').hidden = next === 'run';
    $('org-go').textContent = next === 'review' ? 'Apply' : 'Start';
    $('org-cancel').textContent = next === 'run' ? 'Stop' : next === 'review' ? 'Discard' : 'Cancel';
    $('org-count').textContent = '';
}

export async function openOrganize() {
    if (!(await aiReady())) return openAI();
    say('');
    show('setup');
    results = [];
    $('org-go').disabled = true;
    $('org-summary').textContent = 'Looking for unfiled bookmarks…';
    if (!dialog.open) dialog.showModal();
    try {
        const folders = (await BookmarkFolders()) ?? [];
        known = new Set(pathsOf(folders).map((p) => p.toLowerCase()));
        $('org-folders').replaceChildren(...pathsOf(folders).map((p) => el('option', {value: p})));
        unfiled = (await UnfiledBookmarks()) ?? [];
    } catch (e) {
        unfiled = [];
        say(String(e), true);
    }
    const where = await describeProvider();
    $('org-summary').textContent = unfiled.length
        ? `${plural(unfiled.length, 'bookmark')} in no folder. Their titles, addresses, keywords and descriptions would be sent to ${where}, in ${plural(Math.ceil(unfiled.length / BATCH), 'request')}.`
        : 'Every bookmark is already in a folder.';
    $('org-go').disabled = unfiled.length === 0;
}

// --- running ---

async function start() {
    results = [];
    run = {stopped: false, inflight: new Set(), failure: ''};
    show('run');
    const progress = $('org-progress');
    progress.max = unfiled.length;
    progress.value = 0;
    const proposed = new Set(); // new folders asked for so far, so later batches reuse them
    for (let from = 0; from < unfiled.length && !run.stopped; from += BATCH) {
        const batch = unfiled.slice(from, from + BATCH);
        $('org-current').textContent = `${from} of ${unfiled.length} bookmarks filed`;
        const id = newRequestId();
        run.inflight.add(id);
        try {
            const choices = (await SuggestFolders(id, batch.map((b) => ({
                id: b.id, title: b.title, url: b.url, description: b.description, keywords: b.keywords ?? [],
            })), [...proposed])) ?? [];
            if (run.stopped) break;
            const byId = new Map(choices.map((c) => [c.id, c.folder]));
            for (const b of batch) {
                const folder = byId.get(b.id) ?? '';
                results.push({bookmark: b, folder});
                if (folder && !known.has(folder.toLowerCase())) proposed.add(folder);
            }
            progress.value = results.length;
        } catch (e) {
            if (!run.stopped) run.failure = String(e);
            run.stopped = true;
        } finally {
            run.inflight.delete(id);
        }
    }
    review();
}

function stop() {
    if (!run) return;
    run.stopped = true;
    for (const id of run.inflight) cancelRequest(id);
}

// --- review ---

function review() {
    const placed = results.filter((r) => r.folder);
    const created = new Set(placed.map((r) => r.folder.toLowerCase()).filter((p) => !known.has(p)));
    const notes = [`${placed.length} of ${results.length} bookmarks placed`];
    if (created.size) notes.push(`${plural(created.size, 'new folder')} proposed`);
    if (run.failure) notes.push(`stopped: ${run.failure}`);
    else if (run.stopped && results.length < unfiled.length) notes.push(`stopped after ${results.length} of ${unfiled.length}`);
    $('org-review-note').textContent = `${notes.join('; ')}. Change a folder (type a new path to create one), untick what you do not want, then Apply.`;
    show('review');

    const table = $('org-table');
    const head = el('thead', {}, el('tr', {},
        el('th', {}, el('input', {type: 'checkbox', checked: true, title: 'Select all', onchange: (e) => {
            for (const box of table.querySelectorAll('.pick')) box.checked = e.target.checked;
            countPicked();
        }})),
        el('th', {textContent: 'Bookmark'}), el('th', {textContent: 'Move to folder'})));
    const rows = results.map((r) => {
        const b = r.bookmark;
        const row = el('tr', {className: r.folder ? '' : 'org-nofit'},
            el('td', {}, el('input', {type: 'checkbox', className: 'pick', checked: !!r.folder, onchange: countPicked})),
            el('td', {}, el('div', {textContent: b.title || b.url}), el('div', {className: 'note tag-url', textContent: b.url, title: b.url})),
            el('td', {}, el('input', {type: 'text', className: 'f-folder', value: r.folder, placeholder: 'no suggestion',
                oninput: () => mark(row)})));
        row.querySelector('.f-folder').setAttribute('list', 'org-folders');
        row.bookmark = b;
        mark(row);
        return row;
    });
    table.replaceChildren(head, el('tbody', {}, ...rows));
    countPicked();
}

/** Tags a row whose folder does not exist yet. */
function mark(row) {
    const input = row.querySelector('.f-folder');
    const path = input.value.trim();
    const fresh = path !== '' && !known.has(path.toLowerCase().replace(/\s*[/>]\s*/g, SEP));
    input.title = fresh ? 'A new folder will be created' : '';
    input.classList.toggle('new-folder', fresh);
    if (path && !row.querySelector('.pick').checked && !row.wasEdited) {
        row.wasEdited = true;
        row.querySelector('.pick').checked = true;
        countPicked();
    }
}

const picked = () => [...$('org-table').querySelectorAll('tbody tr')]
    .filter((row) => row.querySelector('.pick').checked && row.querySelector('.f-folder').value.trim());

function countPicked() {
    const n = picked().length;
    $('org-count').textContent = `${n} selected`;
    $('org-go').textContent = `Move ${n}`;
    $('org-go').disabled = n === 0;
}

async function apply() {
    const moves = picked().map((row) => ({id: row.bookmark.id, folder: row.querySelector('.f-folder').value}));
    $('org-go').disabled = true;
    $('org-cancel').disabled = true;
    say(`Moving ${plural(moves.length, 'bookmark')}…`);
    try {
        const res = await ApplyFiling(moves);
        onChange();
        if (res.failed?.length) {
            say(`Moved ${res.moved}; ${res.failed.length} failed. ${res.failed[0]}`, true);
        } else {
            dialog.close();
        }
    } catch (e) {
        say(String(e), true);
        $('org-go').disabled = false;
    } finally {
        $('org-cancel').disabled = false;
    }
}

export function initOrganize(changed) {
    onChange = changed;
    $('bookmarks-organize').addEventListener('click', openOrganize);
    $('org-cancel').addEventListener('click', () => {
        if (stage === 'run') return stop(); // goes on to the review of what was filed so far
        dialog.close();
    });
    $('org-go').addEventListener('click', () => (stage === 'review' ? apply() : start()));
    dialog.addEventListener('cancel', (e) => { // Esc
        if (stage === 'run') { e.preventDefault(); stop(); }
    });
    dialog.addEventListener('close', () => { if (run) stop(); });
}
