package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tkilb/para/internal/store"
)

func TestCreateRenameTrashDeleteInStore(t *testing.T) {
	root := t.TempDir()
	st := store.NewMarkdown(root)

	// Create skill
	fn, createAct, err := CreateInStore(st, "summarize", ModeNote)
	require.NoError(t, err)
	assert.Equal(t, "summarize.md", fn)

	list, err := List(root)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "summarize", list[0].Name)

	// Undo create
	require.NoError(t, createAct.Undo())
	list, err = List(root)
	require.NoError(t, err)
	assert.Empty(t, list)

	// Redo create
	require.NoError(t, createAct.Redo())
	list, err = List(root)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Rename skill
	newFn, renameAct, err := RenameInStore(st, "summarize.md", "brief")
	require.NoError(t, err)
	assert.Equal(t, "brief.md", newFn)

	list, err = List(root)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "brief", list[0].Name)
	assert.Equal(t, "brief.md", list[0].Filename)

	// Undo rename
	require.NoError(t, renameAct.Undo())
	list, err = List(root)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "summarize", list[0].Name)
	assert.Equal(t, "summarize.md", list[0].Filename)

	// Trash skill and undo
	trashAct, err := TrashInStore(st, []string{"summarize.md"})
	require.NoError(t, err)
	list, err = List(root)
	require.NoError(t, err)
	assert.Empty(t, list)

	require.NoError(t, trashAct.Undo())
	list, err = List(root)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Permanent delete
	require.NoError(t, DeleteInStore(st, []string{"summarize.md"}))
	list, err = List(root)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestPasteDuplicateAndCrossNotebook(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	stA := store.NewMarkdown(rootA)
	stB := store.NewMarkdown(rootB)

	_, err := Save(rootA, Skill{
		Name:        "review",
		Description: "Code review prompt",
		Mode:        ModeInteractive,
		Body:        "Review carefully.",
	})
	require.NoError(t, err)

	// 1. Yank & paste in the same notebook -> creates review_1.md
	planSame, err := PlanPaste(stA, stA, true, []string{"review.md"}, false)
	require.NoError(t, err)
	require.Len(t, planSame.Items, 1)
	assert.Equal(t, "review_1.md", planSame.Items[0].DstFilename)

	dupAct, firstDst, err := ExecutePaste(stA, stA, planSame, false)
	require.NoError(t, err)
	assert.Equal(t, "review_1.md", firstDst)

	listA, err := List(rootA)
	require.NoError(t, err)
	require.Len(t, listA, 2)
	assert.Equal(t, "review", listA[0].Name)
	assert.Equal(t, "review_1", listA[1].Name)

	// Undo duplicate
	require.NoError(t, dupAct.Undo())
	listA, err = List(rootA)
	require.NoError(t, err)
	require.Len(t, listA, 1)

	// 2. Cut from notebook A and paste into notebook B
	planCross, err := PlanPaste(stA, stB, false, []string{"review.md"}, true)
	require.NoError(t, err)
	require.Len(t, planCross.Items, 1)
	assert.Equal(t, 0, planCross.Collisions)

	crossAct, dstB, err := ExecutePaste(stA, stB, planCross, true)
	require.NoError(t, err)
	assert.Equal(t, "review.md", dstB)

	listA, _ = List(rootA)
	assert.Empty(t, listA)
	listB, _ := List(rootB)
	require.Len(t, listB, 1)
	assert.Equal(t, "review", listB[0].Name)

	// Undo cross-notebook cut+paste restores notebook A and removes from notebook B
	require.NoError(t, crossAct.Undo())
	listA, _ = List(rootA)
	require.Len(t, listA, 1)
	listB, _ = List(rootB)
	assert.Empty(t, listB)
}
