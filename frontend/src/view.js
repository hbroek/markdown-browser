// Builds the DOM for each kind of page. No state lives here.

function el(tag, props = {}, ...children) {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
}

function originOf(url) {
    try {
        const o = new URL(url).origin;
        return o === 'null' || !o.startsWith('http') ? '' : o;
    } catch {
        return '';
    }
}

export function welcome() {
    const shortcuts = [
        ['Ctrl/Cmd + L', 'Focus the address bar'],
        ['Ctrl/Cmd + T / W', 'New tab / close tab'],
        ['Ctrl + Tab, Ctrl/Cmd + 1-9', 'Switch tabs'],
        ['Ctrl/Cmd + R', 'Reload'],
        ['Alt + ← / →', 'Back / forward'],
        ['Ctrl/Cmd + U', 'Toggle markdown source'],
        ['Ctrl/Cmd + J', 'Show / hide the network console'],
    ];
    return el('section', {className: 'welcome'},
        el('h1', {textContent: 'Mardo - Markdown Browser'}),
        el('p', {textContent: 'Enter an http, https or file address above. Pages are requested as markdown and rendered here. Anything else shows its title and links.'}),
        el('dl', {}, ...shortcuts.flatMap(([k, v]) => [el('dt', {textContent: k}), el('dd', {textContent: v})])),
    );
}

export function markdown(p) {
    // p.html is sanitized by the backend.
    return el('article', {className: 'markdown', innerHTML: p.html});
}

/** A busy line with a spinner and the seconds waited, for models that take their time. */
export function waiting(text) {
    const p = el('p', {className: 'note busy', textContent: `${text}\u2026`});
    const started = Date.now();
    const timer = setInterval(() => {
        if (!p.isConnected) return clearInterval(timer); // the card was replaced or closed
        p.textContent = `${text}\u2026 ${Math.round((Date.now() - started) / 1000)}s`;
    }, 1000);
    return p;
}

/** 4.2s under ten seconds, then whole seconds, then minutes. */
export function seconds(ms) {
    const s = ms / 1000;
    if (s < 10) return `${s.toFixed(1)}s`;
    if (s < 100) return `${Math.round(s)}s`;
    return `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`;
}

/** The AI summary shown above a page: loading, the summary, or what went wrong. */
export function summary(s, onClose, onCancel) {
    const body = s.state === 'loading' ? el('div', {}, waiting(s.busy ?? 'Reading the page and asking the model'),
        el('button', {type: 'button', className: 'summary-cancel', textContent: 'Cancel', onclick: onCancel}))
        : s.state === 'error' ? el('p', {className: 'error-text', textContent: s.error})
        : el('div', {className: 'markdown', innerHTML: s.html}); // sanitized by the backend
    return el('aside', {className: 'summary', ariaLabel: s.title ?? 'AI summary'},
        el('div', {className: 'summary-head'},
            el('strong', {textContent: `\u2728 ${s.title ?? 'Summary'}`}),
            el('button', {type: 'button', className: 'summary-close', title: 'Close', textContent: '\u00D7', onclick: onClose})),
        body,
        s.state === 'done' ? el('p', {className: 'note', textContent: `Written by ${s.source} in ${seconds(s.millis)}; it can be wrong.`}) : '');
}

export function source(p) {
    return el('pre', {className: 'source'}, el('code', {textContent: p.source}));
}

export function fallback(p, onCredentials) {
    const links = p.links ?? [];
    const origin = originOf(p.url);
    return el('section', {className: 'fallback'},
        el('h1', {textContent: p.title}),
        el('p', {className: 'note', textContent: `No markdown available (${p.contentType || 'unknown type'}). ${links.length} link${links.length === 1 ? '' : 's'} found.`}),
        origin ? el('p', {className: 'note'}, 'Expected a signed-in page? ',
            el('a', {href: '#', textContent: `Add credentials for ${origin}`, onclick: (e) => { e.preventDefault(); onCredentials(origin); }})) : '',
        el('ul', {}, ...links.map((l) => el('li', {},
            el('a', {href: l.href, textContent: l.text}),
            l.text === l.href ? '' : el('span', {className: 'href', textContent: l.href}),
        ))),
    );
}

export function error(p, onRetry, onCredentials) {
    const needsLogin = (p.status === 401 || p.status === 403) && p.origin;
    return el('section', {className: 'error'},
        el('h1', {textContent: needsLogin ? 'Sign-in required' : 'Cannot display this page'}),
        el('p', {textContent: p.error}),
        needsLogin ? el('p', {className: 'note', textContent: 'This site did not accept the request as anonymous. Add your session cookies or a token for it.'}) : '',
        p.url ? el('p', {className: 'href', textContent: p.url}) : '',
        needsLogin ? el('button', {className: 'primary', textContent: `Add credentials for ${p.origin}`, onclick: () => onCredentials(p)}) : '',
        p.url ? el('button', {textContent: 'Retry', onclick: onRetry}) : '',
    );
}
