// The reading tools menu: outline, translate, a digest of a feed, and explaining or
// simplifying the text the reader has selected. Outline, translate and digest show their
// result in the card above the page (see readPage in main.js); explaining a selection goes
// through the Ask panel, so it has the conversation, the cancel button and the saved history.
import {askNow} from './ask.js';

const $ = (id) => document.getElementById(id);

const LANGUAGES = ['Arabic', 'Chinese (Simplified)', 'Chinese (Traditional)', 'Dutch', 'English', 'French', 'German', 'Hindi', 'Italian',
    'Japanese', 'Korean', 'Polish', 'Portuguese', 'Russian', 'Spanish', 'Swedish', 'Turkish', 'Ukrainian'];
const MAX_SELECTION = 3000; // characters quoted in a question (the Ask limit is 4000)

function read(key) {
    try { return localStorage.getItem(key); } catch { return null; }
}

function write(key, value) {
    try { localStorage.setItem(key, value); } catch { /* not persisted */ }
}

/** The language of the reader's system, as an English name like "Dutch"; falls back to English. */
function systemLanguage() {
    try {
        const name = new Intl.DisplayNames(['en'], {type: 'language'}).of((navigator.language || 'en').split('-')[0]);
        return LANGUAGES.find((l) => l.startsWith(name)) ?? 'English';
    } catch {
        return 'English';
    }
}

let language = LANGUAGES.includes(read('translate.lang')) ? read('translate.lang') : systemLanguage();

/** Looks like an RSS or Atom document, going by the start of the source. */
const isFeed = (p) => /<(rss|feed|rdf:RDF)[\s>]/i.test((p?.source ?? '').slice(0, 2000));

function selectedText() {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed) return '';
    const inPage = $('content').contains(sel.anchorNode);
    const text = sel.toString().replace(/\s+/g, ' ').trim();
    if (!inPage || !text) return '';
    const runes = [...text];
    return runes.length > MAX_SELECTION ? `${runes.slice(0, MAX_SELECTION).join('')}…` : text;
}

const row = (label, onClick, {disabled = false, title = ''} = {}) => {
    const b = Object.assign(document.createElement('button'), {type: 'button', className: 'menu-row', textContent: label, disabled, title});
    b.setAttribute('role', 'menuitem');
    b.addEventListener('click', onClick);
    return b;
};

/** `host.page()` gives the page of the active tab; `host.run(tool, option)` runs a tool on it. */
export function initReading(host) {
    const btn = $('reading');
    const menu = $('reading-menu');

    const close = () => {
        menu.hidden = true;
        btn.setAttribute('aria-expanded', 'false');
    };

    function build(selection) {
        const p = host.page();
        const select = Object.assign(document.createElement('select'), {title: 'Language to translate into'});
        select.setAttribute('aria-label', 'Language to translate into');
        for (const l of LANGUAGES) select.append(Object.assign(document.createElement('option'), {value: l, textContent: l, selected: l === language}));
        select.addEventListener('change', () => { language = select.value; write('translate.lang', language); });
        const translate = Object.assign(document.createElement('div'), {className: 'menu-line'});
        translate.append(row('Translate into', () => { close(); host.run('translate', language); }), select);

        const note = selection ? '' : 'Select some text on the page first';
        const heading = (t) => Object.assign(document.createElement('div'), {className: 'menu-heading', textContent: t});
        menu.replaceChildren(
            row('Outline of this page', () => { close(); host.run('outline'); }),
            translate,
            ...(isFeed(p) ? [row('Digest of this feed', () => { close(); host.run('digest'); })] : []),
            heading('Selected text'),
            row('Explain the selection', () => { close(); askNow(`Explain this passage from the page, in plain language and with the context of the page:\n\n"${selection}"`); }, {disabled: !selection, title: note}),
            row('Simplify the selection', () => { close(); askNow(`Rewrite this passage from the page in simpler words, keeping its meaning:\n\n"${selection}"`); }, {disabled: !selection, title: note}),
        );
    }

    function open() {
        build(selectedText());
        menu.hidden = false;
        btn.setAttribute('aria-expanded', 'true');
        menu.querySelector('button:not([disabled])')?.focus();
    }

    // Pressing the button must not clear the selection that the menu is about to use.
    btn.addEventListener('mousedown', (e) => e.preventDefault());
    btn.addEventListener('click', () => (menu.hidden ? open() : close()));
    document.addEventListener('mousedown', (e) => {
        if (!menu.hidden && !e.target.closest('#reading-menu') && !e.target.closest('#reading')) close();
    });
    document.addEventListener('keydown', (e) => {
        if (menu.hidden) return;
        if (e.key === 'Escape') { close(); btn.focus(); return; }
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            const rows = [...menu.querySelectorAll('button:not([disabled])')];
            const i = rows.indexOf(document.activeElement);
            rows[(i + (e.key === 'ArrowDown' ? 1 : -1) + rows.length) % rows.length].focus();
        }
    });
    // Enabled with the summarize button: there has to be a page to read.
    new MutationObserver(() => { btn.disabled = $('summarize').disabled; }).observe($('summarize'), {attributes: true, attributeFilter: ['disabled']});
    btn.disabled = $('summarize').disabled;
}
