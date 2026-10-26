// The bookmarks side panel: folders with their bookmarks, a pencil on each
// row to edit it. Folder contents are fetched only while a folder is open.
import {BookmarkFolders, BookmarksIn} from '../wailsjs/go/main/App';
import {openBookmark, openFolder} from './bookmarks.js';

const $ = (id) => document.getElementById(id);
const panel = $('bookmarks-panel');
const tree = $('bookmarks-tree');
const toggle = $('bookmarks-toggle');

let visible = false;
let open = new Set();     // ids of expanded folders
let onOpenUrl = () => {};

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
    return el('button', {type: 'button', className: 'pencil', title: label, onclick: (e) => { e.stopPropagation(); onClick(); }}, el('span', {textContent: '\u270E'}));
}

function indent(row, depth) {
    row.style.paddingLeft = `${8 + depth * 16}px`;
    return row;
}

function folderRow(f, depth) {
    const isOpen = open.has(f.id);
    const label = el('button', {
        type: 'button',
        className: 'tree-label',
        title: f.name,
        onclick: () => {
            isOpen ? open.delete(f.id) : open.add(f.id);
            stored('bookmarks.open', JSON.stringify([...open]));
            refreshPanel();
        },
    }, el('span', {className: 'caret', textContent: isOpen ? '\u25BE' : '\u25B8'}), el('span', {textContent: `\uD83D\uDCC1 ${f.name}`}));
    label.setAttribute('aria-expanded', String(isOpen));
    const row = el('div', {className: 'tree-row folder'}, label, pencil(`Edit folder ${f.name}`, () => openFolder({folder: f})));
    row.setAttribute('role', 'treeitem');
    return indent(row, depth);
}

function bookmarkRow(b, depth) {
    const label = el('button', {
        type: 'button',
        className: 'tree-label plain',
        title: b.description ? `${b.url}\n${b.description}` : b.url,
        textContent: b.title,
        onclick: () => onOpenUrl(b.url),
    });
    const row = el('div', {className: 'tree-row'}, label, pencil(`Edit bookmark ${b.title}`, () => openBookmark({bookmark: b})));
    row.setAttribute('role', 'treeitem');
    return indent(row, depth + 1); // lines up under the folder name, past the caret
}

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
                    if (!inner.length) nodes.push(indent(el('div', {className: 'tree-empty', textContent: 'Empty'}), depth + 2));
                    nodes.push(...inner);
                }
            }
            for (const b of marks.get(parent) ?? []) nodes.push(bookmarkRow(b, depth));
            return nodes;
        };
        const rows = level('', 0);
        tree.replaceChildren(...(rows.length ? rows : [el('p', {className: 'note tree-note', textContent: 'No bookmarks yet. Use the ribbon next to the address bar to save a page.'})]));
    } catch (e) {
        tree.replaceChildren(el('p', {className: 'error-text tree-note', textContent: String(e)}));
    }
}

export function togglePanel(show = !visible) {
    visible = show;
    panel.hidden = !visible;
    toggle.setAttribute('aria-pressed', String(visible));
    toggle.title = visible ? 'Hide bookmarks (Ctrl/Cmd+B)' : 'Show bookmarks (Ctrl/Cmd+B)';
    stored('bookmarks.visible', visible ? '1' : '0');
    refreshPanel();
}

/** `openUrl(url)` is called when a bookmark is clicked. */
export function initPanel(openUrl) {
    onOpenUrl = openUrl;
    try { open = new Set(JSON.parse(stored('bookmarks.open') ?? '[]')); } catch { open = new Set(); }
    toggle.addEventListener('click', () => togglePanel());
    $('bookmarks-close').addEventListener('click', () => togglePanel(false));
    $('folder-new').addEventListener('click', () => openFolder());
    if (stored('bookmarks.visible') === '1') togglePanel(true);
}
