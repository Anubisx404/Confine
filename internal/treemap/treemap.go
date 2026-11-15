package treemap

import (
	"confine/internal/analyzer"
	"confine/internal/indexer"
	"fmt"
	"math"
	"sort"
	"strings"
)

type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type LayoutNode struct {
	ID           uint32            `json:"id"`
	ParentID     uint32            `json:"parentId"`
	Name         string            `json:"name"`
	RelPath      string            `json:"relPath"`
	IsDir        bool              `json:"isDir"`
	Depth        int               `json:"depth"`
	Bounds       Rect              `json:"bounds"`
	ContentRect  Rect              `json:"contentRect"`
	Z            float64           `json:"z"`
	Height       float64           `json:"height"`
	Color        string            `json:"color"`
	ColorRGBA    [4]uint8          `json:"colorRgba"`
	ModuleName   string            `json:"moduleName"`
	Weight       float64           `json:"weight"`
	Stats        indexer.FileStats `json:"stats"`
	Symbols      []analyzer.Symbol `json:"symbols,omitempty"`
	ImportCount  int               `json:"importCount"`
	PreviewLines []int             `json:"previewLines,omitempty"`
}

type LayoutResult struct {
	Preset     string                `json:"preset"`
	CanvasRect Rect                  `json:"canvasRect"`
	Nodes      []LayoutNode          `json:"nodes"`
	ByPath     map[string]LayoutNode `json:"-"`
}

type workNode struct {
	src        *indexer.Node
	weight     float64
	bounds     Rect
	content    Rect
	depth      int
	moduleName string
	colorRGBA  [4]uint8
	colorHex   string
	children   []*workNode
}

var modulePalette = [][4]uint8{
	{0x3b, 0x82, 0xf6, 255},
	{0x14, 0xb8, 0xa6, 255},
	{0xf5, 0x9e, 0x0b, 255},
	{0xec, 0x48, 0x99, 255},
	{0x8b, 0x5c, 0xf6, 255},
	{0x10, 0xb9, 0x81, 255},
	{0xf9, 0x73, 0x16, 255},
	{0x06, 0xb6, 0xd4, 255},
	{0x84, 0xcc, 0x16, 255},
	{0x63, 0x66, 0xf1, 255},
	{0xef, 0x44, 0x44, 255},
	{0xd9, 0x46, 0xef, 255},
	{0x0e, 0xa5, 0xe9, 255},
	{0xea, 0xb3, 0x08, 255},
	{0x22, 0xc5, 0x5e, 255},
	{0xa8, 0x55, 0xf7, 255},
}

func ComputeLayout(tree *indexer.TreeResult, preset string) *LayoutResult {
	normPreset := normalizePreset(preset)
	canvas := Rect{X: 0, Y: 0, W: 1000, H: 650}

	if tree == nil || tree.Root == nil {
		return &LayoutResult{
			Preset:     normPreset,
			CanvasRect: canvas,
			Nodes:      []LayoutNode{},
			ByPath:     map[string]LayoutNode{},
		}
	}

	rootWork := buildWorkTree(tree.Root, normPreset, 0)
	assignModuleColors(rootWork)
	rootWork.bounds = canvas
	layoutSubtree(rootWork)

	nodes := make([]LayoutNode, 0, countWorkNodes(rootWork))
	byPath := make(map[string]LayoutNode, cap(nodes))
	flattenPreOrder(rootWork, &nodes, byPath)

	return &LayoutResult{
		Preset:     normPreset,
		CanvasRect: canvas,
		Nodes:      nodes,
		ByPath:     byPath,
	}
}

func normalizePreset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "bytes":
		return "bytes"
	case "symbols":
		return "symbols"
	default:
		return "sloc"
	}
}

func fileWeight(n *indexer.Node, preset string) float64 {
	switch preset {
	case "bytes":
		w := float64(n.Stats.ByteSize)
		if w < 64 {
			return 64
		}
		return w
	case "symbols":
		w := float64(n.Stats.SymbolCount)*10 + float64(n.Stats.SourceLineCount)
		if w < 1 {
			return 1
		}
		return w
	default:
		w := float64(n.Stats.SourceLineCount)
		if w < 1 {
			return 1
		}
		return w
	}
}

