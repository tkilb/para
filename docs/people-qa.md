# Manual QA: the People field of the Add Note form

| # | Step | Expect |
| --- | --- | --- |
| 1 | Press `A` | The form has a People box under Tags, with the placeholder `optional; comma separated, such as Tyler K., Amber D.`. The key hints show on the People box while it has focus, and not on Tags. |
| 2 | Press `tab` from Tags, then `tab` again | Focus goes to People, then wraps to Title. `shift+tab` and `↑`/`↓` move the same way; `↓` on People stays put. |
| 3 | Type a title, then in People `Tyler K., amber d` and press `enter` | The note is saved; its Info panel shows `Tyler K., amber d` after People, and the file has a `people` list of two names in the order typed. |
| 4 | Add a note with People `Ann Lee, ann lee, , Bo` | `people` holds `Ann Lee` and `Bo` only. |
| 5 | Add a note with People `O'Brien: "Pat"` | The file's front matter stays valid: reopen the note and Info shows the name unchanged. |
| 6 | Add a note with People left empty | The `people` property is there and empty, as before. |
| 7 | Type only in People, press `esc` | PARA asks `Discard the note you started?`. |
| 8 | Choose a template, type in People, change the template | People keeps what you typed. |
| 9 | Resize the terminal small (about 80 x 24) | The form still fits and every box shows; the body gets fewer rows. |
