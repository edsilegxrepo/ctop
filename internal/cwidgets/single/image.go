package single

import (
	"fmt"
	"image"
	"sort"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/sanitize"
	ui "github.com/gizak/termui/v3"
)

// ImageInfo holds all container image metadata
type ImageInfo struct {
	ID            string
	RepoTags      string
	RepoDigests   string
	Architecture  string
	Author        string
	Created       string
	DockerVersion string
	Size          string
	Layers        string
	LayerList     []string
	Entrypoint    string
	Cmd           string
	WorkingDir    string
	User          string
	Env           []string
	ExposedPorts  string
	Volumes       string
	Labels        [][]string
}

type imageRow struct {
	isHeader bool
	key      string
	val      string
}

// Image widget displays container base image metadata, execution defaults, and build layers.
type Image struct {
	ui.Block
	Info   ImageInfo
	Offset int
	empty  bool
}

// NewImage constructs a new Image inspection widget.
func NewImage() *Image {
	im := &Image{
		Block:  *ui.NewBlock(),
		Offset: 0,
		empty:  true,
	}
	im.Title = "IMAGE INFORMATION"
	im.BorderStyle = theme.Style("border.fg")
	im.TitleStyle = theme.Style("label.fg")
	im.SetRect(0, 0, colWidth[0], 6)
	return im
}

// Set populates image details from container metadata
func (w *Image) Set(m map[string]string) {
	w.Info = ImageInfo{
		ID:            m["imageId"],
		RepoTags:      m["imageRepoTags"],
		RepoDigests:   m["imageRepoDigests"],
		Architecture:  m["imageArch"],
		Author:        m["imageAuthor"],
		Created:       m["imageCreated"],
		DockerVersion: m["imageDockerVersion"],
		Size:          m["imageSize"],
		Layers:        m["imageLayers"],
		Entrypoint:    m["imageEntrypoint"],
		Cmd:           m["imageCmd"],
		WorkingDir:    m["imageWorkdir"],
		User:          m["imageUser"],
		ExposedPorts:  m["imageExposedPorts"],
		Volumes:       m["imageVolumes"],
	}

	if w.Info.RepoTags == "" && m["image"] != "" {
		w.Info.RepoTags = m["image"]
	}

	if m["imageLayerList"] != "" {
		w.Info.LayerList = strings.Split(m["imageLayerList"], "\n")
	}

	if m["imageEnv"] != "" {
		w.Info.Env = strings.Split(m["imageEnv"], ";;")
	}

	if m["imageLabels"] != "" {
		var labels [][]string
		for _, entry := range strings.Split(m["imageLabels"], ";;") {
			if strings.TrimSpace(entry) == "" {
				continue
			}
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) == 2 {
				labels = append(labels, []string{parts[0], parts[1]})
			}
		}
		sort.Slice(labels, func(i, j int) bool {
			return labels[i][0] < labels[j][0]
		})
		w.Info.Labels = labels
	}

	w.empty = (w.Info.ID == "" && w.Info.RepoTags == "")
	w.Offset = 0
}

func (w *Image) buildRows() []imageRow {
	if w.empty {
		return nil
	}

	var rows []imageRow

	// Section 1: Image Identity & Platform
	rows = append(rows, imageRow{isHeader: true, key: "[ Image Identity & Build Platform ]"})
	if w.Info.RepoTags != "" {
		rows = append(rows, imageRow{key: "Tags / Repository", val: w.Info.RepoTags})
	}
	if w.Info.ID != "" {
		rows = append(rows, imageRow{key: "Image ID", val: w.Info.ID})
	}
	if w.Info.RepoDigests != "" {
		rows = append(rows, imageRow{key: "Repo Digest", val: w.Info.RepoDigests})
	}
	if w.Info.Architecture != "" {
		rows = append(rows, imageRow{key: "Platform / OS", val: w.Info.Architecture})
	}
	if w.Info.Size != "" {
		rows = append(rows, imageRow{key: "Virtual Size", val: w.Info.Size})
	}
	if w.Info.Created != "" {
		rows = append(rows, imageRow{key: "Build Created", val: w.Info.Created})
	}
	if w.Info.Author != "" {
		rows = append(rows, imageRow{key: "Author / Maintainer", val: w.Info.Author})
	}
	if w.Info.DockerVersion != "" {
		rows = append(rows, imageRow{key: "Engine Builder", val: w.Info.DockerVersion})
	}
	if w.Info.Layers != "" {
		rows = append(rows, imageRow{key: "RootFS Layers", val: w.Info.Layers})
	}

	rows = append(rows, imageRow{}) // blank line

	// Section 2: Container Defaults & Config
	rows = append(rows, imageRow{isHeader: true, key: "[ Image Execution Defaults ]"})
	if w.Info.Entrypoint != "" {
		rows = append(rows, imageRow{key: "ENTRYPOINT", val: w.Info.Entrypoint})
	} else {
		rows = append(rows, imageRow{key: "ENTRYPOINT", val: "none (default)"})
	}
	if w.Info.Cmd != "" {
		rows = append(rows, imageRow{key: "CMD", val: w.Info.Cmd})
	} else {
		rows = append(rows, imageRow{key: "CMD", val: "none (default)"})
	}
	if w.Info.WorkingDir != "" {
		rows = append(rows, imageRow{key: "WORKDIR", val: w.Info.WorkingDir})
	}
	if w.Info.User != "" {
		rows = append(rows, imageRow{key: "USER", val: w.Info.User})
	}
	if w.Info.ExposedPorts != "" {
		rows = append(rows, imageRow{key: "EXPOSE Ports", val: w.Info.ExposedPorts})
	}
	if w.Info.Volumes != "" {
		rows = append(rows, imageRow{key: "VOLUME Mounts", val: w.Info.Volumes})
	}

	// Section 3: Base Environment Variables
	if len(w.Info.Env) > 0 {
		rows = append(rows, imageRow{}) // blank line
		rows = append(rows, imageRow{isHeader: true, key: "[ Base Image Environment Variables ]"})
		for _, envVar := range w.Info.Env {
			parts := strings.SplitN(envVar, "=", 2)
			k := parts[0]
			v := ""
			if len(parts) > 1 {
				v = parts[1]
			}
			if sanitize.IsSensitiveKey(k) && len(v) > 0 {
				v = "•••••••••••• [masked]"
			}
			rows = append(rows, imageRow{key: k, val: v})
		}
	}

	// Section 4: Image Labels & OCI Annotations
	if len(w.Info.Labels) > 0 {
		rows = append(rows, imageRow{}) // blank line
		rows = append(rows, imageRow{isHeader: true, key: "[ Image Labels & OCI Annotations ]"})
		for _, lbl := range w.Info.Labels {
			rows = append(rows, imageRow{key: lbl[0], val: lbl[1]})
		}
	}

	return rows
}

