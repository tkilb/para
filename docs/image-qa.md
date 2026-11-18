# Manual QA: image attachments (`I`, bundling and the preview)

The interface has no automated tests, so run these by hand after changing
image support. Use a scratch notebook, not a real one. The engine under it
(`internal/imgfmt`, `internal/imgpreview`, `internal/attach`, and the image
parts of `internal/store`) is covered by tests that never touch a real
clipboard or terminal.

## Setup

1. Put two PNG files in the notebook's `inbox/`, one small (about 16 x 16) and
   one large (over 1000 pixels wide), and a note `note.md`.
2. Open para on it. Run the steps below once in a terminal that has an image
   protocol (Kitty, Ghostty or iTerm2) and once inside tmux, where the half
   block fallback is used. `PARA_IMAGE_PROTOCOL=blocks` forces the fallback.

## List and preview

| # | Step | Expect |
| --- | --- | --- |
| 1 | Look at the `inbox/` list | Each PNG has the badge ` IMG` (right-aligned in the four-character badge, no kind color) and its name without `.png`; `note.md` shows its kind as before. Folder counts include the images. |
| 2 | Move the cursor onto the large image | Content shows the whole image scaled to fit, with its aspect ratio, and the footer reads `W x H px, size`. Title shows the file name; Related and Info are empty. |
| 3 | Move onto the small image | It is drawn at its own size, not enlarged. |
| 4 | Move onto `note.md` | The image is gone completely (no leftover pixels in Kitty or iTerm2) and the note shows. Move back: the image returns. |
| 5 | Resize the terminal while an image is selected | The image is redrawn to the new size, with no leftovers. |
| 6 | Select the folder that holds the images | Content lists the images with the ` IMG` badge. |
| 7 | Press `e`, `K`, `mm`, `o` on an image | `e`, `mm` and `o` do nothing; `K` says an image has no kind. |
| 8 | Put a text file named `bad.png` in `inbox/` and select it | Content shows an error ("cannot read the image"), with no path in it. |

## Importing images (`I`)

The agent import can stay off (`digest.enabled` unset) for every step here.

| # | Step | Expect |
| --- | --- | --- |
| 9 | Copy text, press `I` | The box opens. Its hint is not an image ("text import is off" when the agent import is off). Enter shows the "Import is off" notice and nothing is written. |
| 10 | Take a screenshot to the clipboard (macOS: `cmd+ctrl+shift+4`), press `I` | The hint says `image from the clipboard, W x H px, saved as a PNG`. Enter saves `YYYY-MM-DD HH-MM-SS.png` in the folder PARA shows, the cursor is on it and the preview shows the screenshot. |
| 11 | Repeat step 10 within the same second | A second file with ` 2` before `.png`; the first is untouched. |
| 12 | Press `u`, then `ctrl+r` | The file goes away, then comes back. |
| 13 | Copy a JPEG or WebP image from a browser, press `I`, Enter | The saved file is a PNG (`file name.png` says PNG image data). |
| 14 | Press `I`, type the path to a `.jpg`, `.webp` or `.avif` file, Enter | The hint says `image file, W x H px, saved as a PNG`. A PNG with the file's name is saved; the original is not changed. A bare name such as `diagram` finds `diagram.png` in `inbox/`. |
| 15 | Type the path to a text file | The hint is the usual text hint (or "not an image" with the agent import off), and nothing image-related happens. |
| 16 | On Linux without `wl-paste` or `xclip`, copy an image and press `I` | The hint is the text hint: the clipboard image cannot be read. An image file path still works. |

## Bundling with a note

| # | Step | Expect |
| --- | --- | --- |
| 17 | `y` on an image, move onto `note.md`, `p` | A question: bundle the image with the note, or paste it beside. `esc` cancels and the buffer is kept. |
| 18 | Answer `y` | `note.assets/` appears on disk with the image; the original stays (a yank copies). PARA shows no `.assets` row; `note` shows `+1`, and the title of Content reads `Content - name`. |
| 19 | `u`, then `ctrl+r` | The image leaves the folder and the folder goes with it; then both return. |
| 20 | Repeat with `x` instead of `y` | The image is moved into the folder (a cut moves it). `u` puts it back. |
| 21 | Yank an image, move onto a note, `p`, answer `n` | The image is pasted beside the note, as a plain paste. |
| 22 | Same with `P` and `n` | Like `p`, but an existing file of that name is replaced after asking. |
| 23 | Yank two images, or an image and a note, move onto a note, `p` | No question: a plain paste. |
| 24 | Yank an image, put the cursor on a folder or another image, `p` | No question: a plain paste. |
| 25 | Bundle the same image twice into one note | The second is `name 2.png`; nothing is replaced. |

## Viewing a bundle (tabs in Content)