func buildWorkTree(n *indexer.Node, preset string, depth int) *workNode {
	wn := &workNode{
		src:   n,
		depth: depth,
	}

	if !n.IsDir {
		wn.weight = fileWeight(n, preset)
		return wn
	}

	if len(n.Children) > 0 {
		wn.children = make([]*workNode, 0, len(n.Children))
		var sum float64
		for _, ch := range n.Children {
			if ch == nil {
				continue
			}
			childWork := buildWorkTree(ch, preset, depth+1)
			wn.children = append(wn.children, childWork)
			sum += childWork.weight
		}
		sort.SliceStable(wn.children, func(i, j int) bool {
			a, b := wn.children[i], wn.children[j]
			if a.weight != b.weight {
				return a.weight > b.weight
			}
			if a.src.IsDir != b.src.IsDir {
				return a.src.IsDir
			}
			if a.src.Name != b.src.Name {
				return a.src.Name < b.src.Name
			}
			return a.src.RelPath < b.src.RelPath
		})
		if sum < 1 {
			sum = 1
		}
		wn.weight = sum
		return wn
	}

	wn.weight = 1
	return wn
}

func assignModuleColors(root *workNode) {
	root.moduleName = root.src.Name
	root.colorRGBA = [4]uint8{0x1b, 0x20, 0x2b, 255}
	root.colorHex = rgbaToHex(root.colorRGBA)

	if len(root.children) == 0 {
		return
	}

	alphaOrder := make([]*workNode, len(root.children))
	copy(alphaOrder, root.children)
	sort.SliceStable(alphaOrder, func(i, j int) bool {
		if alphaOrder[i].src.Name != alphaOrder[j].src.Name {
			return alphaOrder[i].src.Name < alphaOrder[j].src.Name
		}
		return alphaOrder[i].src.RelPath < alphaOrder[j].src.RelPath
	})

	for idx, mod := range alphaOrder {
		baseRGBA := moduleColorForIndex(idx)
		mod.moduleName = mod.src.Name
		mod.colorRGBA = baseRGBA
		mod.colorHex = rgbaToHex(baseRGBA)
		for _, ch := range mod.children {
			propagateColor(ch, mod.moduleName, baseRGBA)
		}
	}
}

func propagateColor(wn *workNode, moduleName string, baseRGBA [4]uint8) {
	wn.moduleName = moduleName
	h, s, l := rgbToHSL(baseRGBA[0], baseRGBA[1], baseRGBA[2])
	depthStep := float64(wn.depth - 1)
	if depthStep < 0 {
		depthStep = 0
	}

	var nl, ns float64
	if wn.src.IsDir {
		nl = clampFloat(l-0.07-depthStep*0.015, 0.20, 0.54)
		ns = clampFloat(s-depthStep*0.04, 0.28, 0.85)
	} else {
		nl = clampFloat(l+0.03+depthStep*0.02, 0.34, 0.74)
		ns = clampFloat(s-depthStep*0.025, 0.32, 0.88)
	}

	wn.colorRGBA = hslToRGBA(h, ns, nl)
	wn.colorHex = rgbaToHex(wn.colorRGBA)

	for _, ch := range wn.children {
		propagateColor(ch, moduleName, baseRGBA)
	}
}

func moduleColorForIndex(idx int) [4]uint8 {
	if idx >= 0 && idx < len(modulePalette) {
		return modulePalette[idx]
	}
	h := math.Mod(float64(idx)*137.508+24.0, 360.0)
	return hslToRGBA(h, 0.68, 0.54)
}

func rgbaToHex(c [4]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func rgbToHSL(r8, g8, b8 uint8) (float64, float64, float64) {
	r := float64(r8) / 255.0
	g := float64(g8) / 255.0
	b := float64(b8) / 255.0

	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	l := (maxC + minC) / 2.0

	if maxC == minC {
		return 0, 0, l
	}

	d := maxC - minC
	var s float64
	if l > 0.5 {
		s = d / (2.0 - maxC - minC)
	} else {
		s = d / (maxC + minC)
	}

	var h float64
	switch maxC {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6.0
		}
	case g:
		h = (b-r)/d + 2.0
	case b:
		h = (r-g)/d + 4.0
	}
	h *= 60.0
	return h, s, l
}

