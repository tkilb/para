# Marks popup manual QA

Run with a throwaway notebook that has several notes.

1. Press `M` with no marks. An info popup says there are no marked notes.
2. Mark three notes with `m` `m` (in different folders). A single `m` only shows
   "press m again" in the status bar and marks nothing; any other key after it
   cancels, and is handled as usual. Press `M` from each panel.
   The Marks popup opens with the notes under bold headings: the top folder
   (orange, in Inbox, Projects, Areas, Resources, Archives order), then its
   second folder (blue), with the rest of the path under it. A note directly in
   a top folder sits under that heading alone.
3. Type part of a name, with skipped letters (a fuzzy match). The list narrows,
   headings with no match disappear, and the matched letters (including ones in
   a folder name) are highlighted.
4. Move with the arrow keys. There is no preview pane. With a long list, the
   highlighted row stays in the middle of the list.
   Select a marked note in PARA, press `M`: that note is highlighted and
   centered. Select an unmarked note: the highlight starts at the top.
5. Press `enter`. The popup closes and PARA shows the note.
6. Press `esc` in the popup. Nothing changes.
7. Rename or move a marked note. `M` lists it under its new path. Delete one
   outside para. It no longer appears in the popup.
8. Press `S`. Nothing happens, and the screen has no Stickies panel. `1` focuses
   the empty `[1]` pane (one line, title only), `2` PARA, `3` Related, `4` Info
   and `0` Content. `esc` leaves `[1]` for PARA.
9. Upgrade check: with a state database that has stickies, start para. Each
   sticky is now a mark (marked bullet in PARA, listed in `M`).
10. Check the help popup (`?`) and the status bar list `m` (mark) and `M` (marks).
