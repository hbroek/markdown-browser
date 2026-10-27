#!/bin/sh
# Installs Markdown Browser for the current user: the program, its launcher entry and its icon.
# Everything goes under ~/.local; remove those three files to uninstall.
set -e
here=$(cd "$(dirname "$0")" && pwd)
bin="$HOME/.local/bin"
apps="$HOME/.local/share/applications"
icons="$HOME/.local/share/icons/hicolor/512x512/apps"
mkdir -p "$bin" "$apps" "$icons"
install -m 755 "$here/markdown-browser" "$bin/markdown-browser"
install -m 644 "$here/markdown-browser.png" "$icons/markdown-browser.png"
sed "s|^Exec=.*|Exec=$bin/markdown-browser|" "$here/markdown-browser.desktop" > "$apps/markdown-browser.desktop"
chmod 644 "$apps/markdown-browser.desktop"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t "$HOME/.local/share/icons/hicolor" || true
echo "Installed. Start Markdown Browser from your applications menu, or run: $bin/markdown-browser"
case ":$PATH:" in *":$bin:"*) ;; *) echo "Note: $bin is not on your PATH." ;; esac