func hslToRGBA(h, s, l float64) [4]uint8 {
	h = math.Mod(h, 360.0)
	if h < 0 {
		h += 360.0
	}
	if s <= 0 {
		v := uint8(math.Round(clampFloat(l, 0, 1) * 255.0))
		return [4]uint8{v, v, v, 255}
	}

	var q float64
	if l < 0.5 {
		q = l * (1.0 + s)
	} else {
		q = l + s - l*s
	}
	p := 2.0*l - q
	hk := h / 360.0

	r := hueToRGB(p, q, hk+1.0/3.0)
	g := hueToRGB(p, q, hk)
	b := hueToRGB(p, q, hk-1.0/3.0)

	return [4]uint8{
		uint8(math.Round(clampFloat(r, 0, 1) * 255.0)),
		uint8(math.Round(clampFloat(g, 0, 1) * 255.0)),
		uint8(math.Round(clampFloat(b, 0, 1) * 255.0)),
		255,
	}
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1.0
	}
	if t > 1 {
		t -= 1.0
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6.0*t
	}
	if t < 0.5 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6.0
	}
	return p
}

func computeContentRect(b Rect, isDir bool, depth int) Rect {
	if !isDir || b.W <= 0 || b.H <= 0 {
		return b
	}

	padX := math.Min(4.0, b.W*0.03)
	if b.W > 12 && padX < 1.5 {
		padX = 1.5
	}
	if padX*2 >= b.W {
		padX = b.W * 0.05
	}

	padBottom := math.Min(4.0, b.H*0.03)
	if b.H > 12 && padBottom < 1.5 {
		padBottom = 1.5
	}

	headerH := math.Min(18.0, math.Max(10.0, b.H*0.06))
	if depth == 0 {
		headerH = math.Min(22.0, b.H*0.06)
	}
	if headerH+padBottom >= b.H*0.45 {
		headerH = b.H * 0.22
		padBottom = b.H * 0.05
	}

	cw := b.W - 2*padX
	ch := b.H - headerH - padBottom
	if cw <= 0 {
		padX = 0
		cw = b.W
	}
	if ch <= 0 {
		headerH = 0
		ch = b.H
	}

	return Rect{
		X: b.X + padX,
		Y: b.Y + headerH,
		W: cw,
		H: ch,
	}
}

func layoutSubtree(wn *workNode) {
	wn.content = computeContentRect(wn.bounds, wn.src.IsDir, wn.depth)
	if !wn.src.IsDir || len(wn.children) == 0 {
		return
	}

	squarify(wn.children, wn.content)
	for _, ch := range wn.children {
		layoutSubtree(ch)
	}
}

func squarify(items []*workNode, rect Rect) {
	if len(items) == 0 || rect.W <= 0 || rect.H <= 0 {
		return
	}
	if len(items) == 1 {
		items[0].bounds = rect
		return
	}

	var totalWeight float64
	for _, it := range items {
		totalWeight += it.weight
	}
	if totalWeight <= 0 {
		return
	}

	rem := rect
	remWeight := totalWeight
	start := 0

	for start < len(items) {
		if rem.W <= 0 || rem.H <= 0 {
			break
		}
		if start == len(items)-1 {
			items[start].bounds = rem
			break
		}

		shortSide := math.Min(rem.W, rem.H)
		scale := (rem.W * rem.H) / remWeight
		rowWeight := items[start].weight
		rowEnd := start + 1
		curWorst := worstRatio(items[start].weight, items[start].weight, rowWeight, shortSide, scale)

		for rowEnd < len(items) {
			nextWeight := rowWeight + items[rowEnd].weight
			nextWorst := worstRatio(items[start].weight, items[rowEnd].weight, nextWeight, shortSide, scale)
			if nextWorst > curWorst {
				break
			}
			rowWeight = nextWeight
			curWorst = nextWorst
			rowEnd++
		}

		isLastRow := rowEnd == len(items)
		row := items[start:rowEnd]

		if rem.W >= rem.H {
			endX := rem.X + rem.W
			var colEnd float64
			if isLastRow {
				colEnd = endX
			} else {
				colW := rem.W * (rowWeight / remWeight)
				maxColW := rem.W * (1.0 - 1e-9)
				if colW > maxColW {
					colW = maxColW
				}
				colEnd = rem.X + colW
				if colEnd > endX {
					colEnd = endX
				}
			}
			colW := colEnd - rem.X

			endY := rem.Y + rem.H
			curY := rem.Y
			var accW float64
			for i, it := range row {
				var itemEnd float64
				if i == len(row)-1 {
					itemEnd = endY
				} else {
					accW += it.weight
					itemEnd = rem.Y + rem.H*(accW/rowWeight)
					if itemEnd > endY {
						itemEnd = endY
					}
				}
				it.bounds = Rect{
					X: rem.X,
					Y: curY,
					W: colW,
					H: itemEnd - curY,
				}
				curY = itemEnd
			}

			rem.X = colEnd
			rem.W = endX - colEnd
		} else {
			endY := rem.Y + rem.H
			var rowEndY float64
			if isLastRow {
				rowEndY = endY
			} else {
				rowH := rem.H * (rowWeight / remWeight)
				maxRowH := rem.H * (1.0 - 1e-9)
				if rowH > maxRowH {
					rowH = maxRowH
				}
				rowEndY = rem.Y + rowH
				if rowEndY > endY {
					rowEndY = endY
				}
			}
			rowH := rowEndY - rem.Y

			endX := rem.X + rem.W
			curX := rem.X
			var accW float64
			for i, it := range row {
				var itemEnd float64
				if i == len(row)-1 {
					itemEnd = endX
				} else {
					accW += it.weight
					itemEnd = rem.X + rem.W*(accW/rowWeight)
					if itemEnd > endX {
						itemEnd = endX
					}
				}
				it.bounds = Rect{
					X: curX,
					Y: rem.Y,
					W: itemEnd - curX,
					H: rowH,
				}
				curX = itemEnd
			}

			rem.Y = rowEndY
			rem.H = endY - rowEndY
		}

		remWeight -= rowWeight
		start = rowEnd
	}
}

