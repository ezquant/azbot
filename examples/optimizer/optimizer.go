package optimizer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/exchange"
	"github.com/ezquant/azbot/azbot/plus/localkv"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/storage"
	"github.com/ezquant/azbot/examples/strategies"

	//log "github.com/sirupsen/logrus" // 使用 logrus
	log "github.com/ezquant/azbot/azbot/tools/log"
)

type Optimizer struct {
	config        *models.Config
	results       []OptimizationResult
	parameterSets []map[string]interface{}
	mu            sync.Mutex
	workerCount   int
}

type OptimizationResult struct {
	Parameters map[string]interface{}
	Sharpe     float64
	Returns    float64
	Drawdown   float64
	Profit     float64
}

type outputCapturer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	readError error
}

var (
	successCount atomic.Int32
	failCount    atomic.Int32
	oldStderr    *os.File
)

// 使用全局的 capturer 实例
var globalCapturer = &outputCapturer{}

// Run 是主入口函数，符合 main.go 中的调用方式
func Run(config *models.Config, dbPath *string) {
	log.Infof("开始优化策略 [%s] 的参数...", config.Strategy)

	optimizer := NewOptimizer(config)
	bestResult, err := optimizer.Optimize()
	if err != nil {
		log.Fatal("优化失败: %v", err)
	}

	// 输出最优参数
	log.Info("\n----------------------------------------")
	log.Info("最优参数组合：")
	for name, value := range bestResult.Parameters {
		log.Infof("%s: %v", name, value)
	}
	log.Info("----------------------------------------")
	log.Infof("夏普率: %.2f", bestResult.Sharpe)
	log.Infof("收益率: %.2f%%", bestResult.Returns*100)
	log.Infof("最大回撤: %.2f%%", bestResult.Drawdown*100)
	log.Info("----------------------------------------")

	// 保存最优参数到配置文件
	if err := saveOptimizedConfig(config, bestResult.Parameters); err != nil {
		log.Errorf("保存优化后的配置失败: %v", err)
	}
}

func NewOptimizer(config *models.Config) *Optimizer {
	return &Optimizer{
		config:        config,
		results:       make([]OptimizationResult, 0),
		parameterSets: make([]map[string]interface{}, 0),
		workerCount:   4, // 可配置的并发数
	}
}

// generateParameterSets 生成所有可能的参数组合
func (o *Optimizer) generateParameterSets() {
	log.Info("开始生成参数组合...")
	// 打印每个参数的范围和步长
	for name, param := range o.config.Parameters {
		log.Infof("参数 %s: 最小值=%v, 最大值=%v, 步长=%v",
			name, param.Min, param.Max, param.Step)
	}

	params := make(map[string][]interface{})

	// 为每个参数生成可能的值
	for _, param := range o.config.Parameters {
		values := make([]interface{}, 0)

		switch param.Type {
		case "bool":
			values = append(values, false, true)

		case "int":
			min := param.Min.(int)
			max := param.Max.(int)
			step := param.Step.(int)
			for i := min; i <= max; i += step {
				values = append(values, i)
			}

		case "float":
			min := param.Min.(float64)
			max := param.Max.(float64)
			step := param.Step.(float64)
			for v := min; v <= max+step/2; v += step { // 添加step/2以处理浮点数精度问题
				values = append(values, math.Round(v*100)/100)
			}
		}

		params[param.Name] = values
		log.Infof("参数 %s 的可能值: %v", param.Name, values) // 添加这行来调试
	}

	// 生成笛卡尔积
	o.generateCartesianProduct(params, make(map[string]interface{}), o.config.Parameters)
}

// generateCartesianProduct 递归生成参数的笛卡尔积
func (o *Optimizer) generateCartesianProduct(params map[string][]interface{}, current map[string]interface{}, paramList []models.Parameter) {
	if len(current) == len(params) {
		paramSet := make(map[string]interface{})
		for k, v := range current {
			paramSet[k] = v
		}
		o.parameterSets = append(o.parameterSets, paramSet)
		return
	}

	param := paramList[len(current)]
	for _, val := range params[param.Name] {
		current[param.Name] = val
		o.generateCartesianProduct(params, current, paramList)
		delete(current, param.Name)
	}
}

