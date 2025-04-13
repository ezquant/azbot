package backtesting

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/exchange"
	"github.com/ezquant/azbot/azbot/plot"
	"github.com/ezquant/azbot/azbot/plus/localkv"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/storage"
	"github.com/ezquant/azbot/examples/strategies"

	log "github.com/sirupsen/logrus"
)

const initialBalance = 10000.0 // 设置初始资金总额

// 用于在goroutine间传递结果的结构体
type summaryResult struct {
	sharpeRatio float64
	err         error
}

// OutputResult 保存解析后的回测数据
type OutputResult struct {
	StartPortfolio float64
	FinalPortfolio float64
	MaxDrawdown    float64
	RawOutput      string
}

// Run executes the backtesting process with the given configuration and database path.
// Parameters:
// - config: The configuration for the backtesting process.
// - databasePath: The path to the database file.

func Run(config *models.Config, databasePath *string) (float64, *plot.Chart) {
	var (
		ctx   = context.Background()
		pairs = make([]string, 0, len(config.AssetWeights))
	)

	for pair := range config.AssetWeights {
		pairs = append(pairs, pair)
	}

	settings := azbot.Settings{
		Pairs: pairs,
	}

	// initialize local KV store for strategies
	kv, err := localkv.NewLocalKV(databasePath)
	if err != nil {
		log.Fatal(err)
	}

	var strategy strategies.Strategy
	switch config.Strategy {
	case "CrossEMA":
		strategy, err = strategies.NewCrossEMA(config, kv)
	case "RLPPO":
		strategy, err = strategies.NewRLPPO(config, kv)
	default:
		log.Fatalf("Unknown strategy: %s", config.Strategy)
	}

	if err != nil {
		log.Fatal(err)
	}

	pairFeed := make([]exchange.PairFeed, 0, len(config.AssetWeights))

	for pair := range config.AssetWeights {
		pairFeed = append(pairFeed, exchange.PairFeed{
			Pair:      pair,
			File:      fmt.Sprintf("testdata/%s-%s.csv", pair, strategy.Timeframe()),
			Timeframe: strategy.Timeframe(),
		})
	}

	csvFeed, err := exchange.NewCSVFeed(strategy.Timeframe(), pairFeed...)
	if err != nil {
		log.Fatal(err)
	}

	storage, err := storage.FromMemory()
	if err != nil {
		log.Fatal(err)
	}

	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", config.BacktestConfig.InitialBalance),
		exchange.WithDataFeed(csvFeed),
	)

	chart, err := plot.NewChart(plot.WithPaperWallet(wallet))
	if err != nil {
		log.Fatal(err)
	}
	bot, err := azbot.NewBot(
		ctx,
		settings,
		wallet,
		strategy,
		azbot.WithBacktest(wallet),
		azbot.WithStorage(storage),
		azbot.WithCandleSubscription(chart),
		azbot.WithOrderSubscription(chart),
		azbot.WithLogLevel(log.WarnLevel),
	)
	if err != nil {
		log.Fatal(err)
	}

	kv.RemoveDB()

	// 重定向错误输出到空设备（关闭回测进度条）
	if err := runWithProgress(false, func() error {
		return bot.Run(ctx)
	}); err != nil {
		return 0, chart
	}

	outputResult, err := captureOutput(func() {
		bot.Summary()
	})
	if err != nil {
		return 0, chart
	}

	// 计算夏普率
	riskFreeRate := 0.02
	returns := (outputResult.FinalPortfolio - outputResult.StartPortfolio) / outputResult.StartPortfolio
	volatility := math.Abs(outputResult.MaxDrawdown / 100.0)
	if volatility == 0 {
		volatility = 0.0001 // 避免除以零
	}
	sharpeRatio := (returns - riskFreeRate) / volatility

	// 打印原始输出
	fmt.Print(outputResult.RawOutput)

	var printDetails bool = false
	if printDetails {
		totalEquity := 0.0
		fmt.Printf("REAL ASSETS VALUE\n")

		for pair := range strategy.GetD().AssetWeights {
			asset, _, err := wallet.Position(pair)
			if err != nil {
				log.Fatal(err)
			}

			assetValue := asset * strategy.GetD().LastClose[pair]
			volume := strategy.GetD().Volume[pair]
			profitPerc := (assetValue - volume) / volume * 100
			fmt.Printf("%s = %.2f USDT, Asset Qty = %f, Profit = %.2f%%\n", pair, assetValue, asset, profitPerc)
			totalEquity += assetValue
		}

		totalVolume := 0.0
		for _, volume := range strategy.GetD().Volume {
			totalVolume += volume
		}

		totalProfit := totalEquity - totalVolume
		totalProfitPerc := totalProfit / totalVolume * 100
		fmt.Printf("TOTAL EQUITY = %.2f USDT, Profit = %.2f = %.2f%%\n--------------\n", totalEquity, totalProfit, totalProfitPerc)
	}

	return sharpeRatio, chart
}

// runWithProgress 临时重定向标准错误输出
// 接受一个函数作为参数，在重定向后执行该函数，然后恢复原有输出
func runWithProgress(withProgress bool, fn func() error) error {
	if withProgress {
		return fn()
	}

	// 保存原始的错误输出
	oldStderr := os.Stderr

	// 重定向到空设备
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	os.Stderr = devNull

	// 执行函数
	err = fn()

	// 恢复错误输出
	os.Stderr = oldStderr

	return err
}

// captureOutput 捕获并解析函数执行过程中的标准输出
// fn 是需要捕获输出的函数
// 返回解析后的输出结果和可能的错误
func captureOutput(fn func()) (*OutputResult, error) {
	// 创建管道用于捕获输出
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	// 保存原始的标准输出
	oldStdout := os.Stdout
	os.Stdout = w

	// 创建一个通道用于同步
	done := make(chan string)

	// 启动 goroutine 读取输出
	go func() {
		var output strings.Builder
		reader := bufio.NewReader(r)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("读取输出时发生错误: %v", err)
				}
				break
			}
			output.WriteString(line)
		}
		done <- output.String()
	}()

	// 执行需要捕获输出的函数
	fn()

	// 恢复标准输出并关闭写入端
	os.Stdout = oldStdout
	w.Close()

	// 等待读取完成并获取输出
	output := <-done
	r.Close()

	// 解析输出
	result := &OutputResult{RawOutput: output}

	// 使用正则表达式解析结果
	startPortfolioRe := regexp.MustCompile(`START PORTFOLIO\s*=\s*([\d.]+)`)
	finalPortfolioRe := regexp.MustCompile(`FINAL PORTFOLIO\s*=\s*([\d.]+)`)
	maxDrawdownRe := regexp.MustCompile(`MAX DRAWDOWN = (-[\d.]+)`)

	matches := startPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		return nil, fmt.Errorf("无法解析起始资金数据")
	}
	result.StartPortfolio, _ = strconv.ParseFloat(matches[1], 64)

	matches = finalPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		return nil, fmt.Errorf("无法解析最终资金数据")
	}
	result.FinalPortfolio, _ = strconv.ParseFloat(matches[1], 64)

	matches = maxDrawdownRe.FindStringSubmatch(output)
	if matches == nil {
		return nil, fmt.Errorf("无法解析最大回撤数据")
	}
	result.MaxDrawdown, _ = strconv.ParseFloat(matches[1], 64)

	return result, nil
}