func worstRatio(maxW, minW, rowWeight, shortSide, scale float64) float64 {
	if rowWeight <= 0 || shortSide <= 0 || scale <= 0 || minW <= 0 {
		return math.Inf(1)
	}
	s := rowWeight * scale
	rMax := maxW * scale
	rMin := minW * scale
	w2 := shortSide * shortSide
	s2 := s * s
	a := (w2 * rMax) / s2
	b := s2 / (w2 * rMin)
	if a > b {
		return a
	}
	return b
}

func computeHeight(isDir bool, sloc int) float64 {
	if isDir {
		return 1.2
	}
	if sloc < 0 {
		sloc = 0
	}
	return 1.6 + 1.8*math.Log2(float64(sloc)+1.0)
}

func samplePreviewLines(lines []string) []int {
	if len(lines) == 0 {
		return nil
	}
	const maxSamples = 64
	n := len(lines)
	if n <= maxSamples {
		out := make([]int, n)
		for i, line := range lines {
			out[i] = normalizedLineLength(line)
		}
		return out
	}
	out := make([]int, maxSamples)
	for i := 0; i < maxSamples; i++ {
		idx := (i * (n - 1)) / (maxSamples - 1)
		out[i] = normalizedLineLength(lines[idx])
	}
	return out
}

func normalizedLineLength(line string) int {
	trimmed := strings.TrimRight(line, "\r\n")
	length := len(trimmed)
	if length <= 0 {
		return 0
	}
	if length > 100 {
		return 100
	}
	return length
}

func countWorkNodes(wn *workNode) int {
	total := 1
	for _, ch := range wn.children {
		total += countWorkNodes(ch)
	}
	return total
}

func flattenPreOrder(wn *workNode, out *[]LayoutNode, byPath map[string]LayoutNode) {
	var preview []int
	if !wn.src.IsDir {
		preview = samplePreviewLines(wn.src.Lines)
	}

	var syms []analyzer.Symbol
	if len(wn.src.Analysis.Symbols) > 0 {
		syms = make([]analyzer.Symbol, len(wn.src.Analysis.Symbols))
		copy(syms, wn.src.Analysis.Symbols)
	}

	ln := LayoutNode{
		ID:           wn.src.ID,
		ParentID:     wn.src.ParentID,
		Name:         wn.src.Name,
		RelPath:      wn.src.RelPath,
		IsDir:        wn.src.IsDir,
		Depth:        wn.depth,
		Bounds:       wn.bounds,
		ContentRect:  wn.content,
		Z:            float64(wn.depth) * 1.4,
		Height:       computeHeight(wn.src.IsDir, wn.src.Stats.SourceLineCount),
		Color:        wn.colorHex,
		ColorRGBA:    wn.colorRGBA,
		ModuleName:   wn.moduleName,
		Weight:       wn.weight,
		Stats:        wn.src.Stats,
		Symbols:      syms,
		ImportCount:  wn.src.Stats.ImportCount,
		PreviewLines: preview,
	}

	*out = append(*out, ln)
	byPath[ln.RelPath] = ln

	for _, ch := range wn.children {
		flattenPreOrder(ch, out, byPath)
	}
}
