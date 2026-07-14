#!/usr/bin/env sh
set -eu

ARCH="${1:?architecture is required}"
TAG="${2:?release tag is required}"
VERSION="${TAG#v}"
BINARY="build/bin/TODO"
DIST="dist"
ROOT="$DIST/linux-$ARCH-root"
PORTABLE="$DIST/TODO-linux-$ARCH"

test -x "$BINARY"
rm -rf "$ROOT" "$PORTABLE"
mkdir -p "$DIST" "$ROOT/DEBIAN" "$PORTABLE"

install -Dm755 "$BINARY" "$ROOT/usr/bin/todo"
install -Dm644 build/appicon.png "$ROOT/usr/share/icons/hicolor/1024x1024/apps/todo.png"
mkdir -p "$ROOT/usr/share/applications"
cat > "$ROOT/usr/share/applications/todo.desktop" <<'EOF'
[Desktop Entry]
Name=TODO
Comment=Local-first task manager and collaborative notes
Exec=/usr/bin/todo
Icon=todo
Terminal=false
Type=Application
Categories=Office;Utility;
EOF

cat > "$ROOT/DEBIAN/control" <<EOF
Package: todo-desktop
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: Mon3tr-v <Mon3tr-v@users.noreply.github.com>
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Description: Local-first task manager and collaborative notes
 TODO is an offline-first desktop task and note application.
EOF

install -Dm755 "$BINARY" "$PORTABLE/TODO"
install -Dm644 build/appicon.png "$PORTABLE/todo.png"
install -Dm644 "$ROOT/usr/share/applications/todo.desktop" "$PORTABLE/TODO.desktop"
install -Dm644 README.md "$PORTABLE/README.md"

dpkg-deb --build --root-owner-group "$ROOT" "$DIST/TODO-linux-$ARCH.deb"
tar -czf "$DIST/TODO-linux-$ARCH.tar.gz" -C "$DIST" "TODO-linux-$ARCH"
rm -rf "$ROOT" "$PORTABLE"