// Up scrolls content up
func (w *Image) Up() {
	if w.Offset > 0 {
		w.Offset--
	}
}

// Down scrolls content down
func (w *Image) Down() {
	rows := w.buildRows()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}
	maxOffset := len(rows) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.Offset < maxOffset {
		w.Offset++
	}
}

// PgUp scrolls one page up
func (w *Image) PgUp() {
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset -= step
	if w.Offset < 0 {
		w.Offset = 0
	}
}

// PgDown scrolls one page down
func (w *Image) PgDown() {
	rows := w.buildRows()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}
	maxOffset := len(rows) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset += step
	if w.Offset > maxOffset {
		w.Offset = maxOffset
	}
}

// GetHeight calculates required widget height
func (w *Image) GetHeight() int {
	if w.empty {
		return 6
	}
	return len(w.buildRows()) + 2
}

// Draw renders formatted image information
func (w *Image) Draw(buf *ui.Buffer) {
	rows := w.buildRows()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}

	maxOffset := len(rows) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.Offset > maxOffset {
		w.Offset = maxOffset
	}

	if len(rows) > visibleH {
		endLine := w.Offset + visibleH
		if endLine > len(rows) {
			endLine = len(rows)
		}
		w.Title = fmt.Sprintf("IMAGE INFORMATION [%d-%d/%d | ▲▼/PgUp/PgDn]", w.Offset+1, endLine, len(rows))
	} else {
		w.Title = "IMAGE INFORMATION"
	}

	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	keyStyle := theme.Style("label.fg")
	valStyle := theme.Style("par.text.fg")
	subHeaderStyle := theme.Style("status.warn")

	y := w.Inner.Min.Y

	if w.empty {
		buf.SetString("No image details available for this container.", valStyle, image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

	// Calculate dynamic key column width
	maxKeyLen := 20
	for _, r := range rows {
		if !r.isHeader && r.key != "" && len(r.key) > maxKeyLen {
			maxKeyLen = len(r.key)
		}
	}

	col0Width := maxKeyLen + 2
	maxAllowedCol0 := (w.Inner.Max.X - w.Inner.Min.X) * 45 / 100
	if maxAllowedCol0 > 20 && col0Width > maxAllowedCol0 {
		col0Width = maxAllowedCol0
	}

	dividerX := w.Inner.Min.X + col0Width
	valX := dividerX + 2

	endIdx := w.Offset + visibleH
	if endIdx > len(rows) {
		endIdx = len(rows)
	}

	for idx := w.Offset; idx < endIdx; idx++ {
		if y >= w.Inner.Max.Y {
			break
		}
		r := rows[idx]
		if r.isHeader {
			buf.SetString(r.key, subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		} else if r.key != "" {
			maxKeyW := dividerX - w.Inner.Min.X - 2
			displayKey := r.key
			if maxKeyW > 0 && len(displayKey) > maxKeyW {
				displayKey = displayKey[:maxKeyW-1] + "…"
			}
			buf.SetString(displayKey, keyStyle, image.Pt(w.Inner.Min.X+2, y))
			buf.SetString("│", headerStyle, image.Pt(dividerX, y))

			maxValLen := w.Inner.Max.X - valX - 1
			displayVal := r.val
			if maxValLen > 0 && len(displayVal) > maxValLen {
				displayVal = displayVal[:maxValLen-1] + "…"
			}
			buf.SetString(displayVal, valStyle, image.Pt(valX, y))
		}
		y++
	}
}
