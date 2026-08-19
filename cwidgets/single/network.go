package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// NetworkInfo represents interface configuration for a network
type NetworkInfo struct {
	Name    string
	IP      string
	Gateway string
	MAC     string
	Subnet  string
}

// Network widget displays container network adapters, addresses, and port mappings.
type Network struct {
	ui.Block
	Networks []NetworkInfo
	Ports    string
	IPs      string
}

// NewNetwork constructs a new Network inspection widget.
func NewNetwork() *Network {
	nw := &Network{
		Block:    *ui.NewBlock(),
		Networks: []NetworkInfo{},
	}
	nw.Title = "NETWORKING & PORTS"
	nw.BorderStyle = theme.Style("border.fg")
	nw.TitleStyle = theme.Style("label.fg")
	nw.SetRect(0, 0, colWidth[0], 6)
	return nw
}

// Set parses the serialized networks and port information
func (w *Network) Set(networkStr, portsStr, ipsStr string) {
	w.Ports = portsStr
	w.IPs = ipsStr
	w.Networks = []NetworkInfo{}

	if strings.TrimSpace(networkStr) != "" {
		entries := strings.Split(networkStr, ";;")
		for _, entry := range entries {
			if strings.TrimSpace(entry) == "" {
				continue
			}
			parts := strings.Split(entry, ":::")
			n := NetworkInfo{
				Name: parts[0],
			}
			if len(parts) > 1 {
				n.IP = parts[1]
			}
			if len(parts) > 2 {
				n.Gateway = parts[2]
			}
			if len(parts) > 3 {
				n.MAC = parts[3]
			}
			if len(parts) > 4 {
				n.Subnet = parts[4]
			}
			w.Networks = append(w.Networks, n)
		}
	}
}

// GetHeight calculates required widget height
func (w *Network) GetHeight() int {
	h := 3 // title + borders
	if len(w.Networks) > 0 {
		h += len(w.Networks) + 2 // header + rows
	} else if w.IPs != "" {
		h += len(strings.Split(w.IPs, "\n")) + 2
	} else {
		h += 2
	}

	if w.Ports != "" {
		h += len(strings.Split(w.Ports, "\n")) + 2
	} else {
		h += 2
	}
	return h
}

// Draw renders formatted network adapters and port tables
func (w *Network) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	keyStyle := theme.Style("header.fg")
	valStyle := theme.Style("par.text.fg")
	subHeaderStyle := theme.Style("status.warn")

	y := w.Inner.Min.Y

	// Section 1: Attached Networks
	buf.SetString("[ Attached Networks ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	if len(w.Networks) > 0 {
		header := fmt.Sprintf("%-18s %-18s %-18s %s", "NETWORK", "IP ADDRESS", "GATEWAY", "MAC ADDRESS")
		buf.SetString(header, headerStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		for _, n := range w.Networks {
			if y >= w.Inner.Max.Y {
				break
			}
			buf.SetString(fmt.Sprintf("%-18s", n.Name), keyStyle, image.Pt(w.Inner.Min.X+1, y))
			buf.SetString(fmt.Sprintf("%-18s", n.IP), valStyle, image.Pt(w.Inner.Min.X+20, y))
			buf.SetString(fmt.Sprintf("%-18s", n.Gateway), valStyle, image.Pt(w.Inner.Min.X+39, y))
			buf.SetString(n.MAC, valStyle, image.Pt(w.Inner.Min.X+58, y))
			y++
		}
	} else if strings.TrimSpace(w.IPs) != "" {
		lines := strings.Split(w.IPs, "\n")
		for _, l := range lines {
			if y >= w.Inner.Max.Y {
				break
			}
			buf.SetString(l, valStyle, image.Pt(w.Inner.Min.X+2, y))
			y++
		}
	} else {
		buf.SetString("No network interfaces attached (e.g. host mode or isolated).", valStyle, image.Pt(w.Inner.Min.X+2, y))
		y++
	}

	y++ // gap

	// Section 2: Port Bindings & Forwarding
	if y < w.Inner.Max.Y {
		buf.SetString("[ Port Mappings & Forwarding ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		if strings.TrimSpace(w.Ports) != "" {
			ports := strings.Split(w.Ports, "\n")
			for _, p := range ports {
				if y >= w.Inner.Max.Y {
					break
				}
				buf.SetString("• "+p, valStyle, image.Pt(w.Inner.Min.X+2, y))
				y++
			}
		} else {
			buf.SetString("No ports exposed or published to host.", valStyle, image.Pt(w.Inner.Min.X+2, y))
			y++
		}
	}
}