// Optimize 执行参数优化
func (o *Optimizer) Optimize() (OptimizationResult, error) {
	log.Info("生成参数组合...")
	o.generateParameterSets()
	totalCombinations := len(o.parameterSets)
	log.Infof("共生成 %d 种参数组合", totalCombinations)

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, o.workerCount)
	progress := 0
	var progressMu sync.Mutex

	for i, params := range o.parameterSets {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(parameters map[string]interface{}, index int) {
			defer wg.Done()
			defer func() {
				<-semaphore // 释放信号量
				progressMu.Lock()
				progress++
				if progress%max(1, totalCombinations/20) == 0 { // 每完成5%输出一次进度
					log.Infof("优化进度: %.1f%% (%d/%d)",
						float64(progress)/float64(totalCombinations)*100,
						progress,
						totalCombinations)
				}
				progressMu.Unlock()
			}()

			// 创建配置副本并更新参数
			configCopy := *o.config
			for i := range configCopy.Parameters {
				if val, exists := parameters[configCopy.Parameters[i].Name]; exists {
					configCopy.Parameters[i].Default = val
				}
			}

			// 运行回测
			result, err := o.runBacktest(&configCopy)
			if err != nil {
				log.Errorf("回测失败: %v", err)
				return
			}

			//println("--> 005 got result:", result.Sharpe)
			log.Warnf("Got result: %.2f, %.2f, %.2f; parameters: %v",
				result.Sharpe, result.Returns, result.Drawdown, parameters)

			// 保存结果
			o.mu.Lock()
			o.results = append(o.results, OptimizationResult{
				Parameters: parameters,
				Sharpe:     result.Sharpe,
				Returns:    result.Returns,
				Drawdown:   result.Drawdown,
			})
			o.mu.Unlock()
		}(params, i)
	}

	wg.Wait()

	// 按夏普率排序
	sort.Slice(o.results, func(i, j int) bool {
		return o.results[i].Sharpe > o.results[j].Sharpe
	})

	//println("--> 007 sorted result:", o.results[0].Sharpe)
	if len(o.results) == 0 {
		println("-> 未找到有效的优化结果")
		return OptimizationResult{}, fmt.Errorf("未找到有效的优化结果")
	}

	// 输出前N个最优结果
	o.printTopResults(5)

	return o.results[0], nil
}

func (oc *outputCapturer) Capture(fn func()) (string, error) {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	// 清空缓冲区
	oc.buffer.Reset()

	// 创建管道
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("创建管道失败: %w", err)
	}
	defer r.Close()

	// 保存并重定向标准输出
	oldStdout := os.Stdout
	os.Stdout = w

	// 确保恢复标准输出
	defer func() {
		os.Stdout = oldStdout
	}()

	// 创建一个done通道用于同步
	done := make(chan struct{})

	// 在goroutine中读取管道数据
	go func() {
		defer close(done)
		_, err := io.Copy(&oc.buffer, r)
		if err != nil {
			oc.readError = err
		}
	}()

	// 执行目标函数
	fn()

	// 关闭写入端并等待读取完成
	w.Close()
	<-done

	if oc.readError != nil {
		return "", fmt.Errorf("读取输出失败: %w", oc.readError)
	}

	return oc.buffer.String(), nil
}

