// cursor.go coordinates user navigation, selection highlighting, and viewport pagination across container rows.
package main

import (
	"math"
	"sync"

	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/container"
)

// GridCursor manages the index and ID of the active container row and manages vertical scrolling.
type GridCursor struct {
	mu          sync.RWMutex
	selectedID  string                    // ID of currently selected container
	filtered    container.Containers      // Active list of containers matching current filter and display flags
	cSuper      *connector.ConnectorSuper // Connector supervisor providing continuous container discovery
	isScrolling bool                      // Flag indicating active user scrolling to temporarily suppress background redraws
}

// Len returns the count of visible/filtered containers.
func (gc *GridCursor) Len() int {
	if gc == nil {
		return 0
	}
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return len(gc.filtered)
}

// Filtered returns a shallow copy of the filtered containers slice under read lock.
func (gc *GridCursor) Filtered() container.Containers {
	if gc == nil {
		return nil
	}
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	res := make(container.Containers, len(gc.filtered))
	copy(res, gc.filtered)
	return res
}

// Selected returns the container currently highlighted by the cursor, or nil if none.
func (gc *GridCursor) Selected() *container.Container {
	if gc == nil {
		return nil
	}
	gc.mu.Lock()
	defer gc.mu.Unlock()
	idx := gc.idxLocked()
	if idx < len(gc.filtered) {
		return gc.filtered[idx]
	}
	return nil
}

// RefreshContainers refreshes containers from source, returning whether the quantity of
// containers has changed and any error
func (gc *GridCursor) RefreshContainers() (bool, error) {
	if gc == nil || gc.cSuper == nil {
		return false, nil
	}
	gc.mu.Lock()
	defer gc.mu.Unlock()

	oldLen := len(gc.filtered)
	gc.filtered = container.Containers{}

	cSource, err := gc.cSuper.Get()
	if err != nil {
		return true, err
	}

	// filter Containers by display bool
	var cursorVisible bool
	for _, c := range cSource.All() {
		if c.Display {
			if c.Id == gc.selectedID {
				cursorVisible = true
			}
			gc.filtered = append(gc.filtered, c)
		}
	}
	gc.filtered.Sort()

	if !cursorVisible || gc.selectedID == "" {
		gc.resetLocked()
	}

	return oldLen != len(gc.filtered), nil
}

// Reset sets an initial cursor position, if possible
func (gc *GridCursor) Reset() {
	if gc == nil {
		return
	}
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.resetLocked()
}

func (gc *GridCursor) resetLocked() {
	if gc.cSuper != nil {
		cSource, err := gc.cSuper.Get()
		if err == nil {
			for _, c := range cSource.All() {
				if c != nil && c.Widgets != nil {
					c.Widgets.UnHighlight()
				}
			}
		}
	} else {
		for _, c := range gc.filtered {
			if c != nil && c.Widgets != nil {
				c.Widgets.UnHighlight()
			}
		}
	}
	if len(gc.filtered) > 0 && gc.filtered[0] != nil {
		gc.selectedID = gc.filtered[0].Id
		if gc.filtered[0].Widgets != nil {
			gc.filtered[0].Widgets.Highlight()
		}
	}
}

// Idx returns current cursor index
func (gc *GridCursor) Idx() int {
	if gc == nil {
		return 0
	}
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.idxLocked()
}

func (gc *GridCursor) idxLocked() int {
	for n, c := range gc.filtered {
		if c.Id == gc.selectedID {
			return n
		}
	}
	gc.resetLocked()
	return 0
}

func (gc *GridCursor) ScrollPage() {
	if cGrid == nil {
		return
	}
	// skip scroll if no need to page
	if gc.Len() < cGrid.MaxRows() {
		cGrid.Offset = 0
		return
	}

	idx := gc.Idx()

	// page down
	if idx >= cGrid.Offset+cGrid.MaxRows() {
		cGrid.Offset++
	}
	// page up
	if idx < cGrid.Offset {
		cGrid.Offset--
	}
}

func (gc *GridCursor) Up() {
	gc.isScrolling = true
	defer func() { gc.isScrolling = false }()

	idx := gc.Idx()
	if idx <= 0 { // already at top
		return
	}
	active := gc.filtered[idx]
	next := gc.filtered[idx-1]

	active.Widgets.UnHighlight()
	gc.selectedID = next.Id
	next.Widgets.Highlight()

	gc.ScrollPage()
	RedrawRows(false)
}

func (gc *GridCursor) Down() {
	gc.isScrolling = true
	defer func() { gc.isScrolling = false }()

	idx := gc.Idx()
	if idx >= gc.Len()-1 { // already at bottom
		return
	}
	active := gc.filtered[idx]
	next := gc.filtered[idx+1]

	active.Widgets.UnHighlight()
	gc.selectedID = next.Id
	next.Widgets.Highlight()

	gc.ScrollPage()
	RedrawRows(false)
}

func (gc *GridCursor) PgUp() {
	idx := gc.Idx()
	if idx <= 0 { // already at top
		return
	}

	maxRows := 10
	if cGrid != nil {
		maxRows = cGrid.MaxRows()
	}
	nextidx := int(math.Max(0.0, float64(idx-maxRows)))
	if cGrid != nil && gc.pgCount() > 0 {
		cGrid.Offset = int(math.Max(float64(cGrid.Offset-maxRows), float64(0)))
	}

	active := gc.filtered[idx]
	next := gc.filtered[nextidx]

	active.Widgets.UnHighlight()
	gc.selectedID = next.Id
	next.Widgets.Highlight()

	RedrawRows(false)
}

func (gc *GridCursor) PgDown() {
	idx := gc.Idx()
	if idx >= gc.Len()-1 { // already at bottom
		return
	}

	maxRows := 10
	if cGrid != nil {
		maxRows = cGrid.MaxRows()
	}
	nextidx := int(math.Min(float64(gc.Len()-1), float64(idx+maxRows)))
	if cGrid != nil && gc.pgCount() > 0 {
		cGrid.Offset = int(math.Min(float64(cGrid.Offset+maxRows), float64(gc.Len()-maxRows)))
	}

	active := gc.filtered[idx]
	next := gc.filtered[nextidx]

	active.Widgets.UnHighlight()
	gc.selectedID = next.Id
	next.Widgets.Highlight()

	RedrawRows(false)
}

// number of pages at current row count and term height
func (gc *GridCursor) pgCount() int {
	if cGrid == nil {
		return 1
	}
	maxRows := cGrid.MaxRows()
	if maxRows <= 0 {
		return 1
	}
	pages := gc.Len() / maxRows
	if gc.Len()%maxRows > 0 {
		pages++
	}
	return pages
}
