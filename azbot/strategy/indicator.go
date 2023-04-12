package strategy

import (
	"time"

	"github.com/ezquant/azbot/azbot/model"
)

type MetricStyle string

const (
	StyleBar       = "bar"
	StyleScatter   = "scatter"
	StyleLine      = "line"
	StyleHistogram = "histogram"
	StyleWaterfall = "waterfall"
)

type IndicatorMetric struct {
	Name   string
	Color  string
	Style  MetricStyle // default: line
	Values model.Series[float64]
}

type ChartIndicator struct {
	Time      []time.Time
	Metrics   []IndicatorMetric
	Overlay   bool
	GroupName string
	Warmup    int
}
