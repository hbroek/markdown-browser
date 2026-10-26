// The menu at the far right of the toolbar: site credentials, theme and color
// scheme. Theme is System, Light or Dark; the palettes of the color schemes live
// in schemes.css and `swatch` is only the preview shown here.
const $ = (id) => document.getElementById(id);
const root = document.documentElement;

const themes = [
    {id: 'system', name: 'Follow system', icon: '\u25D0'},
    {id: 'light', name: 'Light', icon: '\u2600'},
    {id: 'dark', name: 'Dark', icon: '\u263E'},
];
const schemes = [
    {id: '', name: 'Violet', swatch: ['#4f46e5', '#c026d3', '#f97316']},
    {id: 'ocean', name: 'Ocean', swatch: ['#0369a1', '#0891b2', '#f59e0b']},
    {id: 'forest', name: 'Forest', swatch: ['#15803d', '#0d9488', '#ca8a04']},
    {id: 'sunset', name: 'Sunset', swatch: ['#dc2626', '#ea580c', '#db2777']},
    {id: 'rose', name: 'Rose', swatch: ['#be185d', '#e11d48', '#7c3aed']},
    {id: 'graphite', name: 'Graphite', swatch: ['#475569', '#64748b', '#0ea5e9']},
];

function read(key) {
    try { return localStorage.getItem(key); } catch { return null; }
}

function write(key, value) {
    try {
        if (value) localStorage.setItem(key, value);
        else localStorage.removeItem(key);
    } catch { /* not persisted */ }
}

let theme = themes.some((t) => t.id === read('theme')) ? read('theme') : 'system';
let scheme = schemes.some((s) => s.id === read('scheme')) ? read('scheme') : '';
let credentials = {saved: false, origin: ''};

function applyTheme(id) {
    theme = id;
    if (id === 'system') delete root.dataset.theme;
    else root.dataset.theme = id;
    write('theme', id === 'system' ? '' : id);
}

function applyScheme(id) {
    scheme = id;
    if (id) root.dataset.scheme = id;
    else delete root.dataset.scheme;
    write('scheme', id);
}

/** The key lights up the menu button when the current site has a saved credential. */
export function showCredentialState(saved, origin) {
    credentials = {saved, origin};
    $('menu').classList.toggle('active', saved);
}

function item(role, label, checked, onClick, lead) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'menu-row';
    b.setAttribute('role', role);
    if (checked !== null) b.setAttribute('aria-checked', String(checked));
    if (lead) b.append(lead);
    b.append(label);
    b.addEventListener('click', onClick);
    return b;
}

function heading(text) {
    const h = document.createElement('div');
    h.className = 'menu-heading';
    h.textContent = text;
    return h;
}

const span = (className, text = '') => Object.assign(document.createElement('span'), {className, textContent: text});

/** `openCredentials()` is called from the menu's first entry. */
export function initMenu(openCredentials) {
    const btn = $('menu');
    const menu = $('settings-menu');

    const close = () => {
        menu.hidden = true;
        btn.setAttribute('aria-expanded', 'false');
    };

    function build() {
        const creds = item('menuitem', credentials.saved ? 'Site credentials (saved for this site)' : 'Site credentials\u2026', null,
            () => { close(); openCredentials(); }, span('menu-icon', '\u{1F511}'));
        menu.replaceChildren(
            creds,
            heading('Theme'),
            ...themes.map((t) => item('menuitemradio', t.name, t.id === theme, () => { applyTheme(t.id); build(); }, span('menu-icon', t.icon))),
            heading('Color scheme'),
            ...schemes.map((s) => {
                const sw = span('swatch');
                sw.style.background = `linear-gradient(90deg, ${s.swatch.join(', ')})`;
                return item('menuitemradio', s.name, s.id === scheme, () => { applyScheme(s.id); build(); }, sw);
            }),
        );
    }

    function open() {
        build();
        menu.hidden = false;
        btn.setAttribute('aria-expanded', 'true');
        menu.querySelector('button')?.focus();
    }

    applyTheme(theme);
    applyScheme(scheme);
    btn.addEventListener('click', () => (menu.hidden ? open() : close()));
    document.addEventListener('mousedown', (e) => {
        if (!menu.hidden && !e.target.closest('#settings-menu') && !e.target.closest('#menu')) close();
    });
    document.addEventListener('keydown', (e) => {
        if (menu.hidden) return;
        if (e.key === 'Escape') { close(); btn.focus(); return; }
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            const rows = [...menu.querySelectorAll('button')];
            const i = rows.indexOf(document.activeElement);
            rows[(i + (e.key === 'ArrowDown' ? 1 : -1) + rows.length) % rows.length].focus();
        }
    });
}
