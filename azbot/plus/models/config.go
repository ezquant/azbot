package models

type Parameter struct {
	Name    string      `yaml:"name"`
	Type    string      `yaml:"type"`
	Default interface{} `yaml:"default"`
	Min     interface{} `yaml:"min"`
	Max     interface{} `yaml:"max"`
	Step    interface{} `yaml:"step"`
}

type Config struct {
	Strategy       string      `yaml:"strategy"`
	Parameters     []Parameter `yaml:"parameters"`
	BacktestConfig struct {
		Timeframe      string  `yaml:"timeframe"`
		InitialBalance float64 `yaml:"initial_balance"`
		Fee            float64 `yaml:"fee"`
		Slippage       float64 `yaml:"slippage"`
	} `yaml:"backtest"`
	AssetWeights map[string]float64 `yaml:"asset_weights,flow"`
}

// Save 保存配置到指定路径
func (c *Config) Save(path string) error {
	// 实现配置保存逻辑
	// 例如使用 yaml.Marshal 和 ioutil.WriteFile
	return nil
}
