// The bookmarks side panel: folders with their bookmarks, a pencil on each
// row to edit it. Folder contents are fetched only while a folder is open.
// The tree is keyboard driven (arrows, Enter, "e" to edit); see onTreeKey.
import {BookmarkFolders, BookmarksIn, ImportBookmarks, ExportBookmarks} from '../wailsjs/go/main/App';
import {openBookmark, openFolder, setFolderHint} from './bookmarks.js';
import {openStorage} from './storage.js';

const $ = (id) => document.getElementById(id);
const panel = $('bookmarks-panel');
const tree = $('bookmarks-tree');
const toggle = $('bookmarks-toggle');
const message = $('bookmarks-msg');

let visible = false;
let open = new Set();     // ids of expanded folders
let currentKey = '';      // the row that is (or was last) focused: "f:<id>" or "b:<id>"
let onOpenUrl = () => {};
let onChanged = () => {};
const actions = new WeakMap(); // row -> what its keys do

function stored(key, value) {
    try {
        if (value === undefined) return localStorage.getItem(key);
        localStorage.setItem(key, value);
    } catch { /* storage unavailable: the panel just starts closed */ }
    return null;
}

function el(tag, props = {}, ...children) {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
}

function pencil(label, onClick) {
    // The glyph points to the lower right; the span is mirrored in CSS so it points lower left.
    // Not a tab stop: from the keyboard, "e" on the row does the same.
    return el('button', {type: 'button', className: 'pencil', tabIndex: -1, title: label, onclick: (e) => { e.stopPropagation(); onClick(); }}, el('span', {textContent: '✎'}));
}

function newRow(key, folder, depth, ...children) {
    const row = el('div', {className: 'tree-row', tabIndex: -1}, ...children);
    row.dataset.key = key;
    row.dataset.folder = folder; // where new items go when this row is selected
    row.setAttribute('role', 'treeitem');
    row.setAttribute('aria-level', String(depth + 1));
    row.style.paddingLeft = `${8 + depth * 16}px`;
    return row;
}

function folderRow(f, depth) {
    const isOpen = open.has(f.id);
    const toggleFolder = () => {
        isOpen ? open.delete(f.id) : open.add(f.id);
        stored('bookmarks.open', JSON.stringify([...open]));
        refreshPanel();
    };
    const edit = () => openFolder({folder: f});
    const label = el('button', {type: 'button', className: 'tree-label', tabIndex: -1, title: f.name, onclick: toggleFolder},
        el('span', {className: 'caret', textContent: isOpen ? '▾' : '▸'}),
        el('span', {textContent: `📁 ${f.name}`}));
    const row = newRow(`f:${f.id}`, f.id, depth, label, pencil(`Edit folder ${f.name}`, edit));
    row.classList.add('folder');
    row.setAttribute('aria-expanded', String(isOpen));
    row.setAttribute('aria-label', f.name);
    actions.set(row, {folder: true, open: isOpen, activate: toggleFolder, toggle: toggleFolder, edit});
    return row;
}

function bookmarkRow(b, depth) {
    const go = () => onOpenUrl(b.url);
    const edit = () => openBookmark({bookmark: b});
    const label = el('button', {
        type: 'button',
        className: 'tree-label plain',
        tabIndex: -1,
        title: b.description ? `${b.url}\n${b.description}` : b.url,
        textContent: b.title,
        onclick: go,
    });
    // One level in, so the title lines up under folder names rather than their carets.
    const row = newRow(`b:${b.id}`, b.folderId, depth + 1, label, pencil(`Edit bookmark ${b.title}`, edit));
    row.setAttribute('aria-label', b.title);
    actions.set(row, {folder: false, activate: go, edit});
    return row;
}

// The folder of the selected row (the folder itself, or a bookmark's folder),
// or null when the panel is closed or nothing was selected.
setFolderHint(() => {
    if (!visible) return null;
    const row = rowsOf().find((r) => r.dataset.key === currentKey);
    return row ? row.dataset.folder : null;
});

const rowsOf = () => [...tree.querySelectorAll('.tree-row')];
const levelOf = (row) => Number(row.getAttribute('aria-level'));

/** Makes `row` the one tab stop of the tree. */
function setCurrent(row) {
    for (const r of tree.querySelectorAll('.tree-row[tabindex="0"]')) r.tabIndex = -1;
    row.tabIndex = 0;
    currentKey = row.dataset.key;
}

function focusRow(row) {
    if (row) row.focus();
}

tree.addEventListener('focusin', (e) => {
    const row = e.target.closest('.tree-row');
    if (row) setCurrent(row);
});

/**
 * Keys on a focused row: Up/Down move, Home/End jump, Right opens a folder (or
 * steps into it), Left closes it (or steps out), Enter opens the bookmark or
 * toggles the folder, "e" or F2 edits, Escape returns to the address bar.
 */
function onTreeKey(e) {
    const row = e.target;
    if (!row.classList?.contains('tree-row') || e.metaKey || e.ctrlKey || e.altKey) return;
    const rows = rowsOf();
    const i = rows.indexOf(row);
    const act = actions.get(row);
    let handled = true;
    switch (e.key) {
        case 'ArrowDown': focusRow(rows[i + 1]); break;
        case 'ArrowUp': focusRow(rows[i - 1]); break;
        case 'Home': focusRow(rows[0]); break;
        case 'End': focusRow(rows[rows.length - 1]); break;
        case 'ArrowRight':
            if (!act.folder) break;
            if (!act.open) act.toggle();
            else if (rows[i + 1] && levelOf(rows[i + 1]) > levelOf(row)) focusRow(rows[i + 1]);
            break;
        case 'ArrowLeft':
            if (act.folder && act.open) { act.toggle(); break; }
            focusRow(rows.slice(0, i).reverse().find((r) => levelOf(r) < levelOf(row)));
            break;
        case 'Enter':
        case ' ':
            act.activate();
            break;
        case 'e':
        case 'F2':
            act.edit();
            break;
        case 'Escape':
            $('address').focus();
            break;
        default:
            handled = false;
    }
    if (handled) e.preventDefault();
}
tree.addEventListener('keydown', onTreeKey);

