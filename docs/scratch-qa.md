# Scratch pad manual QA

Run with `EDITOR` set (for example `nvim`) and a throwaway notebook.

1. Press `s` from each panel (PARA, Content, Related, Stickies). The editor
   opens on `<notebook-root>/_scratch.md` every time, in edit mode.
2. With no `_scratch.md`, save some text in the editor. Quit the editor. The
   file now exists in the notebook root.
3. Press `s` again. The saved text is there.
4. Start the file with a `# Heading`. After quitting the editor, the file is
   still named `_scratch.md` (it is not renamed to match the title).
5. Quit the editor without saving on a fresh notebook. No file is created and
   no error shows.
6. Set `EDITOR` to a missing command. Pressing `s` shows an error popup.
7. Press `S` on a note: it pins as a sticky, and pressing `S` again unpins it.
   `s` no longer pins.
8. Open a popup or input (for example Add Note) and type an `s`. It types a
   letter and does not open the scratch pad.
9. Check the help popup (`?`) and the status bar list `s` and `S`.
