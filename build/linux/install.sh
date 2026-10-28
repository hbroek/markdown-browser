#!/bin/sh
# Installs Mardo for the current user: the program, its launcher entry and its icon.
# Everything goes under ~/.local; remove those three files to uninstall.
set -e
here=$(cd "$(dirname "$0")" && pwd)
bin="$HOME/.local/bin"
apps="$HOME/.local/share/applications"
icons="$HOME/.local/share/icons/hicolor/512x512/apps"
mkdir -p "$bin" "$apps" "$icons"
install -m 755 "$here/mardo" "$bin/mardo"
install -m 644 "$here/mardo.png" "$icons/mardo.png"
sed "s|^Exec=.*|Exec=$bin/mardo|" "$here/mardo.desktop" > "$apps/mardo.desktop"
chmod 644 "$apps/mardo.desktop"
# Remove what this program was installed as before it was renamed.
rm -f "$bin/markdown-browser" "$apps/markdown-browser.desktop" "$icons/markdown-browser.png"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t "$HOME/.local/share/icons/hicolor" || true
echo "Installed. Start Mardo from your applications menu, or run: $bin/mardo"
case ":$PATH:" in *":$bin:"*) ;; *) echo "Note: $bin is not on your PATH." ;; esac
