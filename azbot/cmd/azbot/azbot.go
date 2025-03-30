package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ezquant/azbot/azbot/download"
	"github.com/ezquant/azbot/azbot/exchange"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/examples/backtesting"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func main() {
	app := &cli.App{
		Name:     "azbot",
		HelpName: "azbot",
		Usage:    "Utilities for bot creation",
		Commands: []*cli.Command{
			{
				Name:     "download",
				HelpName: "download",
				Usage:    "Download historical data",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "pair",
						Aliases:  []string{"p"},
						Usage:    "eg. BTCUSDT",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "days",
						Aliases:  []string{"d"},
						Usage:    "eg. 100 (default 30 days)",
						Required: false,
					},
					&cli.TimestampFlag{
						Name:     "start",
						Aliases:  []string{"s"},
						Usage:    "eg. 2021-12-01",
						Layout:   "2006-01-02",
						Required: false,
					},
					&cli.TimestampFlag{
						Name:     "end",
						Aliases:  []string{"e"},
						Usage:    "eg. 2020-12-31",
						Layout:   "2006-01-02",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "timeframe",
						Aliases:  []string{"t"},
						Usage:    "eg. 1h",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "output",
						Aliases:  []string{"o"},
						Usage:    "eg. ./btc.csv",
						Required: true,
					},
					&cli.BoolFlag{
						Name:     "futures",
						Aliases:  []string{"f"},
						Usage:    "true or false",
						Value:    false,
						Required: false,
					},
				},
				Action: func(c *cli.Context) error {
					var (
						exc service.Feeder
						err error
					)

					if c.Bool("futures") {
						// fetch data from binance futures
						exc, err = exchange.NewBinanceFuture(c.Context)
						if err != nil {
							return err
						}
					} else {
						// fetch data from binance spot
						exc, err = exchange.NewBinance(c.Context)
						if err != nil {
							return err
						}
					}

					var options []download.Option
					if days := c.Int("days"); days > 0 {
						options = append(options, download.WithDays(days))
					}

					start := c.Timestamp("start")
					end := c.Timestamp("end")
					if start != nil && end != nil && !start.IsZero() && !end.IsZero() {
						options = append(options, download.WithInterval(*start, *end))
					} else if start != nil || end != nil {
						log.Fatal("START and END must be informed together")
					}

					return download.NewDownloader(exc).Download(c.Context, c.String("pair"),
						c.String("timeframe"), c.String("output"), options...)

				},
			},
			{
				Name:     "backtest",
				HelpName: "backtest",
				Usage:    "Run backtesting for a custom strategy",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "eg. ./user_data/config_CrossEMA.yml",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					// 调用 backtesting.go 中的主逻辑
					// bug修复：将 c.String("config") 的返回值转换为 *string 类型
					config, err := ReadConfig(c.String("config"))
					if err != nil {
						log.Fatalf("cannot read config file: %v", err)
					}
					databasePath := "./user_data/db"
					sharpeRatio, chart := backtesting.Run(config, &databasePath)
					fmt.Printf("\n策略评估指标:\n")
					fmt.Printf("夏普率: %.2f\n", sharpeRatio)
					fmt.Printf("夏普率解读: \n")
					fmt.Printf("- 小于0: 策略表现差于无风险利率\n")
					fmt.Printf("- 0-1: 策略风险调整后收益一般\n")
					fmt.Printf("- 1-2: 策略风险调整后收益良好\n")
					fmt.Printf("- 大于2: 策略风险调整后收益优秀\n")

					// Display candlesticks chart in browser
					err = chart.Start()
					if err != nil {
						log.Fatal(err)
					}
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func ReadConfig(path string) (config *models.Config, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config = &models.Config{}
	err = yaml.Unmarshal(data, config)

	return
}
