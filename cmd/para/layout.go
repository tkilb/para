package main

const (
	// panelBorderRows is the rows a panel's top and bottom borders take.
	panelBorderRows = 2
	// titleHeight is the outer height of the Title panel: one content row
	// plus the border.
	titleHeight = 3
	// minSideHeight is the smallest outer height given to Related and Content.
	minSideHeight = 5
	// infoHeight is the outer height of the Info panel: the three standard
	// rows plus the border. Other properties scroll.
	infoHeight = 5
	// blankHeight is the outer height of the [1] pane: one row plus the
	// border.
	blankHeight = 3
	// focusedBlankHeight is the outer height when pane [1] is focused (7-10 rows).
	focusedBlankHeight = 9
	// compactHeight is the grid height below which the layout is compact:
	// only PARA and Content are drawn, each the full height of its column.
	compactHeight = 24
	// leftWidthPercent is the left column share of the terminal width.
	leftWidthPercent = 30
)

// grid holds the outer (border included) size of every panel.
type grid struct {
	leftWidth, rightWidth                  int
	blankHeight, paraHeight, relatedHeight int
	titleHeight, contentHeight, infoHeight int
	// compact is set when the terminal is short: the [1] pane, Title,
	// Related and Info are hidden (their heights are 0).
	compact bool
}

// shows reports whether the grid draws the panel p.
func (g grid) shows(p panelID) bool {
	return !g.compact || p == panelPARA || p == panelContent
}

// zoomed gives the focused panel the whole width x height, as lazytask does
// for "+". The other panels keep the sizes of the normal layout in g, which
// are not drawn.
func (g grid) zoomed(focus panelID, width, height int) grid {
	g.leftWidth, g.rightWidth = width, width
	switch focus {
	case panelBlank:
		g.blankHeight = height
	case panelPARA:
		g.paraHeight = height
	case panelRelated:
		g.relatedHeight = height
	case panelContent:
		g.contentHeight = height
	case panelInfo:
		g.infoHeight = height
	}
	return g
}

// computeGrid splits width x height into the two-column layout: the [1] pane,
// PARA and Related on the left; Title, Content and Info, top to bottom, on the
// right. Below compactHeight only PARA and Content are drawn. Related takes about
// a quarter of the height and Info a fixed five rows, PARA and Content take the rest.
// When pane [1] is focused, it expands to focusedBlankHeight.
func computeGrid(width, height int, focus panelID) grid {
	bh := blankHeight
	if focus == panelBlank {
		bh = focusedBlankHeight
	}
	g := grid{
		leftWidth:   width * leftWidthPercent / 100,
		blankHeight: bh,
	}
	g.rightWidth = width - g.leftWidth

	if height < compactHeight {
		g.blankHeight = 0
		g.compact = true
		g.paraHeight, g.contentHeight = height, height
		return g
	}

	side := max(height/4, minSideHeight)
	g.relatedHeight = side
	if focus == panelBlank {
		maxBH := max(height-g.relatedHeight-minSideHeight, blankHeight)
		g.blankHeight = min(focusedBlankHeight, maxBH)
	}
	g.paraHeight = max(height-g.blankHeight-g.relatedHeight, minSideHeight)
	g.titleHeight = titleHeight
	g.infoHeight = infoHeight
	g.contentHeight = max(height-g.titleHeight-g.infoHeight, minSideHeight)
	return g
}
