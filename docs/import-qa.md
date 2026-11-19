# Manual QA: agentic import (`I`)

The interface has no automated tests, so run these by hand after changing the
import. Use a scratch notebook, not a real one. The engine under it
(`internal/digest`, `internal/loader`) is covered by tests that use a fake
agent and never call a model.

## Setup

1. Add to `~/.config/para/config.yaml`:

   ```yaml
   digest:
     enabled: true
   ```

   and an `import` entry under `kinds` (see the README) so files get a kind.
2. Put a short transcript in the notebook's `inbox/`, for example
   `standup.txt` with a few lines of `Name: text` turns, some small talk and
   one action item.
3. `make install`, then start `para`.

## Checks

| # | Do | Expect |
|---|----|--------|
| 1 | With `digest.enabled` off (or absent), press `I` | A notice says import is off and how to turn it on. No box opens. |
| 2 | Enabled: press `I` with nothing selected on an import | A box "Import" opens. Under the field a grey line: `clipboard, N words` (or `the clipboard is empty`). |
| 3 | Type `stand` | The grey line says `file not found`. |
| 4 | Press `Tab` | The field becomes `standup.txt` and the line says `file, N bytes`. |
| 5 | Type `~/Dow`, press `Tab` | Completes to `~/Downloads/`. With several matches the line lists them (`3 matches: …`). |
| 6 | Type `https://example.com` | The line says `web page, example.com`. Type `https://` : `not a valid web address`. |
| 7 | Press `Enter` on `standup.txt` | The box closes. The status bar shows `Importing…` and the app stays usable (move around, open notes). |
| 8 | While it runs, press `I` again | A notice says an import is already running. |
| 9 | When it finishes | Status bar: `✓ Imported: IMPORT_YYYY-MM-DD <topic> (N files)`, gone after 5 seconds. The cursor has not moved. |
| 10 | Open the new folder in the Inbox | `_source.md` first (your text, unchanged), concept notes, `_actions.md` only if there were actions, `_facts.md` only if there were facts, and `_conclusions.md` only if something was concluded or decided. Each file has `kind: import`, a source, the date and tags. Small talk is gone. |
| 11 | Press `u` | The whole folder disappears in one undo. `ctrl+r` brings it back. |
| 12 | Put the cursor on the `IMPORT_` folder (or any file inside it) and press `I` | No box. Status: `Re-running IMPORT_…`, then `✓ Re-ran: …`. Old files are replaced; the folder keeps its name. |
| 13 | Press `u` after a re-run | The previous files come back exactly as they were. |
| 14 | Copy some text, press `I` on an empty box, `Enter` | A folder `IMPORT_<date> <topic or clipboard>` appears. |
| 15 | `I`, type `/nonexistent`, `Enter` | A notice: `Could not import: File not found.` Nothing is written. |
| 16 | Set `digest.command` to a name that does not exist, import | A notice: the agent command was not found, and the source is saved in an `IMPORT_…` folder (press `I` on it to retry). |
| 17 | Import a page behind a login (a Confluence or Slack URL) | A notice: the page needs a login; paste its text or save it as a file. Nothing is written. |
| 18 | Switch notebook (`B`) while an import runs | The finished import lands in the notebook it started in. The status message still names the folder, and the notebook now open is not reloaded or disturbed. |
| 19 | Press `ctrl+/`, search a word that is only in an import's `_source.md`, then press `ctrl+s` | At first there are no matches and the hint reads `_source.md hidden`. After `ctrl+s` the transcript lines are listed and the hint reads `shown`; `ctrl+s` again hides them. Lines of other notes in the folder are listed either way. |
| 20 | In `ctrl+/`, `tab` to Kind and press `→` | The Kind box shows `fact`, then the other kinds, then `unset`, then wraps to `All`. With a kind chosen only the lines of notes of that kind are listed, and the count follows. `unset` lists notes with no kind. Space, Kind and Dir narrow together, and changing the Space clears the Dir. |
| 21 | In `ctrl+/`, `tab` to Dir | A `Folders` dropdown opens under the box, over the top of the preview, listing the folders inside the space. Type the start of one: the list narrows. `↓` highlights the first, `↑` the last, and past either end the highlight goes away. `tab` or `enter` on a highlight fills it in with a `/` and the list shows the folders inside it. With none highlighted, `tab` completes the start several share, `enter` opens the chosen hit, and `tab` with nothing to complete moves to the query. A long list scrolls with the highlight. The dropdown closes when Dir loses focus. |

## Known limits

- There is no cancel key. A stuck agent stops at `digest.timeout` (5 minutes
  by default).
- Quitting during a run kills the agent. The `_source.md` is already saved, so
  press `I` on the folder next time.
- Real model output is the thing to judge by eye: whether notes cluster well
  and small talk is dropped. Tune the built-in prompts in
  `internal/digest/prompts/`, or point `digest.prompt` and `digest.merge_prompt`
  at files to try a variant without a rebuild.
