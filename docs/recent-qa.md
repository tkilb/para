# Manual QA: recent notes (`R`)

Use a notebook with at least 250 notes of different ages. The UI has no
automated tests, so check these by eye.

| # | Step | Expect |
| --- | --- | --- |
| 1 | Press `R` (shift+r) with a panel in focus | A `Recent notes` popup opens with the Space, Kind and Dir boxes above the list. The status bar lists `R recent`. |
| 2 | Look at the list | The note changed last is first, with how long ago it changed at the right (`5m ago`, `3h ago`, `2d ago`, `3mo ago`). The times never go up as you go down. The count at the bottom right is at most 200. |
| 3 | Edit a note elsewhere (or touch it), then press `R` again | It is now first, `now` or `1m ago`. |
| 4 | Type the start of a word that is in the name of several notes of different ages | Notes with the word in their name are listed newest first, whatever the letters before or after it. Notes where the letters are only scattered across the name come after them, and notes matching only a folder last. |
| 5 | Type a word and compare with `ctrl+f` | Find note orders the same matches by how well they fit (an exact name first). Recent notes orders them by how recently they changed. |
| 6 | `tab` to Kind, change it, or press `ctrl+f` and set Modified since | The list is the 200 newest of the notes that pass the filters, so a Kind with few notes lists all of them. What is typed stays. |
| 7 | Press `enter` on a note | PARA jumps to the note and the popup closes. `esc` closes it without moving. |
| 8 | Press `ctrl+f` from the notebook (Find note) afterwards | Find note is unchanged: all notes, sorted by path, no age shown. |
