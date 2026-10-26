// The bookmark and folder dialogs: add, edit and delete.
import {BookmarkFolders, SaveBookmark, DeleteBookmark, SaveBookmarkFolder, DeleteBookmarkFolder} from '../wailsjs/go/main/App';

const $ = (id) => document.getElementById(id);

let onChange = () => {};
let folderHint = () => null;

/** `fn()` gives the folder selected in the panel (or null); new items start there. */
export function setFolderHint(fn) { folderHint = fn; }

function say(el, text, isError = false) {
    el.textContent = text;
    el.classList.toggle('error-text', isError);
}

function lastFolder() {
    try { return localStorage.getItem('bookmark.folder') ?? ''; } catch { return ''; }
}

function rememberFolder(id) {
    try { localStorage.setItem('bookmark.folder', id); } catch { /* not persisted */ }
}

/**
 * Folders in tree order as {id, label}, nested ones indented, with the top
 * level first. `exclude` leaves out a folder and everything under it, so a
 * folder cannot be offered as its own parent.
 */
export function folderOptions(folders, exclude = '') {
    const children = new Map();
    for (const f of folders) {
        const siblings = children.get(f.parentId) ?? [];
        siblings.push(f);
        children.set(f.parentId, siblings);
    }
    const out = [{id: '', label: 'Bookmarks'}];
    const walk = (parent, depth) => {
        for (const f of children.get(parent) ?? []) {
            if (f.id === exclude) continue;
            out.push({id: f.id, label: '\u00A0\u00A0'.repeat(depth) + f.name});
            walk(f.id, depth + 1);
        }
    };
    walk('', 1);
    return out;
}

async function fillFolders(select, wanted, exclude = '') {
    const folders = (await BookmarkFolders()) ?? [];
    const options = folderOptions(folders, exclude);
    select.replaceChildren(...options.map((o) => {
        const opt = document.createElement('option');
        opt.value = o.id;
        opt.textContent = o.label;
        return opt;
    }));
    select.value = options.some((o) => o.id === wanted) ? wanted : '';
}

/**
 * Makes a button delete only on a second click within a few seconds, so a
 * stray click cannot remove anything. Returns a function that resets it.
 */
function twoClickDelete(button, run) {
    let timer = 0;
    const reset = () => {
        clearTimeout(timer);
        button.classList.remove('armed');
        button.textContent = 'Delete';
    };
    button.addEventListener('click', () => {
        if (!button.classList.contains('armed')) {
            button.classList.add('armed');
            button.textContent = 'Click again to delete';
            timer = setTimeout(reset, 4000);
            return;
        }
        run();
    });
    return reset;
}

// --- bookmark dialog ---

const bookmarkDialog = $('bookmark-dialog');
const bookmarkStatus = $('bookmark-status');
let editing = null;      // the bookmark being edited, or null when adding

/**
 * Opens the dialog. With `bookmark` it edits that bookmark; otherwise it adds
 * one for `url`, using `title` as the starting title.
 */
export async function openBookmark({bookmark = null, url = '', title = ''}) {
    editing = bookmark;
    resetBookmarkDelete();
    say(bookmarkStatus, '');
    $('bookmark-title').textContent = bookmark ? 'Edit bookmark' : 'Add bookmark';
    $('bookmark-name').value = bookmark?.title ?? title;
    $('bookmark-url').value = bookmark?.url ?? url;
    $('bookmark-desc').value = bookmark?.description ?? '';
    $('bookmark-keywords').value = (bookmark?.keywords ?? []).join(', ');
    $('bookmark-delete').hidden = !bookmark;
    try {
        await fillFolders($('bookmark-folder'), bookmark ? bookmark.folderId : (folderHint() ?? lastFolder()));
    } catch (e) {
        say(bookmarkStatus, String(e), true);
    }
    if (!bookmarkDialog.open) bookmarkDialog.showModal();
    $('bookmark-name').focus();
    $('bookmark-name').select();
}

const resetBookmarkDelete = twoClickDelete($('bookmark-delete'), async () => {
    try {
        await DeleteBookmark(editing.id);
        bookmarkDialog.close();
        onChange();
    } catch (err) {
        say(bookmarkStatus, String(err), true);
    }
});

// --- folder dialog ---

const folderDialog = $('folder-dialog');
const folderStatus = $('folder-status');
let editingFolder = null;

/** Opens the dialog to edit `folder`, or to add one inside `parentId`. */
export async function openFolder({folder = null, parentId = ''} = {}) {
    editingFolder = folder;
    resetFolderDelete();
    say(folderStatus, '');
    $('folder-title').textContent = folder ? 'Edit folder' : 'New folder';
    $('folder-name').value = folder?.name ?? '';
    $('folder-delete').hidden = !folder;
    $('folder-note').hidden = !folder;
    try {
        await fillFolders($('folder-parent'), folder ? folder.parentId : (folderHint() ?? parentId), folder?.id ?? '');
    } catch (e) {
        say(folderStatus, String(e), true);
    }
    if (!folderDialog.open) folderDialog.showModal();
    $('folder-name').focus();
    $('folder-name').select();
}

const resetFolderDelete = twoClickDelete($('folder-delete'), async () => {
    try {
        await DeleteBookmarkFolder(editingFolder.id);
        folderDialog.close();
        onChange();
    } catch (err) {
        say(folderStatus, String(err), true);
    }
});

/** `changed` runs after any bookmark or folder is saved or deleted. */
export function initBookmarks(changed) {
    onChange = changed;

    $('bookmark-cancel').addEventListener('click', () => bookmarkDialog.close());
    bookmarkDialog.addEventListener('close', resetBookmarkDelete);
    $('bookmark-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const folderId = $('bookmark-folder').value;
        try {
            await SaveBookmark({
                id: editing?.id ?? '',
                folderId,
                title: $('bookmark-name').value,
                url: $('bookmark-url').value,
                description: $('bookmark-desc').value,
                keywords: $('bookmark-keywords').value.split(','),
            });
            rememberFolder(folderId);
            bookmarkDialog.close();
            onChange();
        } catch (err) {
            say(bookmarkStatus, String(err), true);
        }
    });

    $('folder-cancel').addEventListener('click', () => folderDialog.close());
    folderDialog.addEventListener('close', resetFolderDelete);
    $('folder-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        try {
            await SaveBookmarkFolder({
                id: editingFolder?.id ?? '',
                parentId: $('folder-parent').value,
                name: $('folder-name').value,
            });
            folderDialog.close();
            onChange();
        } catch (err) {
            say(folderStatus, String(err), true);
        }
    });
}
