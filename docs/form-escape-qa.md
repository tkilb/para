# Manual QA: esc in the Add Note form asks only when something was typed

Open the form with the add-note key. The UI has no automated tests, so check
these by eye.

| # | Step | Expect |
| --- | --- | --- |
| 1 | Open the form and press `esc` at once | The form closes with no question. |
| 2 | Type a title, then press `esc` | The question "Discard the note you started?" shows over the form. |
| 3 | At the question, press `esc` or `n` | The question closes and the form is still there with the text. |
| 4 | Press `esc` again, then `y` or `enter` | The form closes and no note is saved. |
| 5 | Type text only in the body, tags, or change the kind or source, then `esc` | Each one asks. |
| 6 | Type a title, delete it so the form is empty again, then `esc` | No question: the form matches the empty form. |
| 7 | Choose a template (when the notebook has templates), then `esc` | The question shows, since the template filled the form. |
| 8 | Save with invalid tags so the form reopens with the text, then `esc` | The question shows. |
| 9 | Save a note normally | No question; the note is saved. |