/** Rebuilds the tree from the backend. A no-op while the panel is hidden. */
export async function refreshPanel() {
    if (!visible) return;
    try {
        const folders = (await BookmarkFolders()) ?? [];
        const children = new Map();
        for (const f of folders) children.set(f.parentId, [...(children.get(f.parentId) ?? []), f]);

        // Fetch bookmarks only for the top level and for folders that are shown open.
        const wanted = [''];
        const walk = (parent) => {
            for (const f of children.get(parent) ?? []) {
                if (open.has(f.id)) { wanted.push(f.id); walk(f.id); }
            }
        };
        walk('');
        const lists = await Promise.all(wanted.map((id) => BookmarksIn(id)));
        const marks = new Map(wanted.map((id, i) => [id, lists[i] ?? []]));

        const level = (parent, depth) => {
            const nodes = [];
            for (const f of children.get(parent) ?? []) {
                nodes.push(folderRow(f, depth));
                if (open.has(f.id)) {
                    const inner = level(f.id, depth + 1);
                    if (!inner.length) {
                        const empty = el('div', {className: 'tree-empty', textContent: 'Empty'});
                        empty.style.paddingLeft = `${8 + (depth + 2) * 16}px`;
                        nodes.push(empty);
                    }
                    nodes.push(...inner);
                }
            }
            for (const b of marks.get(parent) ?? []) nodes.push(bookmarkRow(b, depth));
            return nodes;
        };
        const nodes = level('', 0);
        const hadFocus = tree.contains(document.activeElement);
        tree.replaceChildren(...(nodes.length ? nodes : [el('p', {className: 'note tree-note', textContent: 'No bookmarks yet. Use the ribbon next to the address bar to save a page.'})]));

        const rows = rowsOf();
        const current = rows.find((r) => r.dataset.key === currentKey) ?? rows[0];
        if (current) {
            setCurrent(current);
            if (hadFocus) current.focus(); // a rebuild must not drop the keyboard position
        }
    } catch (e) {
        tree.replaceChildren(
            el('p', {className: 'error-text tree-note', textContent: String(e)}),
            el('p', {className: 'tree-note'}, el('button', {type: 'button', className: 'link-button', textContent: 'Bookmark storage\u2026', onclick: openStorage})),
        );
    }
}

export function togglePanel(show = !visible) {
    visible = show;
    panel.hidden = !visible;
    toggle.setAttribute('aria-pressed', String(visible));
    toggle.title = visible ? 'Hide bookmarks (Ctrl/Cmd+B)' : 'Show bookmarks (Ctrl/Cmd+B)';
    stored('bookmarks.visible', visible ? '1' : '0');
    return refreshPanel();
}

/**
 * Ctrl/Cmd+B: open the panel and move into it; from the page, move into an
 * open panel; from inside it, close it and go back to the address bar.
 */
export async function panelShortcut() {
    if (visible && panel.contains(document.activeElement)) {
        togglePanel(false);
        $('address').focus();
        return;
    }
    if (!visible) await togglePanel(true);
    const rows = rowsOf();
    focusRow(rows.find((r) => r.dataset.key === currentKey) ?? rows[0]);
}

// --- import and export ---

function say(text, isError = false) {
    message.textContent = text;
    message.classList.toggle('error-text', isError);
}

const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

function summary(r) {
    const parts = [`Imported ${plural(r.bookmarks, 'bookmark')} and ${plural(r.folders, 'new folder')}`];
    if (r.duplicates) parts.push(`${r.duplicates} already saved`);
    if (r.skipped) parts.push(`${r.skipped} skipped (not a web address)`);
    return parts.join('; ');
}

/** `openUrl(url)` is called when a bookmark is clicked; `changed()` after an import. */
export function initPanel(openUrl, changed = () => {}) {
    onOpenUrl = openUrl;
    onChanged = changed;
    try { open = new Set(JSON.parse(stored('bookmarks.open') ?? '[]')); } catch { open = new Set(); }
    toggle.addEventListener('click', () => togglePanel());
    $('bookmarks-close').addEventListener('click', () => togglePanel(false));
    $('folder-new').addEventListener('click', () => openFolder());

    $('bookmarks-import').addEventListener('click', async () => {
        say('');
        try {
            const result = await ImportBookmarks();
            if (result.canceled) return;
            say(summary(result));
            await refreshPanel();
            onChanged();
        } catch (e) {
            say(String(e), true);
        }
    });
    const exportAs = (format) => async () => {
        say('');
        try {
            const path = await ExportBookmarks(format);
            if (path) say(`Saved to ${path}`);
        } catch (e) {
            say(String(e), true);
        }
    };
    $('bookmarks-export').addEventListener('click', exportAs('html'));
    $('bookmarks-export-md').addEventListener('click', exportAs('markdown'));

    if (stored('bookmarks.visible') === '1') togglePanel(true);
}
