// Theme choice: follow the system (default), or force light or dark.
const button = document.getElementById('theme');
const modes = ['system', 'light', 'dark'];
const labels = {system: 'follow system', light: 'light', dark: 'dark'};
const icons = {system: '◐', light: '☀', dark: '☾'};

function saved() {
    try { return localStorage.getItem('theme'); } catch { return null; }
}

function apply(mode) {
    if (mode === 'system') delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = mode;
    button.textContent = icons[mode];
    button.title = `Theme: ${labels[mode]} (click to change)`;
    try {
        if (mode === 'system') localStorage.removeItem('theme');
        else localStorage.setItem('theme', mode);
    } catch { /* not persisted */ }
}

export function initTheme() {
    let mode = modes.includes(saved()) ? saved() : 'system';
    apply(mode);
    button.addEventListener('click', () => {
        mode = modes[(modes.indexOf(mode) + 1) % modes.length];
        apply(mode);
    });
}