| # | Step | Expect |
| --- | --- | --- |
| 26 | Put the cursor on a note with two bundled images (PARA focused) | The title of pane `[0]` reads `[0]-Content - diagram - whiteboard` (the image names), with `Content` highlighted. Content shows the note text. A note with no images shows `[0]-Content` as before. |
| 27 | Press `[` or `]` with PARA focused | The PARA tab changes, not the Content tab. |
| 28 | Press `→` | Focus moves to Content, and the status bar starts with `[ ] image tabs`. |
| 29 | Press `]` | The first image's tab is highlighted and the image is drawn, scaled; the footer reads `1/2 name.png, W x H px, size`. |
| 30 | Press `]` again, then once more | The second image's tab, then back to `Content` and the note text at the place you left it. |
| 31 | Press `[` on the `Content` tab | It wraps to the last image (`[` and `]` wrap; `←` and `→` do not). |
| 32 | `esc`, then move to another note and back | Focus is back in PARA; coming back, the `Content` tab is open. |
| 33 | In Content on the `Content` tab, press `→` twice (two images bundled) | The first image's tab, then the second's. |
| 34 | Press `→` again on the last tab | Nothing happens: it does not wrap to the note. |
| 35 | Press `←` twice | Back to the first image's tab, then the `Content` tab. |
| 36 | Press `←` on the `Content` tab | Focus goes back to PARA. |
| 37 | With a line selection open on the `Content` tab (`space` or `v`), press `←` | The selection is cleared first and focus stays; a second `←` leaves. |
| 38 | On an image tab, press `esc` | Focus goes back to PARA at once, from any tab. |
| 39 | Bundle images with long names (for example `2026-11-18 09-05-07 quarterly review.png`) and narrow the terminal step by step | The open tab keeps its name and the other tabs shrink to `C` and numbers; when still too narrow, the open name is cut with `…`. The right border of the title never moves out of line. |
| 40 | Bundle an image whose name has accents or symbols (`café ☕.png`) | The tab shows the name and the title border stays aligned. |
| 41 | Bundle two images with short names (`a.png`, `b.png`) in a wide terminal | Every tab shows its full name at once: `Content - a - b`. |
| 42 | In Kitty or iTerm2, switch between image tabs and the `Content` tab | No leftover pixels of an image on the `Content` tab. |
| 43 | In Kitty or iTerm2, select an image and press `r`, `d`, or `?` | The rename box, the delete question and the help are fully visible and not hidden behind the image; the image is gone while they are open and returns when they close. |
| 44 | Bundle one more image with `p` while the note is selected | The tabs gain one named for the new image and the view returns to the `Content` tab. |
| 45 | With 10 or more images bundled | The numbers stay whole (`10`, not `1`) in the title. |

## Going into the bundle folder

| # | Step | Expect |
| --- | --- | --- |
| 46 | With the cursor on a note with bundled images, press `Enter` | PARA goes into `note.assets/`: its images are listed as `IMG` rows, the first is previewed in Content, and no tabs show. |
| 47 | Move down the rows | Each image is previewed as the cursor reaches it. |
| 48 | Press `←` | Back in the parent folder with the cursor on the note, not on another row. |
| 49 | On a note with no bundled images, press `Enter` | Focus moves to Content, as before. |
| 50 | Inside the folder: `r` an image, then `d` it, then `u` twice | The rename and the trash each undo; the `+N` and the tabs follow the count. |
| 51 | Inside the folder: `y` an image, `←`, move to another folder, `p` | The image is copied out as a loose image; the bundle keeps its own. |
| 52 | Inside the folder: `I` with an image file path | The image is saved in the bundle. |
| 53 | Delete every image in the folder, then `←` | The note row loses its `+N`, and `Enter` on it moves to Content again. |

## The bundle stays with the note

| # | Step | Expect |
| --- | --- | --- |
| 54 | `r` on a bundled note, type a new name | `.assets` is renamed to match; `+N` stays; `u` restores both. |
| 55 | `y` on the note, move to another folder, `p` | The note and its `.assets` copy arrive together, with the images as separate files. |
| 56 | `x` the note, `p` in another folder | Both move; the old folder is gone. |
| 57 | `d` on the note, then `u` | Both go to the trash and both return. `D` deletes both permanently, after asking. |
| 58 | Make the target `.assets` folder exist first (create `other.assets/` by hand) and move or copy a bundled note to the name `other.md` | The operation fails with "already exists" and nothing is moved or copied. |
| 59 | Create a folder `lonely.assets/` with no `lonely.md` beside it | It is listed as an ordinary folder. |

## Other actions

| # | Step | Expect |
| --- | --- | --- |
| 60 | `r` on an image, type `diagram` | It becomes `diagram.png`; `u` restores the name. Typing `diagram.png` gives the same. |
| 61 | `y`, move to another folder, `p` | A copy is made; pasting twice in one folder gives `name_1.png`, `name_2.png`. |
| 62 | `d` on an image, then `u` | It goes to the trash, then returns. |
| 63 | `ctrl+f` and `ctrl+/` | Images are not offered as notes and are not searched. |
