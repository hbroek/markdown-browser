// Draggable edges for the side panels. A splitter sits between a panel and the page; the
// width is remembered per panel. Arrow keys resize it, double-click puts it back.
const MIN = 200;

function saved(key) {
    try { return parseInt(localStorage.getItem(key), 10) || 0; } catch { return 0; }
}

function remember(key, width) {
    try { localStorage.setItem(key, String(width)); } catch { /* private window: keep it for this run */ }
}

// side is 'left' when the panel is left of the splitter, 'right' when it is right of it.
function attach(splitter, panel, key, side, fallback) {
    const limit = () => Math.max(MIN, Math.floor(window.innerWidth * 0.6));
    const set = (w, keep = true) => {
        const width = Math.min(limit(), Math.max(MIN, Math.round(w)));
        panel.style.width = width + 'px';
        splitter.setAttribute('aria-valuenow', String(width));
        if (keep) remember(key, width);
        return width;
    };
    const current = () => panel.getBoundingClientRect().width || fallback;

    const start = saved(key);
    if (start) set(start, false);
    splitter.setAttribute('role', 'separator');
    splitter.setAttribute('aria-orientation', 'vertical');
    splitter.setAttribute('aria-valuemin', String(MIN));
    splitter.tabIndex = 0;

    splitter.addEventListener('pointerdown', e => {
        e.preventDefault();
        const x0 = e.clientX, w0 = current();
        splitter.setPointerCapture(e.pointerId);
        document.body.classList.add('resizing');
        const move = ev => set(w0 + (side === 'left' ? ev.clientX - x0 : x0 - ev.clientX), false);
        const stop = () => {
            splitter.removeEventListener('pointermove', move);
            splitter.removeEventListener('pointerup', stop);
            splitter.removeEventListener('pointercancel', stop);
            document.body.classList.remove('resizing');
            remember(key, Math.round(current()));
        };
        splitter.addEventListener('pointermove', move);
        splitter.addEventListener('pointerup', stop);
        splitter.addEventListener('pointercancel', stop);
    });
    splitter.addEventListener('dblclick', () => { set(fallback); });
    splitter.addEventListener('keydown', e => {
        const step = e.shiftKey ? 80 : 20;
        const dir = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
        if (!dir) return;
        e.preventDefault();
        set(current() + (side === 'left' ? dir : -dir) * step);
    });
    window.addEventListener('resize', () => { if (!panel.hidden) set(current(), false); });

    // A splitter is only there while its panel is.
    const sync = () => { splitter.hidden = panel.hidden; };
    new MutationObserver(sync).observe(panel, {attributes: true, attributeFilter: ['hidden']});
    sync();
}

export function initSplitters() {
    attach(document.getElementById('bookmarks-splitter'), document.getElementById('bookmarks-panel'), 'width.bookmarks', 'left', 280);
    attach(document.getElementById('ask-splitter'), document.getElementById('ask-panel'), 'width.ask', 'right', 340);
}
