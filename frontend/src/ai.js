// The AI assistant dialog: which provider to use, and whether it may be sent page text.
import {AISettings, SaveAI, TestAI} from '../wailsjs/go/main/App';

const $ = (id) => document.getElementById(id);
const dialog = $('ai-dialog');
const status = $('ai-status');

let info = {presets: [], preset: '', baseUrl: '', model: '', configured: false, hasKey: false, consented: false};

function say(text, isError = false, busy = false) {
    status.textContent = text;
    status.classList.toggle('error-text', isError);
    status.classList.toggle('busy', busy);
}

function hostOf(address) {
    try { return new URL(address.trim()).hostname; } catch { return ''; }
}

function isLocal(host) {
    return host === 'localhost' || host.endsWith('.localhost') || /^127\./.test(host) || host === '[::1]' || host === '::1';
}

const preset = () => info.presets.find((p) => p.id === $('ai-preset').value);
const input = () => ({preset: $('ai-preset').value, baseUrl: $('ai-url').value, model: $('ai-model').value, consent: $('ai-consent').checked});

/** Shows what the chosen address needs: a key field, and consent when the server is not on this computer. */
function refresh() {
    const p = preset();
    const host = hostOf($('ai-url').value);
    const remote = host !== '' && !isLocal(host);
    $('ai-note').hidden = !p?.note;
    $('ai-note').textContent = p?.note ?? '';
    $('ai-consent-wrap').hidden = !remote;
    $('ai-consent-text').textContent = remote ? `Allow sending page text to ${host}` : '';
    const sameServer = info.configured && info.hasKey && info.baseUrl.trim() === $('ai-url').value.trim();
    $('ai-key').placeholder = sameServer ? '(saved \u2013 leave empty to keep it)' : (p?.keyRequired ? 'required' : 'optional');
}

$('ai-preset').addEventListener('change', () => {
    const p = preset();
    if (p && p.id !== 'custom') {
        $('ai-url').value = p.baseUrl;
        $('ai-model').value = p.model;
    }
    refresh();
});
$('ai-url').addEventListener('input', refresh);

export async function aiReady() {
    const i = await AISettings();
    return i.configured && i.consented;
}

export async function openAI() {
    say('');
    $('ai-key').value = '';
    try {
        info = await AISettings();
    } catch (e) {
        say(String(e), true);
    }
    $('ai-preset').replaceChildren(...info.presets.map((p) => Object.assign(document.createElement('option'), {value: p.id, textContent: p.name})));
    const first = info.presets[0];
    $('ai-preset').value = info.preset || first?.id || '';
    $('ai-url').value = info.configured ? info.baseUrl : (first?.baseUrl ?? '');
    $('ai-model').value = info.configured ? info.model : (first?.model ?? '');
    $('ai-consent').checked = info.consented && info.configured;
    refresh();
    if (!dialog.open) dialog.showModal();
    $('ai-key').focus();
}

export function initAI() {
    $('ai-cancel').addEventListener('click', () => dialog.close());
    $('ai-test').addEventListener('click', async () => {
        say('Asking the server\u2026', false, true);
        try {
            const reply = await TestAI(input(), $('ai-key').value);
            say(`Connected. The model answered: ${reply}`);
        } catch (e) {
            say(String(e), true);
        }
    });
    $('ai-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        try {
            await SaveAI(input(), $('ai-key').value);
            dialog.close();
        } catch (err) {
            say(String(err), true);
        }
    });
}
