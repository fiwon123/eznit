#!/bin/sh
# Add the desktop app to every archive.
#
# Usage: scripts/pack-gui.sh <dist-dir> <gui-root>
#
# <dist-dir> holds the archives `make release` wrote — one per platform, named
# eznit_<version>_<os>_<arch>. <gui-root> holds one directory per platform —
# gui-linux-amd64/eznit-gui, gui-windows-arm64/eznit-gui.exe — as the release
# workflow's GUI matrix produced them.
#
# The window cannot be built where the archives are staged (it needs cgo and the
# platform's GL headers), so it is built by the matrix and packed in here. The
# archive is rebuilt in place from an extracted copy, so the one-folder layout is
# preserved and the checksums can be regenerated afterwards. The licences travel
# with the window: Fyne is BSD-3 and selfupdate is Apache-2.0.
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <dist-dir> <gui-root>" >&2
	exit 2
fi

dist=$1
guis=$2

packed=0

for archive in "$dist"/eznit_*; do
	[ -e "$archive" ] || continue

	base=$(basename "$archive")

	case "$base" in
	*.tar.gz)
		ext=".tar.gz"
		folder=${base%.tar.gz}
		;;
	*.zip)
		ext=".zip"
		folder=${base%.zip}
		;;
	*)
		continue
		;;
	esac

	# eznit_<version>_<os>_<arch> -> <os>_<arch>. Tags are vX.Y.Z, so the last
	# two underscore fields are always the platform.
	rest=${base#eznit_}
	rest=${rest%"$ext"}
	osarch=$(printf '%s' "$rest" | awk -F_ '{print $(NF-1)"_"$NF}')

	binary=eznit-gui
	case "$osarch" in
	windows_*) binary=eznit-gui.exe ;;
	esac

	# The workflow names the artifact directory with hyphens
	# (gui-linux-amd64); a caller may also lay it out with the archive's
	# underscore form.
	source="$guis/gui-$osarch/$binary"
	if [ ! -f "$source" ]; then
		hyphenated=$(printf '%s' "$osarch" | tr '_' '-')
		source="$guis/gui-$hyphenated/$binary"
	fi
	if [ ! -f "$source" ]; then
		# Every archive carries the window (six platforms build it). A missing
		# one is a broken GUI matrix, not a platform we ship without a window:
		# packaging it anyway would publish an archive that silently differs
		# from its neighbours, and the verifier's -gui group would only catch
		# it later.
		echo "ERROR: no window for $osarch — $base would ship without one" >&2
		exit 1
	fi

	# An absolute archive path: the recipe changes directory to rebuild it.
	archive="$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")"
	work=$(mktemp -d)
	trap 'rm -rf "$work"' EXIT

	if [ "$ext" = ".zip" ]; then
		unzip -q "$archive" -d "$work"
	else
		tar -xzf "$archive" -C "$work"
	fi

	cp "$source" "$work/$folder/$binary"
	# GitHub artifacts are zip-based and do not preserve modes: the downloaded
	# window is 0644, and an archive that ships it that way answers
	# "Permission denied" (v0.21.0). Restore the executable bit.
	chmod 0755 "$work/$folder/$binary"
	cp LICENSE-FYNE LICENSE-APACHE NOTICE "$work/$folder/"

	rm -f "$archive"
	if [ "$ext" = ".zip" ]; then
		(cd "$work" && zip -q -r "$archive" "$folder")
	else
		tar -czf "$archive" -C "$work" "$folder"
	fi

	rm -rf "$work"
	trap - EXIT

	echo "packed $base with $binary"
	packed=$((packed + 1))
done

# "No archive found" must not read as success.
if [ "$packed" -eq 0 ]; then
	echo "ERROR: no eznit archive in $dist — nothing was packed, so this is not a pass" >&2
	exit 1
fi

echo "packed $packed archive(s), each with the window"