// runBacktest 执行单次回测
func (o *Optimizer) runBacktest(config *models.Config) (OptimizationResult, error) {
	ctx := context.Background()

	// 创建本地 KV 存储
	kv, err := localkv.NewLocalKV(nil) // 使用临时内存存储
	if err != nil {
		return OptimizationResult{}, err
	}
	defer kv.RemoveDB()

	// 创建策略实例
	var strategy strategies.Strategy
	switch config.Strategy {
	case "CrossEMA":
		strategy, err = strategies.NewCrossEMA(config, kv)
	default:
		return OptimizationResult{}, fmt.Errorf("未知策略类型: %s", config.Strategy)
	}
	if err != nil {
		return OptimizationResult{}, err
	}

	// 准备数据源
	pairFeed := make([]exchange.PairFeed, 0, len(config.AssetWeights))
	for pair := range config.AssetWeights {
		pairFeed = append(pairFeed, exchange.PairFeed{
			Pair:      pair,
			File:      fmt.Sprintf("testdata/%s-%s.csv", pair, strategy.Timeframe()),
			Timeframe: strategy.Timeframe(),
		})
	}

	// 创建 CSV 数据源
	csvFeed, err := exchange.NewCSVFeed(strategy.Timeframe(), pairFeed...)
	if err != nil {
		return OptimizationResult{}, err
	}

	// 创建存储
	storage, err := storage.FromMemory()
	if err != nil {
		return OptimizationResult{}, err
	}

	// 创建模拟钱包
	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", config.BacktestConfig.InitialBalance), // 使用 BacktestConfig 而不是 Backtest
		exchange.WithDataFeed(csvFeed),
	)

	// 获取交易对列表
	pairs := make([]string, 0, len(config.AssetWeights))
	for pair := range config.AssetWeights {
		pairs = append(pairs, pair)
	}

	// 创建回测引擎
	bot, err := azbot.NewBot(
		ctx,
		azbot.Settings{
			Pairs: pairs,
		},
		wallet,
		strategy,
		azbot.WithBacktest(wallet),
		azbot.WithStorage(storage),
		azbot.WithLogLevel(log.WarnLevel), // 使用 info 级别日志太多
	)
	if err != nil {
		return OptimizationResult{}, err
	}

	// 重定向错误输出到空设备（关闭回测进度条）
	oldStderr = os.Stderr
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		log.Fatal(err)
	}
	os.Stderr = devNull
	// 运行回测
	if err := bot.Run(ctx); err != nil {
		os.Stderr = oldStderr // 恢复错误输出
		return OptimizationResult{}, err
	}
	// 恢复错误输出
	os.Stderr = oldStderr

	// 使用互斥锁保护输出重定向操作
	//var outputMutex sync.Mutex
	//outputMutex.Lock()
	//defer outputMutex.Unlock()

	// 运行回测后捕获输出
	output, err := globalCapturer.Capture(func() {
		bot.Summary()
	})
	if err != nil {
		log.Errorf("捕获输出失败: %v", err)
		return OptimizationResult{}, err
	}

	//println("--> 003: length of output", len(output))

	if len(output) < 100 { // 添加基本的输出长度检查
		return OptimizationResult{}, fmt.Errorf("输出数据异常：长度=%d", len(output))
	}

	// 使用正则表达式提取需要的信息
	var (
		returns float64
		//drawdown float64
		//profit   float64
	)

	// 使用正则表达式解析结果
	startPortfolioRe := regexp.MustCompile(`START PORTFOLIO\s*=\s*([\d.]+)`)
	finalPortfolioRe := regexp.MustCompile(`FINAL PORTFOLIO\s*=\s*([\d.]+)`)
	maxDrawdownRe := regexp.MustCompile(`MAX DRAWDOWN = (-[\d.]+)`)
	// 增强容错处理
	//startPortfolioRe := regexp.MustCompile(`(?i)start.*?portfolio\s*[:=]?\s*([\d.]+)`)
	//finalPortfolioRe := regexp.MustCompile(`(?i)final.*?portfolio\s*[:=]?\s*([\d.]+)`)
	//maxDrawdownRe := regexp.MustCompile(`(?i)max(?:imum)?\s*drawdown\s*[:=]?\s*(-?[\d.]+)%?`)

	matches := startPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		//println("--> 101 output:", output)
		failCount.Add(1)
		log.Errorf("无法解析起始资金数据 err %d", failCount.Load())
		return OptimizationResult{}, err
	}
	startPortfolio, _ := strconv.ParseFloat(matches[1], 64)

	matches = finalPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		failCount.Add(1)
		//println("--> 102 output:", output)
		log.Errorf("无法解析最终资金数据 err %d", failCount.Load())
		return OptimizationResult{}, err
	}
	finalPortfolio, _ := strconv.ParseFloat(matches[1], 64)

	matches = maxDrawdownRe.FindStringSubmatch(output)
	if matches == nil {
		//println("--> 103 output:", output)
		failCount.Add(1)
		log.Errorf("无法解析最大回撤数据 err %d", failCount.Load())
		return OptimizationResult{}, err
	}
	maxDrawdown, _ := strconv.ParseFloat(matches[1], 64)

	//println("--> 004 ", startPortfolio, finalPortfolio, maxDrawdown)
	// 计算夏普率
	riskFreeRate := 0.02
	returns = (finalPortfolio - startPortfolio) / startPortfolio
	volatility := math.Abs(maxDrawdown / 100.0)
	if volatility == 0 {
		volatility = 0.0001 // 避免除以零
	}
	sharpeRatio := (returns - riskFreeRate) / volatility

	successCount.Add(1)

	return OptimizationResult{
		Sharpe:   sharpeRatio,
		Returns:  returns,
		Drawdown: maxDrawdown,                     // 使用解析得到的最大回撤值
		Profit:   finalPortfolio - startPortfolio, // 计算实际利润
	}, nil
}

// printTopResults 输出前N个最优结果
func (o *Optimizer) printTopResults(n int) {
	if len(o.results) == 0 {
		return
	}

	log.Warnf("优化回测结果解析成功率: %.01f%%",
		float64(successCount.Load())/float64(successCount.Load()+failCount.Load())*100)
	//os.Stderr = oldStderr // 恢复错误输出（必需）
	//log.SetOutput(os.Stderr) // 默认输出到 stderr
	//log.SetLevel(log.InfoLevel)
	log.Warnf("最优参数组合（前5个）:")
	log.Warnf("----------------------------------------")
	log.Warnf("排名 | 夏普率 | 收益率 | 最大回撤 | 参数")
	log.Warnf("----------------------------------------")

	for i := 0; i < min(n, len(o.results)); i++ {
		result := o.results[i]
		log.Warnf(
			"#%d | %.2f | %.2f%% | %.2f%% | %v",
			i+1,
			result.Sharpe,
			result.Returns*100,
			result.Drawdown*100,
			result.Parameters)
	}
	log.Warnf("----------------------------------------")
}

// saveOptimizedConfig 保存优化后的配置
func saveOptimizedConfig(config *models.Config, bestParams map[string]interface{}) error {
	// 更新配置中的默认参数
	for i := range config.Parameters {
		if val, exists := bestParams[config.Parameters[i].Name]; exists {
			config.Parameters[i].Default = val
		}
	}

	// 生成优化后的配置文件名
	optimizedConfigPath := fmt.Sprintf("user_data/config_%s_optimized.yml", config.Strategy)

	// 保存配置
	// 假设 models.Config 有一个 Save 方法
	err := config.Save(optimizedConfigPath) // 使用 Config 的 Save 方法替代 SaveConfig
	if err != nil {
		return fmt.Errorf("保存优化后的配置失败: %v", err)
	}

	log.Infof("优化后的配置已保存到: %s", optimizedConfigPath)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
