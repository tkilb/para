# Manual QA: image attachments (`C` and the preview)

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

## Pasting (`C`)

| # | Step | Expect |
| --- | --- | --- |
| 9 | Copy text, then press `C` | A notice: the clipboard has no image. Nothing is written. |
| 10 | Take a screenshot to the clipboard (macOS: `cmd+ctrl+shift+4`), press `C` | `YYYY-MM-DD HH-MM-SS.png` appears in the folder PARA shows, the cursor is on it, and the preview shows the screenshot. |
| 11 | Press `C` again within the same second | A second file with ` 2` before `.png`; the first is untouched. |
| 12 | Press `u`, then `ctrl+r` | The file goes away, then comes back. |
| 13 | Copy a JPEG or WebP image from a browser, press `C` | The saved file is a PNG (`file name.png` says PNG image data). |
| 14 | On Linux without `wl-paste` or `xclip`, press `C` | A notice names `wl-clipboard` and `xclip`. |

## Other actions

| # | Step | Expect |
| --- | --- | --- |
| 15 | `r` on an image, type `diagram` | It becomes `diagram.png`; `u` restores the name. Typing `diagram.png` gives the same. |
| 16 | `y`, move to another folder, `p` | A copy is made; pasting twice in one folder gives `name_1.png`, `name_2.png`. |
| 17 | `d` on an image, then `u` | It goes to the trash, then returns. |
| 18 | `ctrl+f` and `ctrl+/` | Images are not offered as notes and are not searched. |
