# Manual QA: cursor memory when drilling out and back in

Going up with `left` and straight back into the same folder restores the row
the cursor was on, as Yazi does. Use a folder with several subfolders, each
holding several notes. The UI has no automated tests, so check these by eye.

| # | Step | Expect |
| --- | --- | --- |
| 1 | Enter a folder, move the cursor to the third row, press `left`, then `right` | The cursor is on the third row again, not the first. |
| 2 | Go two or three folders deep, moving the cursor off the first row in each. Press `left` that many times, then `right` that many times | Each folder shows the row you left in it, all the way back down. |
| 3 | Press `left`, then `down`, `up`, then `right` | The cursor is on the first row: any other key clears the memory. |
| 4 | Press `left`, move onto a different folder, then `right` | The cursor is on the first row of that folder. |
| 5 | Press `left`, then `[` or `]` and back to the tab, then `right` | The cursor is on the first row. |
| 6 | Press `left`, delete or rename the remembered note outside the app, then `right` | No error; the cursor falls back to the first row. |
| 7 | Press `left` in an empty folder, then `right` | The cursor is on the first row; nothing breaks. |
