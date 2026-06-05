package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
)

type FractionalLayout struct {
	Columns []string
	Rows    []string
	Gap     float32
}

type metricType int

const (
	metricFixed metricType = iota
	metricAuto
	metricFractional
)

type metricSpec struct {
	mType metricType
	value float32
}

func parseMetric(spec string) metricSpec {
	spec = strings.TrimSpace(spec)
	if spec == "auto" || spec == "" {
		return metricSpec{mType: metricAuto}
	}
	if strings.HasSuffix(spec, "fr") {
		valStr := strings.TrimSuffix(spec, "fr")
		val, err := strconv.ParseFloat(valStr, 32)
		if err != nil {
			return metricSpec{mType: metricAuto}
		}
		return metricSpec{mType: metricFractional, value: float32(val)}
	}
	valStr := strings.TrimSuffix(spec, "px")
	val, err := strconv.ParseFloat(valStr, 32)
	if err != nil {
		return metricSpec{mType: metricAuto}
	}
	return metricSpec{mType: metricFixed, value: float32(val)}
}

func (l *FractionalLayout) parseSpecs() ([]metricSpec, []metricSpec) {
	cols := make([]metricSpec, len(l.Columns))
	for i, c := range l.Columns {
		cols[i] = parseMetric(c)
	}
	if len(cols) == 0 {
		cols = []metricSpec{{mType: metricFractional, value: 1}}
	}

	rows := make([]metricSpec, len(l.Rows))
	for i, r := range l.Rows {
		rows[i] = parseMetric(r)
	}
	if len(rows) == 0 {
		rows = []metricSpec{{mType: metricFractional, value: 1}}
	}

	return cols, rows
}

func (l *FractionalLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}

	cols, rows := l.parseSpecs()
	colsCount := len(cols)
	rowsCount := len(rows)

	colMinW := make([]float32, colsCount)
	rowMinH := make([]float32, rowsCount)

	for i, obj := range objects {
		if obj == nil || !obj.Visible() {
			continue
		}
		c := i % colsCount
		r := i / colsCount

		if r >= rowsCount {
			continue
		}

		minS := obj.MinSize()
		if minS.Width > colMinW[c] {
			colMinW[c] = minS.Width
		}
		if minS.Height > rowMinH[r] {
			rowMinH[r] = minS.Height
		}
	}

	colWidths := make([]float32, colsCount)
	maxFrColUnit := float32(0)
	for c, spec := range cols {
		switch spec.mType {
		case metricFixed:
			colWidths[c] = spec.value
		case metricAuto:
			colWidths[c] = colMinW[c]
		case metricFractional:
			if spec.value > 0 {
				unit := colMinW[c] / spec.value
				if unit > maxFrColUnit {
					maxFrColUnit = unit
				}
			}
		}
	}
	for c, spec := range cols {
		if spec.mType == metricFractional {
			colWidths[c] = maxFrColUnit * spec.value
		}
	}

	rowHeights := make([]float32, rowsCount)
	maxFrRowUnit := float32(0)
	for r, spec := range rows {
		switch spec.mType {
		case metricFixed:
			rowHeights[r] = spec.value
		case metricAuto:
			rowHeights[r] = rowMinH[r]
		case metricFractional:
			if spec.value > 0 {
				unit := rowMinH[r] / spec.value
				if unit > maxFrRowUnit {
					maxFrRowUnit = unit
				}
			}
		}
	}
	for r, spec := range rows {
		if spec.mType == metricFractional {
			rowHeights[r] = maxFrRowUnit * spec.value
		}
	}

	var totalW float32
	for _, w := range colWidths {
		totalW += w
	}
	if colsCount > 1 {
		totalW += float32(colsCount-1) * l.Gap
	}

	var totalH float32
	for _, h := range rowHeights {
		totalH += h
	}
	if rowsCount > 1 {
		totalH += float32(rowsCount-1) * l.Gap
	}

	return fyne.NewSize(totalW, totalH)
}

func (l *FractionalLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}

	cols, rows := l.parseSpecs()
	colsCount := len(cols)
	rowsCount := len(rows)

	colMinW := make([]float32, colsCount)
	rowMinH := make([]float32, rowsCount)

	for i, obj := range objects {
		if obj == nil || !obj.Visible() {
			continue
		}
		c := i % colsCount
		r := i / colsCount

		if r >= rowsCount {
			continue
		}

		minS := obj.MinSize()
		if minS.Width > colMinW[c] {
			colMinW[c] = minS.Width
		}
		if minS.Height > rowMinH[r] {
			rowMinH[r] = minS.Height
		}
	}

	colWidths := make([]float32, colsCount)
	var nonFrColW float32
	var totalFrColWeight float32
	for c, spec := range cols {
		switch spec.mType {
		case metricFixed:
			colWidths[c] = spec.value
			nonFrColW += spec.value
		case metricAuto:
			colWidths[c] = colMinW[c]
			nonFrColW += colMinW[c]
		case metricFractional:
			totalFrColWeight += spec.value
		}
	}

	gapsW := float32(0)
	if colsCount > 1 {
		gapsW = float32(colsCount-1) * l.Gap
	}
	leftoverW := size.Width - nonFrColW - gapsW
	if leftoverW < 0 {
		leftoverW = 0
	}

	for c, spec := range cols {
		if spec.mType == metricFractional {
			if totalFrColWeight > 0 {
				colWidths[c] = (spec.value / totalFrColWeight) * leftoverW
			} else {
				colWidths[c] = 0
			}
		}
	}

	rowHeights := make([]float32, rowsCount)
	var nonFrRowH float32
	var totalFrRowWeight float32
	for r, spec := range rows {
		switch spec.mType {
		case metricFixed:
			rowHeights[r] = spec.value
			nonFrRowH += spec.value
		case metricAuto:
			rowHeights[r] = rowMinH[r]
			nonFrRowH += rowMinH[r]
		case metricFractional:
			totalFrRowWeight += spec.value
		}
	}

	gapsH := float32(0)
	if rowsCount > 1 {
		gapsH = float32(rowsCount-1) * l.Gap
	}
	leftoverH := size.Height - nonFrRowH - gapsH
	if leftoverH < 0 {
		leftoverH = 0
	}

	for r, spec := range rows {
		if spec.mType == metricFractional {
			if totalFrRowWeight > 0 {
				rowHeights[r] = (spec.value / totalFrRowWeight) * leftoverH
			} else {
				rowHeights[r] = 0
			}
		}
	}

	colX := make([]float32, colsCount)
	var currentX float32
	for c, w := range colWidths {
		colX[c] = currentX
		currentX += w + l.Gap
	}

	rowY := make([]float32, rowsCount)
	var currentY float32
	for r, h := range rowHeights {
		rowY[r] = currentY
		currentY += h + l.Gap
	}

	for i, obj := range objects {
		if obj == nil {
			continue
		}
		c := i % colsCount
		r := i / colsCount

		if r >= rowsCount {
			obj.Resize(fyne.NewSize(0, 0))
			continue
		}

		obj.Move(fyne.NewPos(colX[c], rowY[r]))
		obj.Resize(fyne.NewSize(colWidths[c], rowHeights[r]))
	}
}
