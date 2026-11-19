# Manual QA: scrolling with a margin

Every long list keeps three rows of context around the cursor (like Vim's
`scrolloff`). Use a folder or notebook with at least 40 notes so each list
scrolls. The UI has no automated tests, so check these by eye.

| # | Step | Expect |
| --- | --- | --- |
| 1 | In the PARA panel, hold `down` from the top | The cursor moves down the panel to about three rows above the bottom edge, then stays there while the list scrolls. At the end of the list the cursor moves down to the last row. |
| 2 | Hold `up` from the bottom | The cursor moves up inside the window to about three rows below the top edge, then stays while the list scrolls up. At the start it moves to the first row. |
| 3 | Press `down` a few times, then `up` once or twice | The list does not scroll: the cursor moves inside the window until it is three rows from an edge. |
| 4 | `pgdown` and `pgup` in PARA | The cursor lands inside the window with the margin kept, never stuck on the last row. |
| 5 | Click a row with the mouse after scrolling | The row clicked gets the cursor (the click matches what is drawn). |
| 6 | `ctrl+f` (Find note), hold `down` and then `up` | The same margin behavior in the list. |
| 7 | `ctrl+/` (text search) with a word on 40 or more lines, hold `down` then `up` | The same margin behavior. |
| 8 | Open the glossary lookup, a kind picker and the notebook picker with enough rows | Each scrolls with the margin. The marks picker (`M`) is the exception: it keeps the highlight in the middle. |
| 9 | Make the window very short (a few rows) | The margin shrinks so the cursor can still move; nothing jumps or hides the cursor. |
