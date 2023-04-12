package indicator

import (
	"fmt"
	"time"

	"github.com/ezquant/azbot/azbot/model"
	"github.com/ezquant/azbot/azbot/plot"

	"github.com/markcheno/go-talib"
)

func CCI(period int, color string) plot.Indicator {
	return &cci{
		Period: period,
		Color:  color,
	}
}

type cci struct {
	Period int
	Color  string
	Values model.Series[float64]
	Time   []time.Time
}

func (c cci) Warmup() int {
	return c.Period
}

func (c cci) Name() string {
	return fmt.Sprintf("CCI(%d)", c.Period)
}

func (c cci) Overlay() bool {
	return false
}

func (c *cci) Load(dataframe *model.Dataframe) {
	c.Values = talib.Cci(dataframe.High, dataframe.Low, dataframe.Close, c.Period)[c.Period:]
	c.Time = dataframe.Time[c.Period:]
}

func (c cci) Metrics() []plot.IndicatorMetric {
	return []plot.IndicatorMetric{
		{
			Color:  c.Color,
			Style:  "line",
			Values: c.Values,
			Time:   c.Time,
		},
	}
}
