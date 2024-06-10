import os.path
import plistlib

application = defines.get("app", "QPep.app/")
appname = os.path.basename(application)

format = defines.get("format", "UDBZ")

# Volume size
size = defines.get("size", None)  # noqa: F821

# Files to include
files = [application]
hide = [ 'QPep.app/Contents/MacOS/config/', 'QPep.app/Contents/MacOS/qpep' ]

# Symlinks to create
symlinks = {"Applications": "/Applications"}

icon = 'QPep.app/Contents/Resources/QPep.icns'

icon_locations = { "QPep": (140,120), "Applications": (500,120) }

background = "builtin-arrow"

show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
sidebar_width = 180

# Window position in ((x, y), (w, h)) format
window_rect = ((100, 100), (640, 280))

# Select the default view; must be one of
#
#    'icon-view'
#    'list-view'
#    'column-view'
#    'coverflow'
#
default_view = "icon-view"

# General view configuration
show_icon_preview = False

# Set these to True to force inclusion of icon/list view settings (otherwise
# we only include settings for the default view)
include_icon_view_settings = "auto"
include_list_view_settings = "auto"

# .. Icon view configuration ...................................................

arrange_by = None
grid_offset = (0, 0)
grid_spacing = 100
scroll_position = (0, 0)
label_pos = "bottom"  # or 'right'
text_size = 16
icon_size = 128

# .. List view configuration ...................................................

# Column names are as follows:
#
#   name
#   date-modified
#   date-created
#   date-added
#   date-last-opened
#   size
#   kind
#   label
#   version
#   comments
#
list_icon_size = 16
list_text_size = 12
list_scroll_position = (0, 0)
list_sort_by = "name"
list_use_relative_dates = True
list_calculate_all_sizes = (False,)
list_columns = ("name", "date-modified", "size", "kind", "date-added")
list_column_widths = {
    "name": 300,
    "date-modified": 181,
    "date-created": 181,
    "date-added": 181,
    "date-last-opened": 181,
    "size": 97,
    "kind": 115,
    "label": 100,
    "version": 75,
    "comments": 300,
}
list_column_sort_directions = {
    "name": "ascending",
    "date-modified": "descending",
    "date-created": "descending",
    "date-added": "descending",
    "date-last-opened": "descending",
    "size": "descending",
    "kind": "ascending",
    "label": "ascending",
    "version": "ascending",
    "comments": "ascending",
}

# .. License configuration .....................................................

# Text in the license configuration is stored in the resources, which means
# it gets stored in a legacy Mac encoding according to the language.  dmgbuild
# will *try* to convert Unicode strings to the appropriate encoding, *but*
# you should be aware that Python doesn't support all of the necessary encodings;
# in many cases you will need to encode the text yourself and use byte strings
# instead here.

# Recognized language names are:
#
#    da_DK, de_AT, de_CH, de_DE, en_AU, en_GB, en_IE, en_SG, en_US, es_ES,
#    fi_FI, fr_BE, fr_FR, fr_CA, fr_CH, it_IT, ja_JP, ko_KR, nb_NO, nl_BE,
#    nl_NL, pt_BR, pt_PT, ru_RU, sv_SE, zh_CN, zh_TW,

license = {
    "default-language": "en_US",
    "licenses": {
        # For each language, the text of the license.  This can be plain text,
        # RTF (in which case it must start "{\rtf1"), or a path to a file
        # containing the license text.  If you're using RTF,
        # watch out for Python escaping (or read it from a file).
        "en_GB": b"""QPep.app\\Contents\\Resources\\en.lproj\\Credits.rtf""",
    },
    "buttons": {
        # For each language, text for the buttons on the licensing window.
        #
        # Default buttons and text are built-in for the following languages:
        #
        #   da_DK: Danish
        #   de_DE: German
        #   en_AU: English (Australian)
        #   en_GB: English (UK)
        #   en_NZ: English (New Zealand)
        #   en_US: English (US)
        #   es_ES: Spanish
        #   fr_CA: French (Canadian)
        #   fr_FR: French
        #   it_IT: Italian
        #   ja_JP: Japanese
        #   nb_NO: Norsk
        #   nl_BE: Flemish
        #   nl_NL: Dutch
        #   pt_BR: Brazilian Portuguese
        #   pt_PT: Portugese
        #   sv_SE: Swedish
        #   zh_CN: Simplified Chinese
        #   zh_TW: Traditional Chinese
        #
        # You don't need to specify them for those languages; if you fail to
        # specify them for some other language, English will be used instead.
        "en_US": (
            b"English",
            b"Agree!",
            b"Disagree!",
            b"Print!",
            b"Save!",
            b'Do you agree or not? Press "Agree" or "Disagree".',
        ),
    },
}