# Glossary manual QA

Use a scratch notebook, never a real one.

1. Press `ctrl+y` with no `glossary.yaml`: an empty picker opens, footer lists the keys.
2. `ctrl+n`: enter a term, then a definition. The picker returns with the term highlighted and `glossary.yaml` exists.
3. Add a second term; type part of either name: the list narrows (words must all match).
4. `enter` on a term: a popup shows the full definition; any key returns to the glossary.
5. `ctrl+e`: both fields are pre-filled. Rename the term and change the definition: it keeps its place in the list and the file.
6. Rename onto another existing term: an error popup, nothing changes.
7. `ctrl+x`, then `n`: nothing is deleted. `ctrl+x`, then `y`: the term is gone.
8. Put `# comment` at the top of the file, then add and edit: the comment stays.
9. Replace the file with `- a` : `ctrl+y` reports an error and the file is unchanged.
10. Add the same term with a different definition: both rows show, the file holds a list under the term. Adding the identical pair is refused. Edit and delete act on the highlighted row only; deleting the last one removes the term.
11. `esc` closes the picker at every step.
