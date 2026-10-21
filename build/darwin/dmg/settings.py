# dmgbuild configuration for TunnelKit macOS DMG installer.
# Writes .DS_Store and HiDPI background directly without Finder/AppleScript.
# Note: hide_extensions is intentionally omitted because SetFile -a E adds
# com.apple.FinderInfo extended attributes, which invalidates codesign --strict
# on .app bundles (and macOS Finder already hides .app extensions by default).

import os

app_path = defines.get("app_path", "bin/tunnelkit.app")
app_name = defines.get("app_name", "TunnelKit.app")
assets_dir = defines.get("assets_dir", "build/darwin/dmg")
volume_icon = defines.get("volume_icon", "build/darwin/icons.icns")

format = "UDZO"
compression_level = 9
filesystem = "HFS+"
size = None

files = [(app_path, app_name)]
symlinks = {"Applications": "/Applications"}

if volume_icon and os.path.isfile(volume_icon):
    icon = volume_icon

background = os.path.join(assets_dir, "background.png")

show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
sidebar_width = 180

# 660x400 content area + 28pt macOS window title bar = 660x428 outer window bounds
window_rect = ((200, 120), (660, 428))
default_view = "icon-view"

icon_size = 104
text_size = 13
label_pos = "bottom"

icon_locations = {
    app_name: (180, 172),
    "Applications": (480, 172),
}
