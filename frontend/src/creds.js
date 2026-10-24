// The site credentials dialog. Secrets only go to the backend, never back.
import {Credentials, SaveCredential, DeleteCredential, ImportCookiesTxt, ClearCookies} from '../wailsjs/go/main/App';

const $ = (id) => document.getElementById(id);
const dialog = $('creds-dialog');
const status = $('cred-status');

let onSaved = () => {};

function say(text, isError = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
}

function showKind() {
    const kind = $('cred-kind').value;
    for (const group of dialog.querySelectorAll('[data-kind]')) group.hidden = group.dataset.kind !== kind;
}

async function refreshList() {
    const items = (await Credentials()) ?? [];
    const list = $('cred-list');
    list.replaceChildren(...items.map((c) => {
        const li = document.createElement('li');
        const label = document.createElement('span');
        label.textContent = `${c.origin} · ${c.kind}${c.username ? ` (${c.username})` : ''}`;
        const edit = document.createElement('button');
        edit.type = 'button';
        edit.textContent = 'Replace';
        edit.onclick = () => { $('cred-origin').value = c.origin; $('cred-kind').value = c.kind; showKind(); say(''); };
        const del = document.createElement('button');
        del.type = 'button';
        del.textContent = 'Remove';
        del.onclick = async () => {
            try { await DeleteCredential(c.origin); await refreshList(); } catch (e) { say(String(e), true); }
        };
        li.append(label, edit, del);
        return li;
    }));
}

function clearSecrets() {
    for (const id of ['cred-cookie', 'cred-token', 'cred-user', 'cred-pass', 'cred-cookiefile']) $(id).value = '';
}

/** Opens the dialog. `prefill` may carry {origin, kind}. */
export async function openCredentials(prefill = {}) {
    clearSecrets();
    say('');
    $('cred-origin').value = prefill.origin ?? '';
    $('cred-kind').value = prefill.kind ?? 'cookie';
    showKind();
    await refreshList().catch((e) => say(String(e), true));
    if (!dialog.open) dialog.showModal();
    $('cred-origin').value ? $('cred-cookie').focus() : $('cred-origin').focus();
}

export function initCredentials(savedCallback) {
    onSaved = savedCallback;
    $('cred-kind').addEventListener('change', showKind);
    $('cred-close').addEventListener('click', () => dialog.close());
    dialog.addEventListener('close', clearSecrets);

    $('cred-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const cred = {
            origin: $('cred-origin').value,
            kind: $('cred-kind').value,
            cookie: $('cred-cookie').value.trim(),
            token: $('cred-token').value.trim(),
            username: $('cred-user').value,
            password: $('cred-pass').value,
        };
        try {
            const origin = await SaveCredential(cred);
            clearSecrets();
            await refreshList();
            say(`Saved for ${origin}`);
            dialog.close();
            onSaved(origin);
        } catch (err) {
            say(String(err), true);
        }
    });

    $('cred-cookiefile').addEventListener('change', async (e) => {
        const file = e.target.files[0];
        if (!file) return;
        try {
            const n = await ImportCookiesTxt($('cred-origin').value, await file.text());
            const origin = $('cred-origin').value;
            clearSecrets();
            await refreshList();
            say(`Imported ${n} cookie${n === 1 ? '' : 's'}`);
            dialog.close();
            onSaved(origin);
        } catch (err) {
            say(String(err), true);
            e.target.value = '';
        }
    });

    $('cred-clear-cookies').addEventListener('click', async () => {
        await ClearCookies();
        say('Saved cookies cleared');
    });
}

/** Origins that have a saved credential, as URL.origin strings. */
export async function credentialOrigins() {
    try {
        return new Set(((await Credentials()) ?? []).map((c) => c.origin));
    } catch {
        return new Set();
    }
}
