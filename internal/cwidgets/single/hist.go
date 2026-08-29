package single

type IntHist struct {
	limit  int
	Val    int   // most current data point
	Data   []int // historical data points
	Labels []string
}

func NewIntHist(max int) *IntHist {
	return &IntHist{
		limit:  max,
		Data:   make([]int, max),
		Labels: make([]string, max),
	}
}

func (h *IntHist) SetLimit(limit int) {
	if limit <= 0 {
		return
	}
	h.limit = limit
	if len(h.Data) > limit {
		h.Data = h.Data[len(h.Data)-limit:]
	} else if len(h.Data) < limit {
		padded := make([]int, limit-len(h.Data))
		h.Data = append(padded, h.Data...)
	}
}

func (h *IntHist) Append(val int) {
	if len(h.Data) >= h.limit && h.limit > 0 {
		h.Data = append(h.Data[len(h.Data)-h.limit+1:], val)
	} else {
		h.Data = append(h.Data, val)
	}
	h.Val = val
}

type DiffHist struct {
	*IntHist
	lastVal int
}

func NewDiffHist(max int) *DiffHist {
	return &DiffHist{NewIntHist(max), -1}
}

func (h *DiffHist) Append(val int) {
	if h.lastVal >= 0 { // skip append if this is the initial update
		diff := val - h.lastVal
		h.IntHist.Append(diff)
	}
	h.lastVal = val
}

type FloatHist struct {
	limit  int
	Val    float64   // most current data point
	Data   []float64 // historical data points
	Labels []string
}

func NewFloatHist(max int) FloatHist {
	return FloatHist{
		limit:  max,
		Data:   make([]float64, max),
		Labels: make([]string, max),
	}
}

func (h *FloatHist) SetLimit(limit int) {
	if limit <= 0 {
		return
	}
	h.limit = limit
	if len(h.Data) > limit {
		h.Data = h.Data[len(h.Data)-limit:]
	} else if len(h.Data) < limit {
		padded := make([]float64, limit-len(h.Data))
		h.Data = append(padded, h.Data...)
	}
}

func (h *FloatHist) Append(val float64) {
	if len(h.Data) >= h.limit && h.limit > 0 {
		h.Data = append(h.Data[len(h.Data)-h.limit+1:], val)
	} else {
		h.Data = append(h.Data, val)
	}
	h.Val = val
}
