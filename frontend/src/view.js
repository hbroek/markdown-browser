// Builds the DOM for each kind of page. No state lives here.

function el(tag, props = {}, ...children) {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
}

export function welcome() {
    const shortcuts = [
        ['Ctrl/Cmd + L', 'Focus the address bar'],
        ['Ctrl/Cmd + R', 'Reload'],
        ['Alt + ← / →', 'Back / forward'],
        ['Ctrl/Cmd + U', 'Toggle markdown source'],
    ];
    return el('section', {className: 'welcome'},
        el('h1', {textContent: 'Markdown Browser'}),
        el('p', {textContent: 'Enter an http, https or file address above. Pages are requested as markdown and rendered here. Anything else shows its title and links.'}),
        el('dl', {}, ...shortcuts.flatMap(([k, v]) => [el('dt', {textContent: k}), el('dd', {textContent: v})])),
    );
}

export function markdown(p) {
    // p.html is sanitized by the backend.
    return el('article', {className: 'markdown', innerHTML: p.html});
}

export function source(p) {
    return el('pre', {className: 'source'}, el('code', {textContent: p.source}));
}

export function fallback(p) {
    const links = p.links ?? [];
    return el('section', {className: 'fallback'},
        el('h1', {textContent: p.title}),
        el('p', {className: 'note', textContent: `No markdown available (${p.contentType || 'unknown type'}). ${links.length} link${links.length === 1 ? '' : 's'} found.`}),
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

export function historyMenu(entries, onPick, onClear, onFull) {
    if (!entries.length) {
        return [el('div', {className: 'menu-empty', textContent: 'No history yet'})];
    }
    return [
        ...entries.map((e) => el('button', {className: 'menu-item', title: e.url, onclick: () => onPick(e.url)},
            el('span', {className: 'menu-title', textContent: e.title || e.url}),
            el('span', {className: 'href', textContent: e.url}),
        )),
        el('button', {className: 'menu-clear', textContent: 'Full history…', onclick: onFull}),
        el('button', {className: 'menu-clear', textContent: 'Clear history', onclick: onClear}),
    ];
}
