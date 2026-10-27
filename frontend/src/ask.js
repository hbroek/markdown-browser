// "Ask this page": a conversation about the page in the active tab, in a panel beside it.
// The conversation belongs to the tab (tab.chat) and is about one page. It is saved
// after every answer and comes back when that page is opened again, even after a restart.
import {AskPage, SavedConversation, SaveConversation} from '../wailsjs/go/main/App';
import {aiReady, openAI, newRequestId, cancelRequest} from './ai.js';
import {waiting, seconds} from './view.js';

const $ = (id) => document.getElementById(id);
const panel = $('ask-panel');
const log = $('ask-log');
const input = $('ask-input');
const toggleBtn = $('ask-toggle');

let host = {tab: () => null, open: () => {}}; // set by initAsk
let shown = false;

function stored(value) {
    try {
        if (value === undefined) return localStorage.getItem('ask.open');
        localStorage.setItem('ask.open', value);
    } catch { /* not persisted */ }
    return null;
}

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};

/** The page the active tab shows, if it is one a question can be asked about. */
function askable(tab) {
    const p = tab?.page;
    return p && p.url && p.kind !== 'error' && !p.url.startsWith('bookmarks:') ? p : null;
}

function message(turn) {
    if (turn.role === 'user') return el('div', {className: 'ask-msg user', textContent: turn.text});
    if (turn.role === 'error') return el('div', {className: 'ask-msg error-text', textContent: turn.text});
    return el('div', {className: 'ask-msg assistant'},
        el('div', {className: 'markdown', innerHTML: turn.html}), // sanitized by the backend
        el('div', {className: 'note', textContent: `${turn.source} \u00B7 ${seconds(turn.millis)}`}));
}

/** Redraws the panel for the active tab. */
export function refreshAsk() {
    panel.hidden = !shown;
    toggleBtn.setAttribute('aria-pressed', String(shown));
    if (!shown) return;
    const tab = host.tab();
    const page = askable(tab);
    const chat = tab?.chat;
    if (page && !chat && tab.chatTried !== page.url) loadSaved(tab, page.url);
    const children = [];
    if (!page) {
        children.push(el('p', {className: 'note', textContent: 'Open a page, then ask questions about it here.'}));
    } else if (!chat || (!chat.turns.length && !chat.pending)) {
        children.push(el('p', {className: 'note', textContent: 'Ask a question about this page. The text of the page and your questions are sent to your AI provider.'}));
    }
    for (const turn of chat?.turns ?? []) children.push(message(turn));
    if (chat?.pending) children.push(waiting('Thinking'));
    log.replaceChildren(...children);
    log.scrollTop = log.scrollHeight;
    const sendBtn = $('ask-send');
    sendBtn.disabled = !page;
    sendBtn.textContent = chat?.pending ? 'Cancel' : 'Ask'; // while the model works, the button stops it
    sendBtn.classList.toggle('primary', !chat?.pending);
    input.disabled = !page;
    $('ask-clear').disabled = !chat?.turns.length || Boolean(chat?.pending);
}

/** Brings back the conversation saved for this page, if the tab has none yet. */
async function loadSaved(tab, url) {
    tab.chatTried = url;
    let turns = [];
    try { turns = (await SavedConversation(url)) ?? []; } catch { /* nothing saved */ }
    if (!turns.length || tab.chat || tab.page?.url !== url) return;
    tab.chat = {url, turns, pending: false, requestId: ''};
    if (host.tab() === tab) refreshAsk();
}

/** Saves the conversation (text only; the answers' markup is made again when loaded). */
function persist(chat) {
    const turns = chat.turns.filter((t) => t.role === 'user' || t.role === 'assistant')
        .map((t) => ({role: t.role, text: t.text, html: '', source: t.source ?? '', millis: t.millis ?? 0}));
    SaveConversation(chat.url, turns).catch(() => {});
}

export function toggleAsk(show = !shown) {
    shown = show;
    stored(shown ? '1' : '0');
    refreshAsk();
    if (shown) input.focus();
}

/** While an answer is being written the button cancels it, and the question goes back into the box. */
function cancelAsk(tab) {
    const chat = tab?.chat;
    if (!chat?.pending) return;
    cancelRequest(chat.requestId);
    const last = chat.turns.pop(); // the question that got no answer
    chat.pending = false;
    chat.requestId = '';
    if (last?.role === 'user' && !input.value.trim()) input.value = last.text;
    refreshAsk();
    input.focus();
}

async function send() {
    const tab = host.tab();
    if (tab?.chat?.pending) return cancelAsk(tab);
    const page = askable(tab);
    const question = input.value.trim();
    if (!page || !question) return;
    if (!(await aiReady())) return openAI();
    const chat = (tab.chat ??= {url: page.url, turns: [], pending: false, requestId: ''});
    if (chat.pending) return;
    // Earlier answers go back as they were written.
    const history = chat.turns.filter((t) => t.role === 'user' || t.role === 'assistant').map((t) => ({role: t.role, content: t.text}));
    const id = newRequestId();
    chat.turns.push({role: 'user', text: question});
    chat.pending = true;
    chat.requestId = id;
    input.value = '';
    refreshAsk();
    try {
        const a = await AskPage(id, chat.url, history, question);
        if (chat.requestId !== id) return; // cancelled meanwhile
        chat.turns.push({role: 'assistant', text: a.text, html: a.html, source: a.source, millis: a.millis});
        persist(chat);
    } catch (e) {
        if (chat.requestId !== id) return;
        chat.turns.push({role: 'error', text: String(e)});
    } finally {
        if (chat.requestId === id) {
            chat.pending = false;
            chat.requestId = '';
            if (host.tab() === tab && tab.chat === chat) refreshAsk();
        }
    }
}

/** Opens the panel and asks a question at once, e.g. to explain a passage the reader selected. */
export function askNow(question) {
    if (!askable(host.tab())) return;
    toggleAsk(true);
    if (host.tab().chat?.pending) { // one answer at a time: leave the question for the reader to send
        input.value = question;
        return;
    }
    input.value = question;
    send();
}

/** `host.tab()` gives the active tab; `host.open(url, newTab)` follows a link from an answer. */
export function initAsk(hostApi) {
    host = hostApi;
    shown = stored() === '1';
    toggleBtn.addEventListener('click', () => toggleAsk());
    $('ask-close').addEventListener('click', () => toggleAsk(false));
    $('ask-clear').addEventListener('click', () => {
        const tab = host.tab();
        if (tab && !tab.chat?.pending) {
            if (tab.page?.url) SaveConversation(tab.page.url, []).catch(() => {}); // deletes the saved one too
            tab.chat = null;
            tab.chatTried = tab.page?.url ?? '';
        }
        refreshAsk();
        input.focus();
    });
    $('ask-form').addEventListener('submit', (e) => { e.preventDefault(); send(); });
    input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
        if (e.key === 'Escape') cancelAsk(host.tab());
    });
    // Links in an answer open in the browser, not in the panel.
    log.addEventListener('click', (e) => {
        const a = e.target.closest('a[href]');
        if (!a) return;
        e.preventDefault();
        if (!a.getAttribute('href').startsWith('#')) host.open(a.href, e.metaKey || e.ctrlKey);
    });
    refreshAsk();
}
