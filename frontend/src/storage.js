// The bookmark storage dialog: keep bookmarks on this computer, or in a folder of a LinkaGoGo account.
import {BookmarkSettings, LinkaGoGoFolders, UseLinkaGoGo, UseLocalBookmarks} from '../wailsjs/go/main/App';
import {folderOptions} from './bookmarks.js';

const $ = (id) => document.getElementById(id);
const dialog = $('storage-dialog');
const status = $('storage-status');
const rootSelect = $('storage-root');

let current = {backend: 'sqlite', root: '', connected: false};
let onChange = () => {};
let loaded = false; // the folder list was fetched, so a folder can be chosen

function say(text, isError = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
}

const chosenBackend = () => document.querySelector('input[name="storage-backend"]:checked')?.value ?? 'sqlite';

function showSection() {
    $('storage-lgg').hidden = chosenBackend() !== 'linkagogo';
}

function showNewName() {
    $('storage-new-wrap').hidden = rootSelect.value !== 'new';
}

/** Fills the folder choice: a new folder, the whole account, or any existing folder. */
function fillRoots(folders, wanted) {
    const options = [
        {value: 'new', label: 'Create a new folder\u2026'},
        {value: 'all', label: 'The whole account'},
        ...folderOptions(folders).slice(1).map((o) => ({value: `f:${o.id}`, label: o.label})),
    ];
    rootSelect.replaceChildren(...options.map((o) => {
        const opt = document.createElement('option');
        opt.value = o.value;
        opt.textContent = o.label;
        return opt;
    }));
    rootSelect.disabled = false;
    rootSelect.value = options.some((o) => o.value === wanted) ? wanted : 'new';
    loaded = true;
    showNewName();
}

async function loadFolders() {
    say('Loading folders\u2026');
    try {
        const folders = (await LinkaGoGoFolders($('storage-key').value)) ?? [];
        const wanted = current.backend !== 'linkagogo' ? 'new' : current.root ? `f:${current.root}` : 'all';
        fillRoots(folders, wanted);
        say(`${folders.length} folder${folders.length === 1 ? '' : 's'} found.`);
    } catch (e) {
        loaded = false;
        say(String(e), true);
    }
}

export async function openStorage() {
    say('');
    loaded = false;
    rootSelect.replaceChildren();
    rootSelect.disabled = true;
    $('storage-key').value = '';
    try { current = await BookmarkSettings(); } catch (e) { say(String(e), true); }
    for (const r of document.querySelectorAll('input[name="storage-backend"]')) r.checked = r.value === current.backend;
    $('storage-connected').hidden = !current.connected;
    $('storage-key').placeholder = current.connected ? 'Leave empty to keep the saved key' : 'lgg_\u2026';
    showSection();
    if (current.error) say(current.error, true);
    if (!dialog.open) dialog.showModal();
    if (chosenBackend() === 'linkagogo' && current.connected) loadFolders();
}

export function initStorage(changed) {
    onChange = changed;
    $('storage-open').addEventListener('click', openStorage);
    $('storage-cancel').addEventListener('click', () => dialog.close());
    dialog.addEventListener('close', () => { $('storage-key').value = ''; });
    for (const r of document.querySelectorAll('input[name="storage-backend"]')) {
        r.addEventListener('change', () => {
            showSection();
            if (chosenBackend() === 'linkagogo' && current.connected && !loaded) loadFolders();
        });
    }
    $('storage-load').addEventListener('click', loadFolders);
    rootSelect.addEventListener('change', showNewName);

    $('storage-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        try {
            if (chosenBackend() === 'sqlite') {
                await UseLocalBookmarks();
            } else {
                if (!loaded) { say('Load your folders first, then choose one.', true); return; }
                const pick = rootSelect.value;
                const root = pick.startsWith('f:') ? pick.slice(2) : '';
                const name = pick === 'new' ? $('storage-new').value : '';
                if (pick === 'new' && !name.trim()) { say('Give the new folder a name.', true); return; }
                say('Checking\u2026');
                await UseLinkaGoGo($('storage-key').value, root, name);
            }
            dialog.close();
            onChange();
        } catch (err) {
            say(String(err), true);
        }
    });
}
